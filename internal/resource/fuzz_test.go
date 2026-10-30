package resource_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/langhorst/nectar/internal/conformance"
	"github.com/langhorst/nectar/internal/resource"
)

// The readers turn client bytes into documents the server stores. The property
// these fuzz targets hold them to is the one whose failure once left rows the
// server could not read: anything a reader accepts, within resource.MaxDepth,
// serializes to JSON that the JSON reader accepts again, and to XML without
// failing.
//
// The seeds -- the HL7 example resources plus the inputs behind earlier bugs --
// run on every `go test`; `go test -fuzz=FuzzFromXML` explores beyond them.

func FuzzFromJSON(f *testing.F) {
	addExampleSeeds(f, ".json")
	for _, seed := range []string{
		`{"resourceType":"Patient","active":"yes"}`,
		`{"resourceType":"Observation","valueQuantity":{"value":"1.0.0"}}`,
		`{"resourceType":"Patient","_birthDate":{"extension":[{"url":"http://example.org/x","valueCode":"unknown"}]}}`,
		`{"resourceType":"Patient","name":[{"given":[null,"Ann"],"_given":[{"id":"g1"},null]}]}`,
		`{"resourceType":"Patient"` + strings.Repeat(`,"extension":[{"url":"x"`, 600) + strings.Repeat(`}]`, 600) + `}`,
	} {
		f.Add([]byte(seed))
	}
	idx := conformance.MustLoad(conformance.R5)
	f.Fuzz(func(t *testing.T, data []byte) {
		node, err := resource.FromJSON(idx, data)
		if err != nil || resource.CheckDepth(node) != nil {
			return
		}
		assertReadsBack(t, idx, node)
	})
}

func FuzzFromXML(f *testing.F) {
	addExampleSeeds(f, ".xml")
	for _, seed := range []string{
		`<Patient xmlns="http://hl7.org/fhir"><active value="yes"/></Patient>`,
		`<Patient xmlns="http://hl7.org/fhir"><birthDate><extension url="http://example.org/x"><valueCode value="unknown"/></extension></birthDate></Patient>`,
		`<Observation xmlns="http://hl7.org/fhir"><valueQuantity><value value="1.0.0"/></valueQuantity></Observation>`,
		`<Patient xmlns="http://hl7.org/fhir"><name><given/><given value=""/></name></Patient>`,
		`<Patient xmlns="http://hl7.org/fhir">` + strings.Repeat(`<extension url="x">`, 600) + strings.Repeat(`</extension>`, 600) + `</Patient>`,
	} {
		f.Add([]byte(seed))
	}
	idx := conformance.MustLoad(conformance.R5)
	f.Fuzz(func(t *testing.T, data []byte) {
		node, err := resource.FromXML(idx, data)
		if err != nil || resource.CheckDepth(node) != nil {
			return
		}
		assertReadsBack(t, idx, node)
	})
}

// assertReadsBack checks that an accepted document serializes to JSON the JSON
// reader accepts, which is what the store requires of everything it keeps, and
// that it renders as XML.
func assertReadsBack(t *testing.T, idx *conformance.Index, node *resource.Node) {
	t.Helper()
	data, err := node.MarshalJSON()
	if err != nil {
		t.Fatalf("serializing an accepted document: %v", err)
	}
	if _, err := resource.FromJSON(idx, data); err != nil {
		t.Fatalf("an accepted document does not read back: %v\n%.500s", err, data)
	}
	if _, err := node.XML(""); err != nil {
		t.Fatalf("rendering an accepted document as XML: %v", err)
	}
}

// addExampleSeeds adds the HL7 example resources with the given extension as
// seeds: real documents, with narrative, extensions, contained resources and
// choice elements, for the fuzzer to mutate.
func addExampleSeeds(f *testing.F, ext string) {
	matches, err := filepath.Glob(filepath.Join(examplesDir, "*"+ext))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range matches {
		if filepath.Base(path) == "tests.xml" {
			// The FHIRPath suite itself, not a resource.
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
}
