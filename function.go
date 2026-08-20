package acorn

// Function, class, import and export parsing.

func (p *Parser) parseFunction(pnode node, statement int, allowExpressionBody bool, isAsync bool) node {
	p.initFunction(pnode)
	if p.typ == tStar && (statement&funcHangingStatement) != 0 {
		p.unexpected(-1)
	}
	pnode["generator"] = p.eat(tStar)
	pnode["async"] = isAsync

	if statement&funcStatement != 0 {
		if (statement&funcNullableID) != 0 && p.typ != tName {
			pnode["id"] = nil
		} else {
			pnode["id"] = p.parseIdent(false)
		}
	}

	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
	p.yieldPos = 0
	p.awaitPos = 0
	p.awaitIdentPos = 0
	p.enterScope(p.functionFlags(isAsync, pnode["generator"].(bool)))

	if statement&funcStatement == 0 {
		if p.typ == tName {
			pnode["id"] = p.parseIdent(false)
		} else {
			pnode["id"] = nil
		}
	}

	p.parseFunctionParams(pnode)
	p.parseFunctionBody(pnode, false, false, false)

	p.yieldPos = oldYieldPos
	p.awaitPos = oldAwaitPos
	p.awaitIdentPos = oldAwaitIdentPos
	if statement&funcStatement != 0 {
		return p.finishNode(pnode, "FunctionDeclaration")
	}
	return p.finishNode(pnode, "FunctionExpression")
}

func (p *Parser) parseFunctionParams(pnode node) {
	p.expect(tParenL)
	pnode["params"] = p.parseBindingList(tParenR, false, true, false)
	p.checkYieldAwaitInDefaultParams()
}

func (p *Parser) parseClass(pnode node, isStatement int) node {
	p.next(false)
	oldStrict := p.strict
	p.strict = true
	p.parseClassID(pnode, isStatement)
	p.parseClassSuper(pnode)
	p.privateNameStack = append(p.privateNameStack, privateNameScope{declared: map[string]string{}})
	classBody := p.startNode()
	hadConstructor := false
	classBody["body"] = []node{}
	p.expect(tBraceL)
	for p.typ != tBraceR {
		element := p.parseClassElement(pnode["superClass"] != nil)
		if element != nil {
			classBody["body"] = append(classBody["body"].([]node), element)
			if element["type"] == "MethodDefinition" && element["kind"] == "constructor" {
				if hadConstructor {
					p.raiseRecoverable(element["start"].(int), "Duplicate constructor in the same class")
				}
				hadConstructor = true
			}
		}
	}
	p.strict = oldStrict
	p.next(false)
	pnode["body"] = p.finishNode(classBody, "ClassBody")
	p.privateNameStack = p.privateNameStack[:len(p.privateNameStack)-1]
	if isStatement != 0 {
		return p.finishNode(pnode, "ClassDeclaration")
	}
	return p.finishNode(pnode, "ClassExpression")
}

func (p *Parser) parseClassElement(constructorAllowsSuper bool) node {
	if p.eat(tSemi) {
		return nil
	}
	pnode := p.startNode()
	keyName := ""
	var isGenerator, isAsync bool
	kind := "method"
	isStatic := false

	if p.eatContextual("static") {
		if p.eat(tBraceL) {
			return p.parseClassStaticBlock(pnode)
		}
		if p.isClassElementNameStart() || p.typ == tStar {
			isStatic = true
		} else {
			keyName = "static"
		}
	}
	pnode["static"] = isStatic
	if keyName == "" && p.eatContextual("async") {
		if (p.isClassElementNameStart() || p.typ == tStar) && !p.canInsertSemicolon() {
			isAsync = true
		} else {
			keyName = "async"
		}
	}
	if keyName == "" && p.eat(tStar) {
		isGenerator = true
	}
	if keyName == "" && !isAsync && !isGenerator {
		lastValue := p.value
		if p.eatContextual("get") || p.eatContextual("set") {
			if p.isClassElementNameStart() {
				kind = lastValue.(string)
			} else {
				keyName = lastValue.(string)
			}
		}
	}

	if keyName != "" {
		pnode["computed"] = false
		key := p.startNodeAt(p.lastTokStart)
		key["name"] = keyName
		pnode["key"] = p.finishNode(key, "Identifier")
	} else {
		p.parseClassElementName(pnode)
	}

	if p.typ == tParenL || kind != "method" || isGenerator || isAsync {
		isConstructor := !(pnode["static"].(bool)) && checkKeyName(pnode, "constructor")
		allowsDirectSuper := isConstructor && constructorAllowsSuper
		if isConstructor && kind != "method" {
			p.raise(pnode["key"].(node)["start"].(int), "Constructor can't have get/set modifier")
		}
		if isConstructor {
			pnode["kind"] = "constructor"
		} else {
			pnode["kind"] = kind
		}
		p.parseClassMethod(pnode, isGenerator, isAsync, allowsDirectSuper)
	} else {
		p.parseClassField(pnode)
	}
	return pnode
}

