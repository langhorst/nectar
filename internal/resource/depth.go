package resource

import "fmt"

// MaxDepth is how deeply a document a client sends may nest, counted the way
// JSON counts it: every object and every array is one level.
//
// No real resource comes close; a Bundle of deeply nested Questionnaires stays
// under fifty. But a document only has to be well formed to reach the server,
// and the code that walks one -- XML conversion, validation, FHIRPath, index
// extraction, serialization -- recurses once per level. Go cannot recover from
// a stack overflow, so a document nested deeply enough would exit the whole
// process. The limit also keeps every accepted document far inside the 10,000
// levels encoding/json reads, so whatever is stored can be read back.
//
// Stored documents are not held to it. They were checked on the way in, and
// refusing one on the way out would leave it in the database, unreadable.
const MaxDepth = 1000

// maxXMLDepth is the element depth that converts to at most MaxDepth levels of
// JSON. A repeating element becomes an array holding an object, so one element
// can cost two levels.
const maxXMLDepth = MaxDepth / 2

// CheckDepth refuses a document that nests more than MaxDepth levels deep.
func CheckDepth(node *Node) error {
	if exceedsDepth(node.value, MaxDepth) {
		return fmt.Errorf("resource: the document nests more than %d levels deep", MaxDepth)
	}
	return nil
}

// exceedsDepth reports whether v nests more than limit levels. Its own
// recursion is bounded: encoding/json refuses documents deeper than 10,000
// levels, and FromXML stops at maxXMLDepth while it is still tokenizing.
func exceedsDepth(v any, limit int) bool {
	switch x := v.(type) {
	case map[string]any:
		if limit == 0 {
			return true
		}
		for _, item := range x {
			if exceedsDepth(item, limit-1) {
				return true
			}
		}
	case []any:
		if limit == 0 {
			return true
		}
		for _, item := range x {
			if exceedsDepth(item, limit-1) {
				return true
			}
		}
	}
	return false
}
