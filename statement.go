package acorn

import "strings"

// Statement parsing, closely following acorn's pp$8/pp$9.

func (p *Parser) eat(typ tokenType) bool {
	if p.typ == typ {
		p.next(false)
		return true
	}
	return false
}

func (p *Parser) isContextual(name string) bool {
	return p.typ == tName && p.value == name && !p.containsEsc
}

func (p *Parser) eatContextual(name string) bool {
	if !p.isContextual(name) {
		return false
	}
	p.next(false)
	return true
}

func (p *Parser) expectContextual(name string) {
	if !p.eatContextual(name) {
		p.unexpected(-1)
	}
}

func (p *Parser) canInsertSemicolon() bool {
	return p.typ == tEOF || p.typ == tBraceR || lineBreakBetween(p.input, p.lastTokEnd, p.start)
}

func (p *Parser) semicolon() {
	if !p.eat(tSemi) && !p.canInsertSemicolon() {
		p.unexpected(-1)
	}
}

func (p *Parser) expect(typ tokenType) {
	if !p.eat(typ) {
		p.unexpected(-1)
	}
}

func (p *Parser) afterTrailingComma(tokType tokenType, notNext bool) bool {
	if p.typ == tokType {
		if !notNext {
			p.next(false)
		}
		return true
	}
	return false
}

func (p *Parser) isDirectiveCandidate(stmt node) bool {
	if stmt == nil {
		return false
	}
	expr, ok := stmt["expression"].(node)
	if !ok {
		return false
	}
	if stmt["type"] != "ExpressionStatement" || expr["type"] != "Literal" {
		return false
	}
	if _, isStr := expr["value"].(string); !isStr {
		return false
	}
	start := stmt["start"].(int)
	if start >= len(p.input) {
		return false
	}
	return p.input[start] == '"' || p.input[start] == '\''
}

func (p *Parser) adaptDirectivePrologue(statements []node) {
	for i := 0; i < len(statements) && p.isDirectiveCandidate(statements[i]); i++ {
		expr := statements[i]["expression"].(node)
		raw := expr["raw"].(string)
		statements[i]["directive"] = raw[1 : len(raw)-1]
	}
}

// Tokenizers for `parseTopLevel` etc.

func (p *Parser) parseTopLevel(pnode node) node {
	exports := make(map[string]bool)
	if pnode["body"] == nil {
		pnode["body"] = []node{}
	}
	for p.typ != tEOF {
		stmt := p.parseStatement("", true, exports)
		pnode["body"] = append(pnode["body"].([]node), stmt)
	}
	p.adaptDirectivePrologue(pnode["body"].([]node))
	pnode["sourceType"] = p.optionsSourceType()
	p.next(false)
	p.nodeType(pnode, "Program")
	return pnode
}

func (p *Parser) optionsSourceType() string { return "module" }

func (p *Parser) nodeType(n node, typ string) node {
	n["type"] = typ
	n["end"] = p.lastTokEnd
	n["start"] = n["start"]
	return n
}

var loopLabel = labelInfo{name: "", kind: "loop"}
var switchLabel = labelInfo{name: "", kind: "switch"}

func (p *Parser) isLet(context string) bool {
	if !p.isContextual("let") {
		return false
	}
	next := p.pos
	for next < len(p.input) {
		c := p.input[next]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == 0x0b || c == 0x0c || c == 0xa0 {
			next++
			continue
		}
		if c == '/' {
			next = p.skipWSFrom(next)
			continue
		}
		break
	}
	nextCh := p.charCodeAt(next)
	if nextCh == 91 || nextCh == 92 {
		return true
	}
	if context != "" {
		return false
	}
	if nextCh == 123 || (nextCh > 0xd7ff && nextCh < 0xdc00) {
		return true
	}
	if isIdentifierStartCode(nextCh) {
		pos := next + 1
		for isIdentifierCharCode(p.charCodeAt(pos)) {
			pos++
		}
		if p.charCodeAt(pos) == 92 || p.charCodeAt(pos) > 0xd7ff && p.charCodeAt(pos) < 0xdc00 {
			return true
		}
		ident := p.input[next:pos]
		if !isInOrInstanceof(ident) {
			return true
		}
	}
	return false
}