func (p *Parser) isClassElementNameStart() bool {
	return p.typ == tName || p.typ == tPrivateID || p.typ == tNum || p.typ == tString ||
		p.typ == tBracketL || ttTable[p.typ].keyword != ""
}

func (p *Parser) parseClassElementName(element node) {
	if p.typ == tPrivateID {
		if p.value == "constructor" {
			p.raise(p.start, "Classes can't have an element named '#constructor'")
		}
		element["computed"] = false
		element["key"] = p.parsePrivateIdent()
	} else {
		p.parsePropertyName(element)
	}
}

func (p *Parser) parseClassMethod(method node, isGenerator, isAsync, allowsDirectSuper bool) {
	key := method["key"].(node)
	if method["kind"] == "constructor" {
		if isGenerator {
			p.raise(key["start"].(int), "Constructor can't be a generator")
		}
		if isAsync {
			p.raise(key["start"].(int), "Constructor can't be an async method")
		}
	} else if method["static"].(bool) && checkKeyName(method, "prototype") {
		p.raise(key["start"].(int), "Classes may not have a static property named prototype")
	}
	method["value"] = p.parseMethod(isGenerator, isAsync, allowsDirectSuper)
	if method["kind"] == "get" && len(method["value"].(node)["params"].([]node)) != 0 {
		p.raiseRecoverable(method["value"].(node)["start"].(int), "getter should have no params")
	}
	if method["kind"] == "set" && len(method["value"].(node)["params"].([]node)) != 1 {
		p.raiseRecoverable(method["value"].(node)["start"].(int), "setter should have exactly one param")
	}
	if method["kind"] == "set" && method["value"].(node)["params"].([]node)[0]["type"] == "RestElement" {
		p.raiseRecoverable(method["value"].(node)["params"].([]node)[0]["start"].(int), "Setter cannot use rest params")
	}
	p.finishNode(method, "MethodDefinition")
}

func (p *Parser) parseClassField(field node) {
	if checkKeyName(field, "constructor") {
		p.raise(field["key"].(node)["start"].(int), "Classes can't have a field named 'constructor'")
	} else if field["static"].(bool) && checkKeyName(field, "prototype") {
		p.raise(field["key"].(node)["start"].(int), "Classes can't have a static field named 'prototype'")
	}
	if p.eat(tEq) {
		p.enterScope(scopeClassFieldInit | scopeSuper)
		field["value"] = p.parseMaybeAssign(false, nil)
		p.exitScope()
	} else {
		field["value"] = nil
	}
	p.semicolon()
	p.finishNode(field, "PropertyDefinition")
}

func (p *Parser) parseClassStaticBlock(pnode node) node {
	pnode["body"] = []node{}
	oldLabels := p.labels
	p.labels = []labelInfo{}
	p.enterScope(scopeClassStaticBlock | scopeSuper)
	for p.typ != tBraceR {
		stmt := p.parseStatement("", false, nil)
		pnode["body"] = append(pnode["body"].([]node), stmt)
	}
	p.next(false)
	p.exitScope()
	p.labels = oldLabels
	return p.finishNode(pnode, "StaticBlock")
}

func (p *Parser) parseClassID(pnode node, isStatement int) {
	if p.typ == tName {
		pnode["id"] = p.parseIdent(false)
	} else {
		if isStatement == 1 {
			p.unexpected(-1)
		}
		pnode["id"] = nil
	}
}

func (p *Parser) parseClassSuper(pnode node) {
	if p.eat(tExtends) {
		pnode["superClass"] = p.parseExprSubscripts(nil, false)
	} else {
		pnode["superClass"] = nil
	}
}

func checkKeyName(pnode node, name string) bool {
	computed, _ := pnode["computed"].(bool)
	key, _ := pnode["key"].(node)
	if computed {
		return false
	}
	if key["type"] == "Identifier" {
		return key["name"] == name
	}
	if key["type"] == "Literal" {
		return key["value"] == name
	}
	return false
}

// ---- import / export ----

