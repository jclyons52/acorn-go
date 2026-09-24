package acorn

import "strings"

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

// ParseAll parses source and captures both comments and tokens in a single
// pass (the acorn-go analogue of acorn's onComment + onToken fired together,
// as espree does). Returns the AST, comments, tokens, and any error.
func ParseAll(input string) (v interface{}, comments []Comment, tokens []Token, err error) {
	p := newBaseParser(input)
	p.collectComments = true
	p.collectTokens = true
	v, err = p.run()
	if err != nil {
		return nil, nil, nil, err
	}
	return v, p.comments, p.tokens, nil
}

// ParseOptions are the acorn options this port honours. acorn's own option bag
// is larger; what the ESLint chain needs is sourceType, which decides module
// versus sloppy script semantics (and therefore whether import/export, `with`,
// octal literals and duplicate parameters are legal).
type ParseOptions struct {
	// SourceType is "module" (the default) or "script".
	SourceType string
	// AllowHashBang mirrors acorn's option: a `#!` line at the very start of the
	// file is skipped as a line comment. acorn defaults it to `ecmaVersion >= 14`
	// (ES2023) and this port targets the latest grammar, so nil means enabled.
	// ESLint never depends on it — the Linter rewrites the shebang into a line
	// comment before parsing — but a direct acorn-go/espree-go caller parsing an
	// npm bin script does.
	AllowHashBang *bool
	// AllowReturnOutsideFunction mirrors acorn's option of the same name: a
	// top-level `return` is accepted. espree sets it when
	// parserOptions.ecmaFeatures.globalReturn is on (which `env: node` turns on),
	// i.e. for CommonJS-style sources.
	AllowReturnOutsideFunction bool
}

// hashBangAllowed resolves acorn's `allowHashBang == null ? ecmaVersion >= 14 :
// allowHashBang` default for this port (latest grammar ⇒ enabled).
func hashBangAllowed(opts ParseOptions) bool {
	if opts.AllowHashBang != nil {
		return *opts.AllowHashBang
	}
	return true
}

// ParseAllWithOptions is ParseAll with acorn options.
func ParseAllWithOptions(input string, opts ParseOptions) (v interface{}, comments []Comment, tokens []Token, err error) {
	p := newParserWithOptions(input, opts)
	p.collectComments = true
	p.collectTokens = true
	v, err = p.run()
	if err != nil {
		return nil, nil, nil, err
	}
	return v, p.comments, p.tokens, nil
}

// newBaseParser builds the shared parser state (extracted from Parse).
func newBaseParser(input string) *Parser {
	return newParserWithOptions(input, ParseOptions{SourceType: "module"})
}

// newParserWithOptions builds the parser state for a sourceType.
func newParserWithOptions(input string, opts ParseOptions) *Parser {
	sourceType := opts.SourceType
	if sourceType != "script" {
		sourceType = "module"
	}
	inModule := sourceType == "module"
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
		sourceType:       sourceType,
		inModule:         inModule,
		labels:           []labelInfo{},
		potentialArrowAt: -1,
	}
	// acorn: this.strict = this.inModule || options.strict === true ||
	// this.strictDirective(this.pos)
	p.strict = inModule || p.strictDirective(0)

	// acorn, in this order:
	//   if (this.pos === 0 && options.allowHashBang && this.input.slice(0, 2) === "#!")
	//     { this.skipLineComment(2); }
	// The order matters: strictness was decided from offset 0 just above, so acorn
	// reads `#!...` + `"use strict"` as sloppy code (the directive is no longer at
	// offset 0). Only a real shebang — at byte 0, after BOM stripping — is skipped.
	// The skip itself happens in run(), once the comment collector is configured:
	// acorn fires onComment from inside the constructor because its options bag is
	// already complete there, while this port enables collection after construction
	// (ParseAllWithOptions), which would drop the hashbang from the comment stream.
	p.hashBang = p.pos == 0 && hashBangAllowed(opts) && strings.HasPrefix(input, "#!")
	p.allowReturnOutsideFunction = opts.AllowReturnOutsideFunction
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
	if p.hashBang {
		// Same effect as acorn's constructor-time skipLineComment(2), but with the
		// comment recorded (espree reports it as a "Hashbang" comment).
		p.skipLineComment(2)
	}
	p.nextToken()
	prog := p.startNode()
	return p.parseTopLevel(prog), nil
}

// LexAt lexes the single token that starts at (or after) offset and returns its
// span, or an error if the tokenizer rejects the input there.
//
// This exists for espree's overridden unexpected(): espree re-tokenizes at the
// failure position and appends the offending token's source text to the
// message ("Unexpected token ;"), which raw acorn does not do. Re-tokenizing
// works because the failure position is always a token boundary acorn had
// already scanned.
func LexAt(input string, offset int) (start, end int, err error) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(input) {
		offset = len(input)
	}
	p := newBaseParser(input)
	p.pos = offset
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*SyntaxError_); ok {
				start, end, err = offset, offset, &ParseError{Msg: e.msg, Pos: e.pos}
				return
			}
			panic(r)
		}
	}()
	p.nextToken()
	return p.start, p.end, nil
}