func isInOrInstanceof(w string) bool {
	return w == "in" || w == "instanceof"
}

func (p *Parser) skipWSFrom(i int) int {
	for i < len(p.input) {
		c := p.input[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == 0x0b || c == 0x0c || c == 0xa0 {
			i++
			continue
		}
		if c == '/' && i+1 < len(p.input) {
			if p.input[i+1] == '/' {
				for i < len(p.input) && p.input[i] != '\n' {
					i++
				}
				continue
			}
			if p.input[i+1] == '*' {
				j := i + 2
				for j+1 < len(p.input) && !(p.input[j] == '*' && p.input[j+1] == '/') {
					j++
				}
				i = j + 2
				continue
			}
		}
		break
	}
	return i
}

func (p *Parser) isAsyncFunction() bool {
	if !p.isContextual("async") {
		return false
	}
	next := p.pos
	for next < len(p.input) {
		c := p.input[next]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == 0x0b || c == 0x0c || c == 0xa0 {
			next++
			continue
		}
		if c == '/' {
			next = p.skipWSFrom(next)
			continue
		}
		break
	}
	slice := p.input[p.pos:next]
	if lineBreakBetween(slice, 0, len(slice)) {
		return false
	}
	if next+8 <= len(p.input) && p.input[next:next+8] == "function" {
		if next+8 == len(p.input) {
			return true
		}
		after := p.charCodeAt(next + 8)
		if !isIdentifierCharCode(after) && !(after > 0xd7ff && after < 0xdc00) {
			return true
		}
	}
	return false
}

func (p *Parser) parseStatement(context string, topLevel bool, exports map[string]bool) node {
	starttype := p.typ
	pnode := p.startNode()
	kind := ""

	if p.isLet(context) {
		starttype = tVar
		kind = "let"
	}

	switch starttype {
	case tBreak, tContinue:
		return p.parseBreakContinueStatement(pnode, ttTable[starttype].keyword)
	case tDebugger:
		return p.parseDebuggerStatement(pnode)
	case tDo:
		return p.parseDoStatement(pnode)
	case tFor:
		return p.parseForStatement(pnode)
	case tFunction:
		return p.parseFunctionStatement(pnode, false, !(context != ""))
	case tClass:
		if context != "" {
			p.unexpected(-1)
		}
		return p.parseClass(pnode, 1)
	case tIf:
		return p.parseIfStatement(pnode)
	case tReturn:
		return p.parseReturnStatement(pnode)
	case tSwitch:
		return p.parseSwitchStatement(pnode)
	case tThrow:
		return p.parseThrowStatement(pnode)
	case tTry:
		return p.parseTryStatement(pnode)
	case tConst, tVar:
		if kind == "" {
			kind = p.value.(string)
		}
		if context != "" && kind != "var" {
			p.unexpected(-1)
		}
		return p.parseVarStatement(pnode, kind, false)
	case tWhile:
		return p.parseWhileStatement(pnode)
	case tWith:
		return p.parseWithStatement(pnode)
	case tBraceL:
		return p.parseBlock(true, pnode, false)
	case tSemi:
		return p.parseEmptyStatement(pnode)
	case tExport, tImport:
		// ecmaVersion > 10 (i.e. latest): `import(...)` and `import.meta` in
		// statement position are expressions, not import declarations
		// (acorn: next char '(' or '.' -> parseExpressionStatement).
		if starttype == tImport {
			if next := p.nextSignificantChar(); next < len(p.input) && (p.input[next] == '(' || p.input[next] == '.') {
				return p.parseExpressionStatement(pnode, p.parseExpression(false, nil))
			}
		}
		if context != "" {
			p.raise(p.start, "'import' and 'export' may only appear at the top level")
		}
		if !p.inModule {
			p.raise(p.start, "'import' and 'export' may appear only with 'sourceType: module'")
		}
		if starttype == tImport {
			return p.parseImport(pnode)
		}
		return p.parseExport(pnode, exports)
	default:
		if p.isAsyncFunction() {
			if context != "" {
				p.unexpected(-1)
			}
			p.next(false)
			return p.parseFunctionStatement(pnode, true, true)
		}
		maybeName := p.value
		expr := p.parseExpression(false, nil)
		if starttype == tName && expr["type"] == "Identifier" && p.eat(tColon) {
			return p.parseLabeledStatement(pnode, maybeName.(string), expr, context)
		}
		return p.parseExpressionStatement(pnode, expr)
	}
}

func (p *Parser) parseBreakContinueStatement(pnode node, keyword string) node {
	isBreak := keyword == "break"
	p.next(false)
	if p.eat(tSemi) || p.canInsertSemicolon() {
		pnode["label"] = nil
	} else if p.typ != tName {
		p.unexpected(-1)
	} else {
		pnode["label"] = p.parseIdent(false)
		p.semicolon()
	}
	i := 0
	for ; i < len(p.labels); i++ {
		lab := p.labels[i]
		if pnode["label"] == nil || lab.name == pnode["label"].(node)["name"] {
			if lab.kind != "" && (isBreak || lab.kind == "loop") {
				break
			}
			if pnode["label"] != nil && isBreak {
				break
			}
		}
	}
	if i == len(p.labels) {
		p.raise(pnode["start"].(int), "Unsyntactic "+keyword)
	}
	if isBreak {
		return p.finishNode(pnode, "BreakStatement")
	}
	return p.finishNode(pnode, "ContinueStatement")
}

func (p *Parser) parseDebuggerStatement(pnode node) node {
	p.next(false)
	p.semicolon()
	return p.finishNode(pnode, "DebuggerStatement")
}

func (p *Parser) parseDoStatement(pnode node) node {
	p.next(false)
	p.labels = append(p.labels, loopLabel)
	ctx := "do"
	pnode["body"] = p.parseStatement(ctx, false, nil)
	p.labels = p.labels[:len(p.labels)-1]
	p.expect(tWhile)
	pnode["test"] = p.parseParenExpression()
	p.eat(tSemi)
	return p.finishNode(pnode, "DoWhileStatement")
}

func (p *Parser) parseForStatement(pnode node) node {
	p.next(false)
	awaitAt := -1
	if p.canAwait() && p.eatContextual("await") {
		awaitAt = p.lastTokStart
	}
	p.labels = append(p.labels, loopLabel)
	p.enterScope(0)
	p.expect(tParenL)
	if p.typ == tSemi {
		if awaitAt > -1 {
			p.unexpected(awaitAt)
		}
		return p.parseFor(pnode, nil)
	}
	isLet := p.isLet("")
	if p.typ == tVar || p.typ == tConst || isLet {
		init := p.startNode()
		k := "let"
		if !isLet {
			k = p.value.(string)
		}
		p.next(false)
		p.parseVar(init, true, k)
		p.finishNode(init, "VariableDeclaration")
		return p.parseForAfterInit(pnode, init, awaitAt)
	}
	startsWithLet := p.isContextual("let")
	ctx := ""
	containsEsc := p.containsEsc
	initPos := p.start
	init := p.parseExpression(true, nil)
	isForOf := p.isContextual("of")
	if p.typ == tIn || isForOf {
		if awaitAt > -1 {
			if p.typ == tIn {
				p.unexpected(awaitAt)
			}
			pnode["await"] = true
		} else if isForOf {
			if init["start"].(int) == initPos && !containsEsc && init["type"] == "Identifier" && init["name"] == "async" {
				p.unexpected(-1)
			} else {
				pnode["await"] = false
			}
		}
		if startsWithLet && isForOf {
			p.raise(init["start"].(int), "The left-hand side of a for-of loop may not start with 'let'.")
		}
		init = p.toAssignable(init, false, nil)
		return p.parseForIn(pnode, init)
	}
	_ = ctx
	if awaitAt > -1 {
		p.unexpected(awaitAt)
	}
	return p.parseFor(pnode, init)
}

func (p *Parser) parseForAfterInit(pnode node, init node, awaitAt int) node {
	if (p.typ == tIn || p.isContextual("of")) && len(init["declarations"].([]node)) == 1 {
		if p.typ == tIn {
			if awaitAt > -1 {
				p.unexpected(awaitAt)
			}
		} else {
			pnode["await"] = awaitAt > -1
		}
		return p.parseForIn(pnode, init)
	}
	if awaitAt > -1 {
		p.unexpected(awaitAt)
	}
	return p.parseFor(pnode, init)
}

func (p *Parser) parseFunctionStatement(pnode node, isAsync, declarationPosition bool) node {
	p.next(false)
	funcStat := 1
	if !declarationPosition {
		funcStat |= 2
	}
	return p.parseFunction(pnode, funcStat, false, isAsync)
}

func (p *Parser) parseIfStatement(pnode node) node {
	p.next(false)
	pnode["test"] = p.parseParenExpression()
	ctx := "if"
	pnode["consequent"] = p.parseStatement(ctx, false, nil)
	if p.eat(tElse) {
		ctx2 := "if"
		pnode["alternate"] = p.parseStatement(ctx2, false, nil)
	} else {
		pnode["alternate"] = nil
	}
	return p.finishNode(pnode, "IfStatement")
}

func (p *Parser) parseReturnStatement(pnode node) node {
	if !p.inFunction() {
		p.raise(p.start, "'return' outside of function")
	}
	p.next(false)
	if p.eat(tSemi) || p.canInsertSemicolon() {
		pnode["argument"] = nil
	} else {
		pnode["argument"] = p.parseExpression(false, nil)
		p.semicolon()
	}
	return p.finishNode(pnode, "ReturnStatement")
}

func (p *Parser) parseSwitchStatement(pnode node) node {
	p.next(false)
	pnode["discriminant"] = p.parseParenExpression()
	pnode["cases"] = []node{}
	p.expect(tBraceL)
	p.labels = append(p.labels, switchLabel)
	p.enterScope(0)
	var cur node
	sawDefault := false
	for p.typ != tBraceR {
		if p.typ == tCase || p.typ == tDefault {
			isCase := p.typ == tCase
			if cur != nil {
				cur = p.finishNode(cur, "SwitchCase")
			}
			cur = p.startNode()
			pnode["cases"] = append(pnode["cases"].([]node), cur)
			cur["consequent"] = []node{}
			p.next(false)
			if isCase {
				cur["test"] = p.parseExpression(false, nil)
			} else {
				if sawDefault {
					p.raiseRecoverable(p.lastTokStart, "Multiple default clauses")
				}
				sawDefault = true
				cur["test"] = nil
			}
			p.expect(tColon)
		} else {
			if cur == nil {
				p.unexpected(-1)
			}
			cur["consequent"] = append(cur["consequent"].([]node), p.parseStatement("", false, nil))
		}
	}
	p.exitScope()
	if cur != nil {
		cur = p.finishNode(cur, "SwitchCase")
	}
	p.next(false)
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(pnode, "SwitchStatement")
}

func (p *Parser) parseThrowStatement(pnode node) node {
	p.next(false)
	if lineBreakBetween(p.input, p.lastTokEnd, p.start) {
		p.raise(p.lastTokEnd, "Illegal newline after throw")
	}
	pnode["argument"] = p.parseExpression(false, nil)
	p.semicolon()
	return p.finishNode(pnode, "ThrowStatement")
}

func emptyNodeArray() []node { return []node{} }

func (p *Parser) parseCatchClauseParam() node {
	param := p.parseBindingAtom()
	simple := param["type"] == "Identifier"
	if simple {
		p.enterScope(scopeSimpleCatch)
	} else {
		p.enterScope(0)
	}
	p.checkLValPattern(param, func() int {
		if simple {
			return bindSimpleCatch
		}
		return bindLexical
	}(), nil)
	p.expect(tParenR)
	return param
}

func (p *Parser) parseTryStatement(pnode node) node {
	p.next(false)
	pnode["block"] = p.parseBlock(false, p.startNode(), false)
	pnode["handler"] = nil
	if p.typ == tCatch {
		clause := p.startNode()
		p.next(false)
		if p.eat(tParenL) {
			clause["param"] = p.parseCatchClauseParam()
		} else {
			clause["param"] = nil
			p.enterScope(0)
		}
		clause["body"] = p.parseBlock(false, p.startNode(), false)
		p.exitScope()
		pnode["handler"] = p.finishNode(clause, "CatchClause")
	}
	if p.eat(tFinally) {
		pnode["finalizer"] = p.parseBlock(false, nil, false)
	} else {
		pnode["finalizer"] = nil
	}
	if pnode["handler"] == nil && pnode["finalizer"] == nil {
		p.raise(pnode["start"].(int), "Missing catch or finally clause")
	}
	return p.finishNode(pnode, "TryStatement")
}

func (p *Parser) parseVarStatement(pnode node, kind string, allowMissingInitializer bool) node {
	p.next(false)
	p.parseVar(pnode, false, kind)
	p.semicolon()
	return p.finishNode(pnode, "VariableDeclaration")
}

func (p *Parser) parseWhileStatement(pnode node) node {
	p.next(false)
	pnode["test"] = p.parseParenExpression()
	p.labels = append(p.labels, loopLabel)
	ctx := "while"
	pnode["body"] = p.parseStatement(ctx, false, nil)
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(pnode, "WhileStatement")
}

func (p *Parser) parseWithStatement(pnode node) node {
	if p.strict {
		p.raise(p.start, "'with' in strict mode")
	}
	p.next(false)
	pnode["object"] = p.parseParenExpression()
	ctx := "with"
	pnode["body"] = p.parseStatement(ctx, false, nil)
	return p.finishNode(pnode, "WithStatement")
}

func (p *Parser) parseEmptyStatement(pnode node) node {
	p.next(false)
	return p.finishNode(pnode, "EmptyStatement")
}

func (p *Parser) parseLabeledStatement(pnode node, maybeName string, expr node, context string) node {
	for i := 0; i < len(p.labels); i++ {
		if p.labels[i].name == maybeName {
			p.raise(expr["start"].(int), "Label '"+maybeName+"' is already declared")
		}
	}
	kind := ""
	p.kindFor(expr, &kind)
	for i := len(p.labels) - 1; i >= 0; i-- {
		if p.labels[i].statementStart == pnode["start"].(int) {
			p.labels[i].statementStart = p.start
			p.labels[i].kind = kind
		} else {
			break
		}
	}
	p.labels = append(p.labels, labelInfo{name: maybeName, kind: kind, statementStart: p.start})
	cs := "label"
	if context != "" {
		if !containsSubstr(context, "label") {
			cs = context + "label"
		} else {
			cs = context
		}
	}
	pnode["body"] = p.parseStatement(cs, false, nil)
	p.labels = p.labels[:len(p.labels)-1]
	pnode["label"] = expr
	return p.finishNode(pnode, "LabeledStatement")
}

func containsSubstr(s, sub string) bool {
	return strings.Contains(s, sub)
}

func (p *Parser) kindFor(expr node, kind *string) {
	// compute loop/switch kind from current token type
	info := ttTable[p.typ]
	if info.isLoop {
		*kind = "loop"
	} else if p.typ == tSwitch {
		*kind = "switch"
	} else {
		*kind = ""
	}
}

func (p *Parser) parseExpressionStatement(pnode node, expr node) node {
	pnode["expression"] = expr
	p.semicolon()
	return p.finishNode(pnode, "ExpressionStatement")
}

func (p *Parser) parseBlock(createNewLexicalScope bool, pnode node, exitStrict bool) node {
	if !createNewLexicalScope {
		// acorn default true; here caller passes false meaning no new scope
	}
	if pnode == nil {
		pnode = p.startNode()
	}
	pnode["body"] = []node{}
	p.expect(tBraceL)
	if createNewLexicalScope {
		p.enterScope(0)
	}
	for p.typ != tBraceR {
		stmt := p.parseStatement("", false, nil)
		pnode["body"] = append(pnode["body"].([]node), stmt)
	}
	if exitStrict {
		p.strict = false
	}
	p.next(false)
	if createNewLexicalScope {
		p.exitScope()
	}
	return p.finishNode(pnode, "BlockStatement")
}

func (p *Parser) parseFor(pnode node, init node) node {
	pnode["init"] = init
	p.expect(tSemi)
	if p.typ == tSemi {
		pnode["test"] = nil
	} else {
		pnode["test"] = p.parseExpression(false, nil)
	}
	p.expect(tSemi)
	if p.typ == tParenR {
		pnode["update"] = nil
	} else {
		pnode["update"] = p.parseExpression(false, nil)
	}
	p.expect(tParenR)
	ctx := "for"
	pnode["body"] = p.parseStatement(ctx, false, nil)
	p.exitScope()
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(pnode, "ForStatement")
}

func (p *Parser) parseForIn(pnode node, init node) node {
	isForIn := p.typ == tIn
	p.next(false)
	pnode["left"] = init
	if isForIn {
		pnode["right"] = p.parseExpression(false, nil)
	} else {
		pnode["right"] = p.parseMaybeAssign(false, nil)
	}
	p.expect(tParenR)
	ctx := "for"
	pnode["body"] = p.parseStatement(ctx, false, nil)
	p.exitScope()
	p.labels = p.labels[:len(p.labels)-1]
	if isForIn {
		return p.finishNode(pnode, "ForInStatement")
	}
	return p.finishNode(pnode, "ForOfStatement")
}

func (p *Parser) parseVar(pnode node, isFor bool, kind string) {
	pnode["declarations"] = []node{}
	pnode["kind"] = kind
	for {
		decl := p.startNode()
		p.parseVarID(decl, kind)
		if p.eat(tEq) {
			decl["init"] = p.parseMaybeAssign(isFor, nil)
		} else if kind == "const" && !(p.typ == tIn || p.isContextual("of")) {
			p.unexpected(-1)
		} else if decl["id"].(node)["type"] != "Identifier" && !(isFor && (p.typ == tIn || p.isContextual("of"))) {
			p.raise(p.lastTokEnd, "Complex binding patterns require an initialization value")
		} else {
			decl["init"] = nil
		}
		pnode["declarations"] = append(pnode["declarations"].([]node), p.finishNode(decl, "VariableDeclarator"))
		if !p.eat(tComma) {
			break
		}
	}
}

func (p *Parser) parseVarID(decl node, kind string) {
	if kind == "using" || kind == "await using" {
		decl["id"] = p.parseIdent(false)
	} else {
		decl["id"] = p.parseBindingAtom()
	}
	p.checkLValPattern(decl["id"].(node), func() int {
		if kind == "var" {
			return bindVar
		}
		return bindLexical
	}(), nil)
}

const (
	funcStatement        = 1
	funcHangingStatement = 2
	funcNullableID       = 4
)
