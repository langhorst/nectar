package rest_test

import (
	"net/http"
	"strings"
	"testing"
)

// Tests for what the server does with hostile or malformed request bodies.
// Each failure here once either took the whole process down or left stored
// data the server could no longer read.

// deepPatientXML nests n extensions under a Patient, n+1 elements deep.
func deepPatientXML(n int) string {
	return `<Patient xmlns="http://hl7.org/fhir">` +
		strings.Repeat(`<extension url="http://example.org/x">`, n) +
		strings.Repeat(`</extension>`, n) + `</Patient>`
}

// deepPatientJSON nests n extensions under a Patient, 2n+1 levels deep: each
// extension is an array holding an object.
func deepPatientJSON(n int) string {
	return `{"resourceType":"Patient"` +
		strings.Repeat(`,"extension":[{"url":"http://example.org/x"`, n) +
		strings.Repeat(`}]`, n) + `}`
}

// pathOf turns a Location header into a path the test client can request.
func pathOf(t *testing.T, location string) string {
	t.Helper()
	i := strings.Index(location, "/Patient/")
	if i < 0 {
		t.Fatalf("Location %q does not name a Patient", location)
	}
	return location[i:]
}

// A document nested past the limit is refused before anything recursive runs
// over it, and refusing it leaves nothing behind.
func TestDeepDocumentsAreRefused(t *testing.T) {
	c := newServer(t)
	cases := []struct{ name, contentType, body, phrase string }{
		{"xml", "application/fhir+xml", deepPatientXML(600), "more than 500 elements deep"},
		{"json", "application/fhir+json", deepPatientJSON(600), "more than 1000 levels deep"},
		// Deep enough that converting it recursively would take a large share
		// of the stack; it now fails while the XML is still being tokenized.
		{"xml, 100,000 elements", "application/fhir+xml", deepPatientXML(100_000), "more than 500 elements deep"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.do("POST", "/Patient", tc.body, "Content-Type", tc.contentType)
			if resp.status != http.StatusBadRequest {
				t.Fatalf("status %d, want 400\nbody: %.300s", resp.status, resp.body)
			}
			assertOutcome(t, resp, "invalid")
			if !strings.Contains(string(resp.body), tc.phrase) {
				t.Errorf("outcome does not mention %q: %.300s", tc.phrase, resp.body)
			}
		})
	}
	if got := c.total("/Patient"); got != 0 {
		t.Errorf("a refused document was stored: total = %v", got)
	}

	// At the limit, a document stores and reads back in either format.
	for _, tc := range []struct{ contentType, body string }{
		{"application/fhir+xml", deepPatientXML(499)},
		{"application/fhir+json", deepPatientJSON(499)},
	} {
		resp := c.expect(http.StatusCreated, "POST", "/Patient", tc.body, "Content-Type", tc.contentType)
		c.expect(http.StatusOK, "GET", pathOf(t, resp.headers.Get("Location")), "")
	}
	c.expect(http.StatusOK, "GET", "/Patient", "")
	c.expect(http.StatusOK, "GET", "/_history", "")
}

// A data-absent-reason sent as XML is an element with an extension and no
// value attribute. It used to be stored with an invented value -- "" for a
// date, false for a boolean -- and a value-less primitive stored from JSON
// disappeared from XML responses, extension and all.
func TestValuelessPrimitivesSurviveXML(t *testing.T) {
	c := newServer(t)
	const absent = `<extension url="http://hl7.org/fhir/StructureDefinition/data-absent-reason">` +
		`<valueCode value="unknown"/></extension>`
	resp := c.expect(http.StatusCreated, "POST", "/Patient",
		`<Patient xmlns="http://hl7.org/fhir"><active>`+absent+`</active>`+
			`<birthDate>`+absent+`</birthDate></Patient>`,
		"Content-Type", "application/fhir+xml")
	path := pathOf(t, resp.headers.Get("Location"))

	stored := c.expect(http.StatusOK, "GET", path, "").json(t)
	for _, element := range []string{"active", "birthDate"} {
		if v, present := stored[element]; present {
			t.Errorf("%s was stored with the value %#v; it has none", element, v)
		}
		if _, present := stored["_"+element]; !present {
			t.Errorf("%s lost its data-absent-reason", element)
		}
	}

	xml := string(c.expect(http.StatusOK, "GET", path, "", "Accept", "application/fhir+xml").body)
	for _, want := range []string{"<active>" + absent + "</active>", "<birthDate>" + absent + "</birthDate>"} {
		if !strings.Contains(xml, want) {
			t.Errorf("XML response is missing %s\n%s", want, xml)
		}
	}
}

// An XML number that is not a number once went into the stored JSON as a
// number token, so the stored document no longer parsed and every later search
// of the type failed. It is now kept as text, as the JSON reader keeps it, and
// validation says what is wrong with it.
func TestInvalidXMLNumbersStayReadable(t *testing.T) {
	c := newServer(t)
	body := `<Observation xmlns="http://hl7.org/fhir"><status value="final"/>` +
		`<code><text value="x"/></code>` +
		`<valueQuantity><value value="1.0.0"/></valueQuantity></Observation>`
	c.expect(http.StatusCreated, "POST", "/Observation", body, "Content-Type", "application/fhir+xml")
	c.expect(http.StatusOK, "GET", "/Observation", "")
	c.expect(http.StatusOK, "GET", "/_history", "")

	resp := c.expect(http.StatusOK, "POST", "/Observation/$validate", body,
		"Content-Type", "application/fhir+xml")
	if !strings.Contains(string(resp.body), "1.0.0") {
		t.Errorf("validation does not report the invalid value: %s", resp.body)
	}
}