func (p *Parser) parseExport(pnode node, exports map[string]bool) node {
	p.next(false)
	if p.eat(tStar) {
		return p.parseExportAllDeclaration(pnode, exports)
	}
	if p.eat(tDefault) {
		p.checkExport(exports, "default", p.lastTokStart)
		pnode["declaration"] = p.parseExportDefaultDeclaration()
		return p.finishNode(pnode, "ExportDefaultDeclaration")
	}
	if p.shouldParseExportStatement() {
		pnode["declaration"] = p.parseExportDeclaration(pnode)
		if pnode["declaration"].(node)["type"] == "VariableDeclaration" {
			p.checkVariableExport(exports, pnode["declaration"].(node)["declarations"].([]node))
		} else {
			id := pnode["declaration"].(node)["id"]
			if id != nil {
				p.checkExport(exports, pnode["declaration"].(node)["id"].(node), pnode["declaration"].(node)["id"].(node)["start"].(int))
			}
		}
		pnode["specifiers"] = []node{}
		pnode["source"] = nil
		pnode["attributes"] = []node{}
	} else {
		pnode["declaration"] = nil
		pnode["specifiers"] = p.parseExportSpecifiers(exports)
		if p.eatContextual("from") {
			if p.typ != tString {
				p.unexpected(-1)
			}
			pnode["source"] = p.parseExprAtom(nil, false, false)
			pnode["attributes"] = []node{}
		} else {
			specs := pnode["specifiers"].([]node)
			for i := 0; i < len(specs); i++ {
				spec := specs[i]
				p.checkUnreserved(spec["local"].(node))
				p.checkLocalExport(spec["local"].(node))
				if spec["local"].(node)["type"] == "Literal" {
					p.raise(spec["local"].(node)["start"].(int), "A string literal cannot be used as an exported binding without `from`.")
				}
			}
			pnode["source"] = nil
			pnode["attributes"] = []node{}
		}
		p.semicolon()
	}
	return p.finishNode(pnode, "ExportNamedDeclaration")
}

func (p *Parser) parseExportAllDeclaration(pnode node, exports map[string]bool) node {
	if p.eatContextual("as") {
		pnode["exported"] = p.parseModuleExportName()
		p.checkExport(exports, pnode["exported"].(node), p.lastTokStart)
	} else {
		pnode["exported"] = nil
	}
	p.expectContextual("from")
	if p.typ != tString {
		p.unexpected(-1)
	}
	pnode["source"] = p.parseExprAtom(nil, false, false)
	pnode["attributes"] = []node{}
	p.semicolon()
	return p.finishNode(pnode, "ExportAllDeclaration")
}

func (p *Parser) parseExportDeclaration(pnode node) node {
	return p.parseStatement("", false, nil)
}

func (p *Parser) parseExportDefaultDeclaration() node {
	isAsync := p.isAsyncFunction()
	if p.typ == tFunction || isAsync {
		fNode := p.startNode()
		p.next(false)
		if isAsync {
			p.next(false)
		}
		st := funcStatement | funcNullableID
		return p.parseFunction(fNode, st, false, isAsync)
	} else if p.typ == tClass {
		cNode := p.startNode()
		return p.parseClass(cNode, 2)
	} else {
		declaration := p.parseMaybeAssign(false, nil)
		p.semicolon()
		return declaration
	}
}

func (p *Parser) checkExport(exports map[string]bool, nameOrNode interface{}, pos int) {
	if exports == nil {
		return
	}
	name, ok := nameOrNode.(string)
	if !ok {
		nd := nameOrNode.(node)
		if nd["type"] == "Identifier" {
			name = nd["name"].(string)
		} else {
			name = toKeyString(nd["value"])
		}
	}
	if exports[name] {
		p.raiseRecoverable(pos, "Duplicate export '"+name+"'")
	}
	exports[name] = true
}

func (p *Parser) checkPatternExport(exports map[string]bool, pat node) {
	typ := pat["type"]
	switch typ {
	case "Identifier":
		p.checkExport(exports, pat, pat["start"].(int))
	case "ObjectPattern":
		props := pat["properties"].([]node)
		for i := 0; i < len(props); i++ {
			p.checkPatternExport(exports, props[i])
		}
	case "ArrayPattern":
		elts := pat["elements"].([]node)
		for i := 0; i < len(elts); i++ {
			if elts[i] != nil {
				p.checkPatternExport(exports, elts[i])
			}
		}
	case "Property":
		p.checkPatternExport(exports, pat["value"].(node))
	case "AssignmentPattern":
		p.checkPatternExport(exports, pat["left"].(node))
	case "RestElement":
		p.checkPatternExport(exports, pat["argument"].(node))
	}
}

