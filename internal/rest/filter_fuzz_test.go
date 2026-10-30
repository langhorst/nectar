package rest

import (
	"strings"
	"testing"

	"github.com/langhorst/nectar/internal/conformance"
)

// FuzzParseFilter feeds arbitrary text to the _filter parser, which reads
// client input with its own grammar and recursion. The property is only that
// it answers: a panic or a stack overflow would take the server down with it.
//
// The seeds run on every `go test`; `go test -fuzz=FuzzParseFilter` explores
// beyond them.
func FuzzParseFilter(f *testing.F) {
	for _, seed := range []string{
		`name eq "Chalmers"`,
		`name eq "Chalmers" or (name eq "Nowak" and gender eq male)`,
		`not (name eq "Chalmers")`,
		`birthdate pr true`,
		`identifier eq http://example.org/mrn|A1`,
		`patient.family sw "Chal"`,
		`subject[Patient].family sw "Chal"`,
		`gender in http://example.org/vs`,
		`(name eq "x"`,
		`name eq "x")`,
		`name eq "x`,
		`name eq "a\"b\\c"`,
		strings.Repeat("(", 40) + `name eq "x"` + strings.Repeat(")", 40),
		strings.Repeat("not(", 40) + `name eq "x"` + strings.Repeat(")", 40),
		strings.Repeat(`name eq "x" or `, 300) + `name eq "x"`,
		strings.Repeat("( ", 5000),
	} {
		f.Add(seed)
	}
	idx := conformance.MustLoad(conformance.R5)
	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = parseFilter(idx, "Patient", raw)
	})
}
