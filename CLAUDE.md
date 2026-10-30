# CLAUDE.md

Nectar is a FHIR R4 and R5 server in Go: one binary, conformance data compiled
in, SQLite or PostgreSQL storage. The README describes behavior for users; this
file is how to work on the code. Keep it current: a change that makes a line
here wrong updates that line in the same pull request.

## Commands

```sh
make check          # gofmt check, go vet, race tests: the gate before every push
make check-parity   # the storage and REST suites on PostgreSQL too
make build          # bin/nectar; run it with: ./bin/nectar serve -db :memory:
make vendor         # fetch the packages pinned in third_party/packages.lock
make gen            # recompile them into internal/conformance/data
make gen-check      # fail if the committed indexes are stale
```

Focused runs:

```sh
go test -run TestFilter ./internal/rest/
go test -run 'TestConformance/SearchByDateRange' ./internal/storage/sqlite/
go test -run '^$' -fuzz '^FuzzFromXML$' -fuzztime 60s ./internal/resource/
NECTAR_SUITE_ALL=1 go test -run TestOfficialFHIRPathSuites ./internal/fhirpath/
```

### PostgreSQL for the parity gate

`make check` runs on SQLite only unless `NECTAR_TEST_POSTGRES` is set, and says
so. CI always runs both engines. Tests create a schema each, so any database
will do.

With Docker, matching CI:

```sh
docker run -d --name nectar-pg -p 5432:5432 -e POSTGRES_USER=nectar \
  -e POSTGRES_PASSWORD=nectar -e POSTGRES_DB=nectar postgres:16
export NECTAR_TEST_POSTGRES='postgres://nectar:nectar@localhost:5432/nectar?sslmode=disable'
```

Without Docker, with PostgreSQL installed. It refuses to run as root, so a
root shell needs another user:

```sh
PG_BIN=$(ls -d /usr/lib/postgresql/*/bin | sort -V | tail -1)
id pgdev >/dev/null 2>&1 || useradd -m pgdev
su pgdev -c "$PG_BIN/initdb -D /home/pgdev/pg -U nectar --auth=trust >/dev/null"
su pgdev -c "$PG_BIN/pg_ctl -D /home/pgdev/pg -l /home/pgdev/pg.log -o '-p 5433 -k /home/pgdev' start"
export NECTAR_TEST_POSTGRES='postgres://nectar@/postgres?host=/home/pgdev&port=5433&sslmode=disable'
```

## House rules

- **Pull requests into `main`, never a direct push.** `main` is protected; the
  `lint + race tests` and `storage parity (SQLite + PostgreSQL)` checks must pass.
- **One change per commit.** An imperative subject, a body that says why, and
  the issue it closes (`Fixes #12`) or advances (`Part of #49`).
- **Prove a fix.** A bug fix comes with a test that fails without it. Run the
  test against the old code once before committing.
- **Every error is an OperationOutcome.** In `internal/rest`, every failure goes
  through `s.fail`. There are no bare status codes.
- **Refuse rather than fake.** When the server cannot do something, such as a
  modifier that needs terminology or an invariant it cannot evaluate, it answers
  400 or reports "not checked". Never return an empty result or a pass that
  implies a check happened.
- **No SQL outside `internal/storage`.** Search builds a typed
  `storage.SearchQuery`; `internal/storage/sqlstore` renders it for both engines,
  and engine differences go through its `Dialect`.
- **Storage behavior is tested on both engines.** It goes in
  `internal/storage/storagetest`, which SQLite and PostgreSQL both run.
- **Bound anything that recurses over client input.** Go cannot recover from a
  stack overflow, so one deep request kills the process. Request bodies are held
  to `resource.MaxDepth` in `readResource`, and `_filter` has limits of its own.
  A new parser of client input needs a limit and a fuzz target.
- **JSON and XML stay symmetric.** Both readers must accept the same documents
  and produce the same shape. The store re-parses every write before committing
  and refuses one that would not read back.
- **Divergence lists stay honest in both directions.** `knownDivergences` in
  `internal/fhirpath/suite_test.go` and `expectedErrors` in
  `internal/validate/corpus_test.go` fail the build when a listed case starts
  passing, as well as when an unlisted one fails. Update them, with the reason,
  in the same change.
- **Generated data is committed, never edited by hand.** Change `tools/confgen`,
  run `make gen`, and commit the code and `internal/conformance/data` together.
- **Migrations are append-only.** Each file in `internal/storage/sqlite/migrations`
  and `internal/storage/postgres/migrations` is applied once, by name. Add a new
  numbered file for each engine and never edit one that has shipped. Existing
  index rows are not rebuilt by a migration.
- **Docs follow behavior.** A change a client can see updates the README too.

## Package map

| Path | Holds |
|---|---|
| `cmd/nectar` | The daemon: `serve`, `conformance`, `version` |
| `internal/conformance` | Loads the embedded index for one release |
| `internal/conformance/model` | Index types and type-system navigation, shared with `confgen` |
| `internal/fhirpath` | FHIRPath lexer, parser, evaluator and functions |
| `internal/resource` | Untyped documents: the JSON and XML readers and writers |
| `internal/rest` | HTTP: routes, handlers, search parsing, `_filter`, transactions, CapabilityStatement, `$validate` |
| `internal/smart` | SMART App Launch: authorization server, tokens, scopes, compartment filters |
| `internal/validate` | Structure, invariants, bindings and profiles |
| `internal/storage` | The `Backend` interface and the query types |
| `internal/storage/index` | Search parameter extraction into index rows |
| `internal/storage/sqlstore` | The one SQL implementation for both engines |
| `internal/storage/sqlite`, `postgres` | Dialects and migrations |
| `internal/storage/storagetest` | The backend suite both engines run |
| `tools/confgen`, `tools/vendorpkg` | Compile and fetch the conformance packages |

## Where to start

- **A search feature:** parse in `internal/rest/search.go` into a
  `storage.ParamMatch`, render it in `internal/storage/sqlstore/search.go`, and
  index it in `internal/storage/index/extract.go`. Test it over HTTP in
  `internal/rest/search_test.go` or `advanced_test.go`.
- **A REST interaction:** add the route in `routes` in `internal/rest/server.go`
  and the handler in `handlers.go`. Bundle entries are dispatched back through
  the same handler, so they get it too unless `transaction_parse.go` refuses the
  method.
- **An operation:** only `$validate` exists, routed in `server.go` and handled in
  `validate.go`. Routing `$name` segments in general is #14.
- **A FHIRPath function:** `internal/fhirpath/functions*.go`, then run the suite.

REST tests start a server with `newServer(t)`, over PostgreSQL when
`NECTAR_TEST_POSTGRES` is set, and drive it with `c.expect(status, method, path,
body, headers...)`, `c.createPatient`, `c.total(query)`, `assertOutcome` and
`assertBadRequest`. The REST and storage suites load R5; running them on R4
too is #48.

## Environment notes

- `packages.fhir.org` is often unreachable from sandboxes, so the pins in
  `third_party/packages.lock` use npm and GitHub mirrors. That is why US Core is
  not vendored.
- `vuln.go.dev` and `hapifhir.io` may be blocked as well; say so rather than
  guessing at what they would have returned.