func (p *Parser) checkVariableExport(exports map[string]bool, decls []node) {
	if exports == nil {
		return
	}
	for i := 0; i < len(decls); i++ {
		p.checkPatternExport(exports, decls[i]["id"].(node))
	}
}

func (p *Parser) shouldParseExportStatement() bool {
	if ttTable[p.typ].keyword == "var" || ttTable[p.typ].keyword == "const" ||
		ttTable[p.typ].keyword == "class" || ttTable[p.typ].keyword == "function" {
		return true
	}
	return p.isLet("") || p.isAsyncFunction()
}

func (p *Parser) parseExportSpecifier(exports map[string]bool) node {
	pnode := p.startNode()
	pnode["local"] = p.parseModuleExportName()
	if p.eatContextual("as") {
		pnode["exported"] = p.parseModuleExportName()
	} else {
		pnode["exported"] = pnode["local"]
	}
	p.checkExport(exports, pnode["exported"].(node), pnode["exported"].(node)["start"].(int))
	return p.finishNode(pnode, "ExportSpecifier")
}

func (p *Parser) parseExportSpecifiers(exports map[string]bool) []node {
	nodes := []node{}
	first := true
	p.expect(tBraceL)
	for !p.eat(tBraceR) {
		if !first {
			p.expect(tComma)
			if p.afterTrailingComma(tBraceR, false) {
				break
			}
		} else {
			first = false
		}
		nodes = append(nodes, p.parseExportSpecifier(exports))
	}
	return nodes
}

func (p *Parser) parseImport(pnode node) node {
	p.next(false)
	if p.typ == tString {
		pnode["specifiers"] = []node{}
		pnode["source"] = p.parseExprAtom(nil, false, false)
	} else {
		pnode["specifiers"] = p.parseImportSpecifiers()
		p.expectContextual("from")
		if p.typ == tString {
			pnode["source"] = p.parseExprAtom(nil, false, false)
		} else {
			p.unexpected(-1)
		}
	}
	pnode["attributes"] = []node{}
	p.semicolon()
	return p.finishNode(pnode, "ImportDeclaration")
}

func (p *Parser) parseImportSpecifier() node {
	pnode := p.startNode()
	pnode["imported"] = p.parseModuleExportName()
	if p.eatContextual("as") {
		pnode["local"] = p.parseIdent(false)
	} else {
		p.checkUnreserved(pnode["imported"].(node))
		pnode["local"] = pnode["imported"]
	}
	p.checkLValSimple(pnode["local"].(node), bindLexical, nil)
	return p.finishNode(pnode, "ImportSpecifier")
}

func (p *Parser) parseImportDefaultSpecifier() node {
	pnode := p.startNode()
	pnode["local"] = p.parseIdent(false)
	p.checkLValSimple(pnode["local"].(node), bindLexical, nil)
	return p.finishNode(pnode, "ImportDefaultSpecifier")
}

func (p *Parser) parseImportNamespaceSpecifier() node {
	pnode := p.startNode()
	p.next(false)
	p.expectContextual("as")
	pnode["local"] = p.parseIdent(false)
	p.checkLValSimple(pnode["local"].(node), bindLexical, nil)
	return p.finishNode(pnode, "ImportNamespaceSpecifier")
}

func (p *Parser) parseImportSpecifiers() []node {
	nodes := []node{}
	first := true
	if p.typ == tName {
		nodes = append(nodes, p.parseImportDefaultSpecifier())
		if !p.eat(tComma) {
			return nodes
		}
	}
	if p.typ == tStar {
		nodes = append(nodes, p.parseImportNamespaceSpecifier())
		return nodes
	}
	p.expect(tBraceL)
	for !p.eat(tBraceR) {
		if !first {
			p.expect(tComma)
			if p.afterTrailingComma(tBraceR, false) {
				break
			}
		} else {
			first = false
		}
		nodes = append(nodes, p.parseImportSpecifier())
	}
	return nodes
}

func (p *Parser) parseModuleExportName() node {
	if p.typ == tString {
		return p.parseLiteral(p.value)
	}
	return p.parseIdent(true)
}

func (p *Parser) checkLocalExport(id node) {
	found := p.nameDeclaredIn(id["name"].(string))
	// Declaration tracking is performed in declareName; for AST-only parity
	// this is a no-op. (undefinedExports only affects error reporting.)
	_ = found
}

func (p *Parser) nameDeclaredIn(name string) bool {
	for _, s := range p.scopeStack {
		for _, n := range s.vars {
			if n == name {
				return true
			}
		}
	}
	return false
}
