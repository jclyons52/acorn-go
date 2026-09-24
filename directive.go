package acorn

import "regexp"

// directive.go — acorn's strictDirective(): the "use strict" prologue scan.
//
// acorn scans the raw text between a function's head and its body (and at the
// top of the program) for a leading string literal that is exactly "use strict"
// and is followed by a statement terminator. It cannot rely on the AST because
// the directive has to be known before the body is parsed.

var (
	// acorn: /(?:\s|\/\/.*|\/\*[^]*?\*\/)* /g — whitespace, line and block
	// comments. `[^]` (any char) becomes `[\s\S]` in Go.
	directiveSkipWhiteSpace = regexp.MustCompile(`(?:\s|//.*|/\*(?s:.*?)\*/)*`)
	// acorn: /^(?:'((?:\\[^]|[^'\\])*?)'|"((?:\\[^]|[^"\\])*?)")/
	directiveLiteral = regexp.MustCompile(`^(?:'((?:\\[\s\S]|[^'\\])*?)'|"((?:\\[\s\S]|[^"\\])*?)")`)
	// acorn's lineBreak: /\r\n?|\n|\u2028|\u2029/
	directiveLineBreak = regexp.MustCompile("\r\n?|\n|\u2028|\u2029")
	// acorn: /[(`.[+\-/*%<>=,?^&]/
	directiveUnsafeAfter = regexp.MustCompile("[(`.[+\\-/*%<>=,?^&]")
)

// strictDirective reports whether a "use strict" directive starts at (or after
// whitespace/comments at) offset start in the source text.
func (p *Parser) strictDirective(start int) bool {
	for start < len(p.input) {
		// Try to find a string literal.
		start += directiveSkipWhiteSpace.FindStringIndex(p.input[start:])[1]

		loc := directiveLiteral.FindStringSubmatchIndex(p.input[start:])
		if loc == nil {
			return false
		}
		// Go reports -1 for a capture group that did not participate, so the
		// group has to be checked before it is sliced: a double-quoted literal
		// leaves the single-quoted group unmatched (and vice versa), and slicing
		// by -1 panics. `"use strict";` — the most common directive in shipped
		// npm code — is exactly that shape.
		value := ""
		switch {
		case loc[2] >= 0:
			value = p.input[start+loc[2] : start+loc[3]]
		case loc[4] >= 0:
			value = p.input[start+loc[4] : start+loc[5]]
		}
		if value == "use strict" {
			after := start + loc[1]
			spaceAfterEnd := after + directiveSkipWhiteSpace.FindStringIndex(p.input[after:])[1]
			var next string
			if spaceAfterEnd < len(p.input) {
				next = p.input[spaceAfterEnd : spaceAfterEnd+1]
			}
			nextIsEquals := spaceAfterEnd+1 < len(p.input) && p.input[spaceAfterEnd+1] == '='
			return next == ";" || next == "}" ||
				(directiveLineBreak.MatchString(p.input[after:spaceAfterEnd]) &&
					!(directiveUnsafeAfter.MatchString(next) || (next == "!" && nextIsEquals)))
		}
		start += loc[1]

		// Skip semicolon, if any.
		start += directiveSkipWhiteSpace.FindStringIndex(p.input[start:])[1]
		if start < len(p.input) && p.input[start] == ';' {
			start++
		}
	}
	return false
}
