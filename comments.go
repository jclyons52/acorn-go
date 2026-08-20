package acorn

// Comment is a line or block comment encountered while parsing, captured when
// ParseWithComments or ParseComments is used. This mirrors acorn's onComment
// callback (needed by espree for comment attachment).
//
// Positions are 0-based byte offsets into input. Block=true for /* ... */,
// false for // ... and #!-style line/hashbang comments.
type Comment struct {
	Block bool
	Text  string
	Start int
	End   int
}

// ParseComments parses source (as a module, latest ECMAScript) and also
// returns every comment encountered, in source order. It is the acorn-go
// analogue of acorn's onComment: useful for espree-style comment attachment.
func ParseComments(input string) (v interface{}, comments []Comment, err error) {
	p := newBaseParser(input)
	p.collectComments = true
	v, err = p.run()
	if err != nil {
		return nil, nil, err
	}
	return v, p.comments, nil
}

// ParseWithComments is an alias of ParseComments retained for symmetry with the
// existing Parse/ParseJSON names.
func ParseWithComments(input string) (v interface{}, comments []Comment, err error) {
	return ParseComments(input)
}

// Token is a single lexical token captured during parsing via ParseTokens.
// Label is acorn's token-type label (e.g. "name", "num", "string", "eof", "(",
// "==", ...; matching acorn token.type.label). Value is the raw token value
// (identifier name, numeric value, string value, or nil for punctuators).
// Start/End are 0-based byte offsets.
type Token struct {
	Label string
	Value interface{}
	Start int
	End   int
}

// ParseTokens parses source (as a module, latest ECMAScript) and also returns
// every token, in source order. This is the acorn-go analogue of acorn's
// onToken (espree's tokenizer/tokens support).
func ParseTokens(input string) (v interface{}, tokens []Token, err error) {
	p := newBaseParser(input)
	p.collectTokens = true
	v, err = p.run()
	if err != nil {
		return nil, nil, err
	}
	return v, p.tokens, nil
}

// newBaseParser builds the shared parser state (extracted from Parse).
func newBaseParser(input string) *Parser {
	p := &Parser{
		input:            input,
		pos:              0,
		typ:              tEOF,
		start:            0,
		end:              0,
		lastTokStart:     0,
		lastTokEnd:       0,
		context:          []*tokContext{ctxBStat},
		exprAllowed:      true,
		inModule:         true, // sourceType: 'module'
		strict:           true, // module code is strict
		labels:           []labelInfo{},
		potentialArrowAt: -1,
	}
	p.enterScope(scopeTop)
	return p
}

// run executes the parse and converts a declared syntax error panic.
func (p *Parser) run() (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*SyntaxError_); ok {
				err = &ParseError{Msg: e.msg, Pos: e.pos}
				v = nil
				return
			}
			panic(r)
		}
	}()
	p.nextToken()
	prog := p.startNode()
	return p.parseTopLevel(prog), nil
}
