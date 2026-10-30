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
