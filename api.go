package acorn

import "encoding/json"

// Parse parses JavaScript source (as a module, latest ECMAScript version)
// and returns the ESTree AST as a JSON-serializable value. The returned tree
// uses map[string]interface{} nodes with "type", "start", "end" and
// per-construct fields, matching acorn 8.x output. Positions are 0-based
// character offsets (byte offsets into input, which equal JS code-unit
// offsets for ASCII input).
func Parse(input string) (v interface{}, err error) {
	return newBaseParser(input).run()
}

// ParseError is returned when the source cannot be parsed.
type ParseError struct {
	Msg string
	Pos int
}

func (e *ParseError) Error() string { return e.Msg }

// ParseJSON is a convenience returning the AST marshaled to JSON.
func ParseJSON(input string) (string, error) {
	v, err := Parse(input)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
