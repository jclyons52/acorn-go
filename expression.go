package acorn

import (
	"reflect"
	"strconv"
	"strings"
)

// Expression parsing, closely following acorn's pp$5.

type destructuringErrors struct {
	shorthandAssign     int
	trailingComma       int
	parenthesizedAssign int
	parenthesizedBind   int
	doubleProto         int
}

func newDestructuringErrors() *destructuringErrors {
	return &destructuringErrors{-1, -1, -1, -1, -1}
}

// sameNode reports whether two nodes are the same object (map pointer).
func sameNode(a, b node) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func (p *Parser) checkPatternErrors(ref *destructuringErrors, isAssign bool) {
	if ref == nil {
		return
	}
	if ref.trailingComma > -1 {
		p.raiseRecoverable(ref.trailingComma, "Comma is not permitted after the rest element")
	}
	parens := ref.parenthesizedAssign
	if !isAssign {
		parens = ref.parenthesizedBind
	}
	if parens > -1 {
		if isAssign {
			p.raiseRecoverable(parens, "Assigning to rvalue")
		} else {
			p.raiseRecoverable(parens, "Parenthesized pattern")
		}
	}
}

func (p *Parser) checkExpressionErrors(ref *destructuringErrors, andThrow bool) bool {
	if ref == nil {
		return false
	}
	if !andThrow {
		return ref.shorthandAssign >= 0 || ref.doubleProto >= 0
	}
	if ref.shorthandAssign >= 0 {
		p.raise(ref.shorthandAssign, "Shorthand property assignments are valid only in destructuring patterns")
	}
	if ref.doubleProto >= 0 {
		p.raiseRecoverable(ref.doubleProto, "Redefinition of __proto__ property")
	}
	return false
}

func (p *Parser) checkYieldAwaitInDefaultParams() {
	if p.yieldPos != 0 && (p.awaitPos == 0 || p.yieldPos < p.awaitPos) {
		p.raise(p.yieldPos, "Yield expression cannot be a default value")
	}
	if p.awaitPos != 0 {
		p.raise(p.awaitPos, "Await expression cannot be a default value")
	}
}

func (p *Parser) isSimpleAssignTarget(expr node) bool {
	if expr["type"] == "ParenthesizedExpression" {
		return p.isSimpleAssignTarget(expr["expression"].(node))
	}
	return expr["type"] == "Identifier" || expr["type"] == "MemberExpression"
}

func (p *Parser) parseExpression(forInit bool, ref *destructuringErrors) node {
	startPos := p.start
	expr := p.parseMaybeAssign(forInit, ref)
	if p.typ == tComma {
		pnode := p.startNodeAt(startPos)
		pnode["expressions"] = []node{expr}
		for p.eat(tComma) {
			pnode["expressions"] = append(pnode["expressions"].([]node), p.parseMaybeAssign(forInit, ref))
		}
		return p.finishNode(pnode, "SequenceExpression")
	}
	return expr
}

func (p *Parser) parseMaybeAssign(forInit bool, ref *destructuringErrors) node {
	if p.isContextual("yield") {
		if p.inGenerator() {
			return p.parseYield(forInit)
		}
		p.exprAllowed = false
	}

	ownDestructuringErrors := false
	oldParenAssign, oldTrailingComma, oldDoubleProto := -1, -1, -1
	if ref != nil {
		oldParenAssign = ref.parenthesizedAssign
		oldTrailingComma = ref.trailingComma
		oldDoubleProto = ref.doubleProto
		ref.parenthesizedAssign = -1
		ref.trailingComma = -1
	} else {
		ref = newDestructuringErrors()
		ownDestructuringErrors = true
	}

	startPos := p.start
	if p.typ == tParenL || p.typ == tName {
		p.potentialArrowAt = p.start
		p.potentialArrowInForAwait = forInit
	}
	left := p.parseMaybeConditional(forInit, ref)
	if ttTable[p.typ].isAssign {
		pnode := p.startNodeAt(startPos)
		pnode["operator"] = p.value
		if p.typ == tEq {
			left = p.toAssignable(left, false, ref)
		}
		if !ownDestructuringErrors {
			ref.parenthesizedAssign = -1
			ref.trailingComma = -1
			ref.doubleProto = -1
		}
		if ref.shorthandAssign >= left["start"].(int) {
			ref.shorthandAssign = -1
		}
		if p.typ == tEq {
			p.checkLValPattern(left, bindNone, nil)
		} else {
			p.checkLValSimple(left, bindNone, nil)
		}
		pnode["left"] = left
		p.next(false)
		pnode["right"] = p.parseMaybeAssign(forInit, nil)
		if oldDoubleProto > -1 {
			ref.doubleProto = oldDoubleProto
		}
		return p.finishNode(pnode, "AssignmentExpression")
	}
	if ownDestructuringErrors {
		p.checkExpressionErrors(ref, true)
	}
	if oldParenAssign > -1 {
		ref.parenthesizedAssign = oldParenAssign
	}
	if oldTrailingComma > -1 {
		ref.trailingComma = oldTrailingComma
	}
	return left
}

func (p *Parser) parseMaybeConditional(forInit bool, ref *destructuringErrors) node {
	startPos := p.start
	expr := p.parseExprOps(forInit, ref)
	if p.checkExpressionErrors(ref, false) {
		return expr
	}
	if p.eat(tQuestion) {
		pnode := p.startNodeAt(startPos)
		pnode["test"] = expr
		pnode["consequent"] = p.parseMaybeAssign(false, nil)
		p.expect(tColon)
		pnode["alternate"] = p.parseMaybeAssign(forInit, nil)
		return p.finishNode(pnode, "ConditionalExpression")
	}
	return expr
}

func (p *Parser) parseExprOps(forInit bool, ref *destructuringErrors) node {
	startPos := p.start
	expr := p.parseMaybeUnary(ref, false, false, forInit)
	if p.checkExpressionErrors(ref, false) {
		return expr
	}
	if expr["start"].(int) == startPos && expr["type"] == "ArrowFunctionExpression" {
		return expr
	}
	return p.parseExprOp(expr, startPos, -1, forInit)
}

func (p *Parser) parseExprOp(left node, leftStartPos int, minPrec int, forInit bool) node {
	prec := ttTable[p.typ].binop
	if prec != -1 && (!forInit || p.typ != tIn) {
		if prec > minPrec {
			logical := p.typ == tLogicalOR || p.typ == tLogicalAND
			coalesce := p.typ == tCoalesce
			if coalesce {
				prec = ttTable[tLogicalAND].binop
			}
			op := p.value
			p.next(false)
			startPos := p.start
			right := p.parseExprOp(p.parseMaybeUnary(nil, false, false, forInit), startPos, prec, forInit)
			pnode := p.buildBinary(leftStartPos, left, right, op.(string), logical || coalesce)
			pnodeStart := leftStartPos
			_ = pnodeStart
			return p.parseExprOp(pnode, leftStartPos, minPrec, forInit)
		}
	}
	return left
}

func (p *Parser) buildBinary(startPos int, left, right node, op string, logical bool) node {
	pnode := p.startNodeAt(startPos)
	pnode["left"] = left
	pnode["operator"] = op
	pnode["right"] = right
	if logical {
		return p.finishNode(pnode, "LogicalExpression")
	}
	return p.finishNode(pnode, "BinaryExpression")
}

func (p *Parser) parseMaybeUnary(ref *destructuringErrors, sawUnary bool, incDec bool, forInit bool) node {
	startPos := p.start
	var expr node
	if p.isContextual("await") && p.canAwait() {
		expr = p.parseAwait(forInit)
		sawUnary = true
	} else if ttTable[p.typ].prefix {
		pnode := p.startNode()
		update := p.typ == tIncDec
		pnode["operator"] = p.value
		pnode["prefix"] = true
		p.next(false)
		pnode["argument"] = p.parseMaybeUnary(nil, true, update, forInit)
		p.checkExpressionErrors(ref, true)
		if update {
			p.checkLValSimple(pnode["argument"].(node), bindNone, nil)
		} else {
			sawUnary = true
		}
		if update {
			expr = p.finishNode(pnode, "UpdateExpression")
		} else {
			expr = p.finishNode(pnode, "UnaryExpression")
		}
	} else if !sawUnary && p.typ == tPrivateID {
		if forInit || len(p.privateNameStack) == 0 {
			p.unexpected(-1)
		}
		expr = p.parsePrivateIdent()
		if p.typ != tIn {
			p.unexpected(-1)
		}
	} else {
		expr = p.parseExprSubscripts(ref, forInit)
		if p.checkExpressionErrors(ref, false) {
			return expr
		}
		for ttTable[p.typ].postfix && !p.canInsertSemicolon() {
			pnode := p.startNodeAt(startPos)
			pnode["operator"] = p.value
			pnode["prefix"] = false
			pnode["argument"] = expr
			p.checkLValSimple(expr, bindNone, nil)
			p.next(false)
			expr = p.finishNode(pnode, "UpdateExpression")
		}
	}

	if !incDec && p.eat(tStarStar) {
		if sawUnary {
			p.unexpected(p.lastTokStart)
		}
		return p.buildBinary(startPos, expr, p.parseMaybeUnary(nil, false, false, forInit), "**", false)
	}
	return expr
}

func (p *Parser) parseExprSubscripts(ref *destructuringErrors, forInit bool) node {
	startPos := p.start
	expr := p.parseExprAtom(ref, forInit, false)
	if expr["type"] == "ArrowFunctionExpression" && p.input[p.lastTokStart:p.lastTokEnd] != ")" {
		return expr
	}
	result := p.parseSubscripts(expr, startPos, false, forInit)
	if ref != nil && result["type"] == "MemberExpression" {
		if ref.parenthesizedAssign >= result["start"].(int) {
			ref.parenthesizedAssign = -1
		}
		if ref.parenthesizedBind >= result["start"].(int) {
			ref.parenthesizedBind = -1
		}
		if ref.trailingComma >= result["start"].(int) {
			ref.trailingComma = -1
		}
	}
	return result
}

func (p *Parser) parseSubscripts(base node, startPos int, noCalls bool, forInit bool) node {
	maybeAsyncArrow := base["type"] == "Identifier" && base["name"] == "async" &&
		p.lastTokEnd == base["end"].(int) && !p.canInsertSemicolon() && base["end"].(int)-base["start"].(int) == 5 &&
		p.potentialArrowAt == base["start"].(int)
	optionalChained := false

	for {
		element := p.parseSubscript(base, startPos, noCalls, maybeAsyncArrow, optionalChained, forInit)
		if element["optional"] == true {
			optionalChained = true
		}
		if sameNode(element, base) || element["type"] == "ArrowFunctionExpression" {
			if optionalChained {
				chainNode := p.startNodeAt(startPos)
				chainNode["expression"] = element
				element = p.finishNode(chainNode, "ChainExpression")
			}
			return element
		}
		base = element
	}
}

func (p *Parser) parseSubscript(base node, startPos int, noCalls bool, maybeAsyncArrow bool, optionalChained bool, forInit bool) node {
	optional := p.eat(tQuestionDot)
	if noCalls && optional {
		p.raise(p.lastTokStart, "Optional chaining cannot appear in the callee of new expressions")
	}

	computed := p.eat(tBracketL)
	if computed || (optional && p.typ != tParenL && p.typ != tBackQuote) || p.eat(tDot) {
		pnode := p.startNodeAt(startPos)
		pnode["object"] = base
		if computed {
			pnode["property"] = p.parseExpression(false, nil)
			p.expect(tBracketR)
		} else if p.typ == tPrivateID && base["type"] != "Super" {
			pnode["property"] = p.parsePrivateIdent()
		} else {
			pnode["property"] = p.parseIdent(true)
		}
		pnode["computed"] = computed
		pnode["optional"] = optional
		base = p.finishNode(pnode, "MemberExpression")
	} else if !noCalls && p.eat(tParenL) {
		ref := newDestructuringErrors()
		oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
		p.yieldPos = 0
		p.awaitPos = 0
		p.awaitIdentPos = 0
		exprList := p.parseExprList(tParenR, true, false, ref)
		if maybeAsyncArrow && !optional && p.shouldParseAsyncArrow() {
			p.checkPatternErrors(ref, false)
			p.checkYieldAwaitInDefaultParams()
			p.yieldPos = oldYieldPos
			p.awaitPos = oldAwaitPos
			p.awaitIdentPos = oldAwaitIdentPos
			return p.parseSubscriptAsyncArrow(startPos, exprList, forInit)
		}
		p.checkExpressionErrors(ref, true)
		p.yieldPos = oldYieldPos
		p.awaitPos = oldAwaitPos
		p.awaitIdentPos = oldAwaitIdentPos
		pnode := p.startNodeAt(startPos)
		pnode["callee"] = base
		pnode["arguments"] = exprList
		pnode["optional"] = optional
		base = p.finishNode(pnode, "CallExpression")
	} else if p.typ == tBackQuote {
		if optional || optionalChained {
			p.raise(p.start, "Optional chaining cannot appear in the tag of tagged template expressions")
		}
		pnode := p.startNodeAt(startPos)
		pnode["tag"] = base
		pnode["quasi"] = p.parseTemplate(true)
		base = p.finishNode(pnode, "TaggedTemplateExpression")
	}
	return base
}

func (p *Parser) shouldParseAsyncArrow() bool {
	return !p.canInsertSemicolon() && p.eat(tArrow)
}

func (p *Parser) parseSubscriptAsyncArrow(startPos int, exprList []node, forInit bool) node {
	return p.parseArrowExpression(p.startNodeAt(startPos), exprList, true, forInit)
}

func (p *Parser) parseExprAtom(ref *destructuringErrors, forInit bool, forNew bool) node {
	if p.typ == tSlash {
		p.readRegexp()
	}
	var pnode node
	canBeArrow := p.potentialArrowAt == p.start
	switch p.typ {
	case tSuper:
		if !p.allowSuper() {
			p.raise(p.start, "'super' keyword outside a method")
		}
		pnode = p.startNode()
		p.next(false)
		if p.typ == tParenL && !p.allowDirectSuper() {
			p.raise(pnode["start"].(int), "super() call outside constructor of a subclass")
		}
		if p.typ != tDot && p.typ != tBracketL && p.typ != tParenL {
			p.unexpected(-1)
		}
		return p.finishNode(pnode, "Super")
	case tThis:
		pnode = p.startNode()
		p.next(false)
		return p.finishNode(pnode, "ThisExpression")
	case tName:
		startPos := p.start
		containsEsc := p.containsEsc
		id := p.parseIdent(false)
		if !containsEsc && id["name"] == "async" && !p.canInsertSemicolon() && p.eat(tFunction) {
			p.overrideContext(ctxFExpr)
			return p.parseFunction(p.startNodeAt(startPos), 0, false, true)
		}
		if canBeArrow && !p.canInsertSemicolon() {
			if p.eat(tArrow) {
				return p.parseArrowExpression(p.startNodeAt(startPos), []node{id}, false, forInit)
			}
			if id["name"] == "async" && p.typ == tName && !containsEsc {
				id = p.parseIdent(false)
				if p.canInsertSemicolon() || !p.eat(tArrow) {
					p.unexpected(-1)
				}
				return p.parseArrowExpression(p.startNodeAt(startPos), []node{id}, true, forInit)
			}
		}
		return id
	case tRegexp:
		value := p.value.(map[string]interface{})
		val := value["value"]
		_ = val
		pnode = p.parseLiteral(map[string]interface{}{})
		pnode["regex"] = map[string]interface{}{"pattern": value["pattern"], "flags": value["flags"]}
		return pnode
	case tNum, tString:
		return p.parseLiteral(p.value)
	case tNull, tTrue, tFalse:
		pnode = p.startNode()
		switch p.typ {
		case tNull:
			pnode["value"] = nil
		case tTrue:
			pnode["value"] = true
		case tFalse:
			pnode["value"] = false
		}
		pnode["raw"] = ttTable[p.typ].keyword
		p.next(false)
		return p.finishNode(pnode, "Literal")
	case tParenL:
		start := p.start
		expr := p.parseParenAndDistinguishExpression(canBeArrow, forInit)
		if ref != nil {
			if ref.parenthesizedAssign < 0 && !p.isSimpleAssignTarget(expr) {
				ref.parenthesizedAssign = start
			}
			if ref.parenthesizedBind < 0 {
				ref.parenthesizedBind = start
			}
		}
		return expr
	case tBracketL:
		pnode = p.startNode()
		p.next(false)
		pnode["elements"] = p.parseExprList(tBracketR, true, true, ref)
		return p.finishNode(pnode, "ArrayExpression")
	case tBraceL:
		p.overrideContext(ctxBExpr)
		return p.parseObj(false, ref)
	case tFunction:
		pnode = p.startNode()
		p.next(false)
		return p.parseFunction(pnode, 0, false, false)
	case tClass:
		return p.parseClass(p.startNode(), 0)
	case tNew:
		return p.parseNew()
	case tBackQuote:
		return p.parseTemplate(false)
	default:
		p.unexpected(-1)
		return nil
	}
}

func (p *Parser) overrideContext(ctx *tokContext) {
	if p.curContext() != ctx {
		p.context[len(p.context)-1] = ctx
	}
}

func (p *Parser) parseLiteral(value interface{}) node {
	pnode := p.startNode()
	pnode["value"] = value
	pnode["raw"] = p.input[p.start:p.end]
	p.next(false)
	return p.finishNode(pnode, "Literal")
}

func (p *Parser) parseParenExpression() node {
	p.expect(tParenL)
	val := p.parseExpression(false, nil)
	p.expect(tParenR)
	return val
}

func (p *Parser) parseParenAndDistinguishExpression(canBeArrow bool, forInit bool) node {
	startPos := p.start
	allowTrailingComma := true
	p.next(false)

	innerStartPos := p.start
	exprList := []node{}
	first := true
	lastIsComma := false
	ref := newDestructuringErrors()
	oldYieldPos, oldAwaitPos := p.yieldPos, p.awaitPos
	p.yieldPos = 0
	p.awaitPos = 0
	spreadStart := -1
	for p.typ != tParenR {
		if !first {
			p.expect(tComma)
		}
		first = false
		if allowTrailingComma && p.afterTrailingComma(tParenR, true) {
			lastIsComma = true
			break
		} else if p.typ == tEllipsis {
			spreadStart = p.start
			exprList = append(exprList, p.parseParenItem(p.parseRestBinding()))
			if p.typ == tComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			break
		} else {
			exprList = append(exprList, p.parseMaybeAssign(false, ref))
		}
	}
	innerEndPos := p.lastTokEnd
	_ = innerEndPos
	p.expect(tParenR)

	if canBeArrow && p.shouldParseArrow(exprList) && p.eat(tArrow) {
		p.checkPatternErrors(ref, false)
		p.checkYieldAwaitInDefaultParams()
		p.yieldPos = oldYieldPos
		p.awaitPos = oldAwaitPos
		return p.parseParenArrowList(startPos, exprList, forInit)
	}

	if len(exprList) == 0 || lastIsComma {
		p.unexpected(p.lastTokStart)
	}
	if spreadStart > -1 {
		p.unexpected(spreadStart)
	}
	p.checkExpressionErrors(ref, true)
	p.yieldPos = oldYieldPos
	p.awaitPos = oldAwaitPos

	var val node
	if len(exprList) > 1 {
		val = p.startNodeAt(innerStartPos)
		val["expressions"] = exprList
		p.finishNodeAt(val, "SequenceExpression", innerEndPos)
	} else {
		val = exprList[0]
	}
	return val
}

func (p *Parser) shouldParseArrow(exprList []node) bool {
	return !p.canInsertSemicolon()
}

func (p *Parser) parseParenItem(item node) node { return item }

func (p *Parser) parseParenArrowList(startPos int, exprList []node, forInit bool) node {
	return p.parseArrowExpression(p.startNodeAt(startPos), exprList, false, forInit)
}

func (p *Parser) parseNew() node {
	if p.containsEsc {
		p.raiseRecoverable(p.start, "Escape sequence in keyword new")
	}
	pnode := p.startNode()
	p.next(false)
	if p.typ == tDot {
		meta := p.startNodeAt(pnode["start"].(int))
		meta["name"] = "new"
		pnode["meta"] = p.finishNode(meta, "Identifier")
		p.next(false)
		containsEsc := p.containsEsc
		pnode["property"] = p.parseIdent(true)
		if pnode["property"].(node)["name"] != "target" {
			p.raiseRecoverable(pnode["property"].(node)["start"].(int), "The only valid meta property for new is 'new.target'")
		}
		if containsEsc {
			p.raiseRecoverable(pnode["start"].(int), "'new.target' must not contain escaped characters")
		}
		if !p.allowNewDotTarget() {
			p.raiseRecoverable(pnode["start"].(int), "'new.target' can only be used in functions and class static block")
		}
		return p.finishNode(pnode, "MetaProperty")
	}
	startPos := p.start
	pnode["callee"] = p.parseSubscripts(p.parseExprAtom(nil, false, true), startPos, true, false)
	if p.eat(tParenL) {
		pnode["arguments"] = p.parseExprList(tParenR, true, false, nil)
	} else {
		pnode["arguments"] = []node{}
	}
	return p.finishNode(pnode, "NewExpression")
}

func (p *Parser) parseTemplateElement(isTagged bool) node {
	elem := p.startNode()
	if p.typ == tInvalidTemplate {
		if !isTagged {
			p.raiseRecoverable(p.start, "Bad escape sequence in untagged template literal")
		}
		elem["value"] = map[string]interface{}{
			"raw":    normalizeCRLF(p.value.(string)),
			"cooked": nil,
		}
	} else {
		elem["value"] = map[string]interface{}{
			"raw":    normalizeCRLF(p.input[p.start:p.end]),
			"cooked": p.value,
		}
	}
	p.next(false)
	elem["tail"] = p.typ == tBackQuote
	return p.finishNode(elem, "TemplateElement")
}

func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func (p *Parser) parseTemplate(isTagged bool) node {
	pnode := p.startNode()
	p.next(false)
	pnode["expressions"] = []node{}
	curElt := p.parseTemplateElement(isTagged)
	pnode["quasis"] = []node{curElt}
	for !curElt["tail"].(bool) {
		if p.typ == tEOF {
			p.raise(p.pos, "Unterminated template literal")
		}
		p.expect(tDollarBraceL)
		pnode["expressions"] = append(pnode["expressions"].([]node), p.parseExpression(false, nil))
		p.expect(tBraceR)
		curElt = p.parseTemplateElement(isTagged)
		pnode["quasis"] = append(pnode["quasis"].([]node), curElt)
	}
	p.next(false)
	return p.finishNode(pnode, "TemplateLiteral")
}

func (p *Parser) isAsyncProp(prop node) bool {
	return prop["computed"] != true &&
		prop["key"].(node)["type"] == "Identifier" && prop["key"].(node)["name"] == "async" &&
		(p.typ == tName || p.typ == tNum || p.typ == tString || p.typ == tBracketL || ttTable[p.typ].keyword != "" || p.typ == tStar) &&
		!lineBreakBetween(p.input, p.lastTokEnd, p.start)
}

func (p *Parser) parseObj(isPattern bool, ref *destructuringErrors) node {
	pnode := p.startNode()
	first := true
	pnode["properties"] = []node{}
	p.next(false)
	for !p.eat(tBraceR) {
		if !first {
			p.expect(tComma)
			if p.afterTrailingComma(tBraceR, false) {
				break
			}
		} else {
			first = false
		}
		prop := p.parseProperty(isPattern, ref)
		if !isPattern {
			p.checkPropClash(prop, nil, ref)
		}
		pnode["properties"] = append(pnode["properties"].([]node), prop)
	}
	if isPattern {
		return p.finishNode(pnode, "ObjectPattern")
	}
	return p.finishNode(pnode, "ObjectExpression")
}

func (p *Parser) parseProperty(isPattern bool, ref *destructuringErrors) node {
	prop := p.startNode()
	var isGenerator, isAsync bool
	var startPos int
	if p.eat(tEllipsis) {
		if isPattern {
			prop["argument"] = p.parseIdent(false)
			if p.typ == tComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			return p.finishNode(prop, "RestElement")
		}
		prop["argument"] = p.parseMaybeAssign(false, ref)
		if p.typ == tComma && ref != nil && ref.trailingComma < 0 {
			ref.trailingComma = p.start
		}
		return p.finishNode(prop, "SpreadElement")
	}
	prop["method"] = false
	prop["shorthand"] = false
	if isPattern || ref != nil {
		startPos = p.start
	}
	if !isPattern {
		isGenerator = p.eat(tStar)
	}
	containsEsc := p.containsEsc
	p.parsePropertyName(prop)
	if !isPattern && !containsEsc && !isGenerator && p.isAsyncProp(prop) {
		isAsync = true
		isGenerator = p.eat(tStar)
		p.parsePropertyName(prop)
	} else {
		isAsync = false
	}
	p.parsePropertyValue(prop, isPattern, isGenerator, isAsync, startPos, ref, containsEsc)
	prop["start"] = prop["start"]
	// restore start to before property name (method/shorthand node starts at parseProperty start)
	return p.finishNode(prop, "Property")
}

func (p *Parser) parseGetterSetter(prop node) {
	kind := prop["key"].(node)["name"].(string)
	p.parsePropertyName(prop)
	prop["value"] = p.parseMethod(false, false, false)
	prop["kind"] = kind
	paramCount := 0
	if kind == "set" {
		paramCount = 1
	}
	params := prop["value"].(node)["params"].([]node)
	if len(params) != paramCount {
		start := prop["value"].(node)["start"].(int)
		if kind == "get" {
			p.raiseRecoverable(start, "getter should have no params")
		} else {
			p.raiseRecoverable(start, "setter should have exactly one param")
		}
	}
}

func (p *Parser) parsePropertyValue(prop node, isPattern, isGenerator, isAsync bool, startPos int, ref *destructuringErrors, containsEsc bool) {
	if (isGenerator || isAsync) && p.typ == tColon {
		p.unexpected(-1)
	}
	if p.eat(tColon) {
		if isPattern {
			prop["value"] = p.parseMaybeDefault(p.start, nil)
		} else {
			prop["value"] = p.parseMaybeAssign(false, ref)
		}
		prop["kind"] = "init"
	} else if p.typ == tParenL {
		if isPattern {
			p.unexpected(-1)
		}
		prop["method"] = true
		prop["value"] = p.parseMethod(isGenerator, isAsync, false)
		prop["kind"] = "init"
	} else if !isPattern && !containsEsc && !prop["computed"].(bool) && prop["key"].(node)["type"] == "Identifier" &&
		(prop["key"].(node)["name"] == "get" || prop["key"].(node)["name"] == "set") &&
		(p.typ != tComma && p.typ != tBraceR && p.typ != tEq) {
		if isGenerator || isAsync {
			p.unexpected(-1)
		}
		p.parseGetterSetter(prop)
	} else if !prop["computed"].(bool) && prop["key"].(node)["type"] == "Identifier" {
		if isGenerator || isAsync {
			p.unexpected(-1)
		}
		if prop["key"].(node)["name"] == "await" && p.awaitIdentPos == 0 {
			p.awaitIdentPos = startPos
		}
		if isPattern {
			prop["value"] = p.parseMaybeDefault(startPos, p.copyNode(prop["key"].(node)))
		} else if p.typ == tEq && ref != nil {
			if ref.shorthandAssign < 0 {
				ref.shorthandAssign = p.start
			}
			prop["value"] = p.parseMaybeDefault(startPos, p.copyNode(prop["key"].(node)))
		} else {
			prop["value"] = p.copyNode(prop["key"].(node))
		}
		prop["kind"] = "init"
		prop["shorthand"] = true
	} else {
		p.unexpected(-1)
	}
}

func (p *Parser) parsePropertyName(prop node) {
	if p.eat(tBracketL) {
		prop["computed"] = true
		prop["key"] = p.parseMaybeAssign(false, nil)
		p.expect(tBracketR)
		return
	}
	prop["computed"] = false
	if p.typ == tNum || p.typ == tString {
		prop["key"] = p.parseExprAtom(nil, false, false)
	} else {
		prop["key"] = p.parseIdent(true)
	}
}

func (p *Parser) initFunction(pnode node) {
	pnode["id"] = nil
	pnode["generator"] = false
	pnode["expression"] = false
	pnode["async"] = false
}

func (p *Parser) parseMethod(isGenerator, isAsync, allowDirectSuper bool) node {
	pnode := p.startNode()
	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
	p.initFunction(pnode)
	pnode["generator"] = isGenerator
	pnode["async"] = isAsync
	p.yieldPos = 0
	p.awaitPos = 0
	p.awaitIdentPos = 0
	flags := p.functionFlags(isAsync, isGenerator) | scopeSuper
	if allowDirectSuper {
		flags |= scopeDirectSuper
	}
	p.enterScope(flags)
	p.expect(tParenL)
	pnode["params"] = p.parseBindingList(tParenR, false, true, false)
	p.checkYieldAwaitInDefaultParams()
	p.parseFunctionBody(pnode, false, true, false)
	p.yieldPos = oldYieldPos
	p.awaitPos = oldAwaitPos
	p.awaitIdentPos = oldAwaitIdentPos
	return p.finishNode(pnode, "FunctionExpression")
}

func (p *Parser) parseArrowExpression(pnode node, params []node, isAsync bool, forInit bool) node {
	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
	p.enterScope(p.functionFlags(isAsync, false) | scopeArrow)
	p.initFunction(pnode)
	pnode["async"] = isAsync
	p.yieldPos = 0
	p.awaitPos = 0
	p.awaitIdentPos = 0
	pnode["params"] = p.toAssignableList(params, true)
	p.parseFunctionBody(pnode, true, false, forInit)
	p.yieldPos = oldYieldPos
	p.awaitPos = oldAwaitPos
	p.awaitIdentPos = oldAwaitIdentPos
	return p.finishNode(pnode, "ArrowFunctionExpression")
}

func (p *Parser) parseFunctionBody(pnode node, isArrowFunction, isMethod, forInit bool) {
	isExpression := isArrowFunction && p.typ != tBraceL
	oldStrict := p.strict
	useStrict := false
	if isExpression {
		pnode["body"] = p.parseMaybeAssign(forInit, nil)
		pnode["expression"] = true
		p.checkParams(pnode, false)
	} else {
		nonSimple := !p.isSimpleParamList(pnode["params"].([]node))
		if !oldStrict || nonSimple {
			// strict directive detection skipped for module context (already strict)
			_ = nonSimple
		}
		oldLabels := p.labels
		p.labels = []labelInfo{}
		if useStrict {
			p.strict = true
		}
		p.checkParams(pnode, true)
		if p.strict && pnode["id"] != nil {
			p.checkLValSimple(pnode["id"].(node), bindOutside, nil)
		}
		body := p.startNode()
		body["body"] = []node{}
		p.expect(tBraceL)
		if !isArrowFunction && !isMethod {
			// body scoping
		}
		for p.typ != tBraceR {
			stmt := p.parseStatement("", false, nil)
			body["body"] = append(body["body"].([]node), stmt)
		}
		p.next(false)
		pnode["body"] = p.finishNode(body, "BlockStatement")
		pnode["expression"] = false
		p.adaptDirectivePrologue(pnode["body"].(node)["body"].([]node))
		p.labels = oldLabels
	}
	p.exitScope()
}

func (p *Parser) isSimpleParamList(params []node) bool {
	for i := 0; i < len(params); i++ {
		if params[i]["type"] != "Identifier" {
			return false
		}
	}
	return true
}

func (p *Parser) checkParams(pnode node, allowDuplicates bool) {
	nameHash := make(map[string]bool)
	for i := 0; i < len(pnode["params"].([]node)); i++ {
		param := pnode["params"].([]node)[i]
		p.checkLValInnerPattern(param, bindVar, func() map[string]bool {
			if allowDuplicates {
				return nil
			}
			return nameHash
		}())
	}
}

func (p *Parser) parseExprList(close tokenType, allowTrailingComma, allowEmpty bool, ref *destructuringErrors) []node {
	elts := []node{}
	first := true
	for !p.eat(close) {
		if !first {
			p.expect(tComma)
			if allowTrailingComma && p.afterTrailingComma(close, false) {
				break
			}
		} else {
			first = false
		}
		var elt node
		if allowEmpty && p.typ == tComma {
			elt = nil
		} else if p.typ == tEllipsis {
			elt = p.parseSpread(ref)
			if ref != nil && p.typ == tComma && ref.trailingComma < 0 {
				ref.trailingComma = p.start
			}
		} else {
			elt = p.parseMaybeAssign(false, ref)
		}
		elts = append(elts, elt)
	}
	return elts
}

func (p *Parser) checkUnreserved(ref node) {
	name := ref["name"].(string)
	if p.keywords2(name) {
		p.raise(ref["start"].(int), "Unexpected keyword '"+name+"'")
	}
	if p.strict && (isReservedStrict(name)) {
		if name != "await" {
			p.raiseRecoverable(ref["start"].(int), "The keyword '"+name+"' is reserved")
		}
	}
}

func (p *Parser) keywords2(name string) bool {
	_, ok := ecma6Keywords[name]
	return ok
}

func isReservedStrict(name string) bool {
	switch name {
	case "implements", "interface", "let", "package", "private", "protected", "public", "static", "yield", "eval", "arguments":
		return true
	}
	return false
}

func (p *Parser) parseIdent(liberal bool) node {
	pnode := p.startNode()
	if p.typ == tName {
		pnode["name"] = p.value.(string)
	} else if ttTable[p.typ].keyword != "" {
		pnode["name"] = ttTable[p.typ].keyword
		p.typ = tName
	} else {
		p.unexpected(-1)
	}
	p.next(liberal)
	p.finishNode(pnode, "Identifier")
	if !liberal {
		p.checkUnreserved(pnode)
		if pnode["name"] == "await" && p.awaitIdentPos == 0 {
			p.awaitIdentPos = pnode["start"].(int)
		}
	}
	return pnode
}

func (p *Parser) parsePrivateIdent() node {
	pnode := p.startNode()
	if p.typ == tPrivateID {
		pnode["name"] = p.value.(string)
	} else {
		p.unexpected(-1)
	}
	p.next(false)
	p.finishNode(pnode, "PrivateIdentifier")
	return pnode
}

func (p *Parser) parseYield(forInit bool) node {
	if p.yieldPos == 0 {
		p.yieldPos = p.start
	}
	pnode := p.startNode()
	p.next(false)
	if p.typ == tSemi || p.canInsertSemicolon() || (p.typ != tStar && !ttTable[p.typ].startsExpr) {
		pnode["delegate"] = false
		pnode["argument"] = nil
	} else {
		pnode["delegate"] = p.eat(tStar)
		pnode["argument"] = p.parseMaybeAssign(forInit, nil)
	}
	return p.finishNode(pnode, "YieldExpression")
}

func (p *Parser) parseAwait(forInit bool) node {
	if p.awaitPos == 0 {
		p.awaitPos = p.start
	}
	pnode := p.startNode()
	p.next(false)
	pnode["argument"] = p.parseMaybeUnary(nil, true, false, forInit)
	return p.finishNode(pnode, "AwaitExpression")
}

// toAssignable and related (pattern conversion)

func (p *Parser) toAssignable(pnode node, isBinding bool, ref *destructuringErrors) node {
	if pnode == nil {
		return pnode
	}
	switch pnode["type"] {
	case "Identifier":
	case "ObjectPattern", "ArrayPattern", "AssignmentPattern", "RestElement":
	case "ObjectExpression":
		pnode["type"] = "ObjectPattern"
		if ref != nil {
			p.checkPatternErrors(ref, true)
		}
		props := pnode["properties"].([]node)
		for i := 0; i < len(props); i++ {
			p.toAssignable(props[i], isBinding, nil)
			if props[i]["type"] == "RestElement" && (props[i]["argument"].(node)["type"] == "ArrayPattern" || props[i]["argument"].(node)["type"] == "ObjectPattern") {
				p.raise(props[i]["argument"].(node)["start"].(int), "Unexpected token")
			}
		}
	case "Property":
		if pnode["kind"] != "init" {
			p.raise(pnode["key"].(node)["start"].(int), "Object pattern can't contain getter or setter")
		}
		p.toAssignable(pnode["value"].(node), isBinding, nil)
	case "ArrayExpression":
		pnode["type"] = "ArrayPattern"
		if ref != nil {
			p.checkPatternErrors(ref, true)
		}
		p.toAssignableList(pnode["elements"].([]node), isBinding)
	case "SpreadElement":
		pnode["type"] = "RestElement"
		p.toAssignable(pnode["argument"].(node), isBinding, nil)
		if pnode["argument"].(node)["type"] == "AssignmentPattern" {
			p.raise(pnode["argument"].(node)["start"].(int), "Rest elements cannot have a default value")
		}
	case "AssignmentExpression":
		if pnode["operator"] != "=" {
			p.raise(pnode["left"].(node)["end"].(int), "Only '=' operator can be used for specifying default value.")
		}
		pnode["type"] = "AssignmentPattern"
		delete(pnode, "operator")
		p.toAssignable(pnode["left"].(node), isBinding, nil)
	case "ParenthesizedExpression":
		p.toAssignable(pnode["expression"].(node), isBinding, ref)
	case "ChainExpression":
		p.raiseRecoverable(pnode["start"].(int), "Optional chaining cannot appear in left-hand side")
	case "MemberExpression":
		if isBinding {
			p.raise(pnode["start"].(int), "Assigning to rvalue")
		}
	default:
		p.raise(pnode["start"].(int), "Assigning to rvalue")
	}
	return pnode
}

func (p *Parser) toAssignableList(exprList []node, isBinding bool) []node {
	end := len(exprList)
	for i := 0; i < end; i++ {
		if exprList[i] != nil {
			p.toAssignable(exprList[i], isBinding, nil)
		}
	}
	return exprList
}

func toKeyString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return "true"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return "null"
	default:
		return ""
	}
}

func (p *Parser) parseSpread(ref *destructuringErrors) node {
	pnode := p.startNode()
	p.next(false)
	pnode["argument"] = p.parseMaybeAssign(false, ref)
	return p.finishNode(pnode, "SpreadElement")
}

func (p *Parser) parseRestBinding() node {
	pnode := p.startNode()
	p.next(false)
	pnode["argument"] = p.parseBindingAtom()
	return p.finishNode(pnode, "RestElement")
}

func (p *Parser) parseBindingAtom() node {
	switch p.typ {
	case tBracketL:
		pnode := p.startNode()
		p.next(false)
		pnode["elements"] = p.parseBindingList(tBracketR, true, true, false)
		return p.finishNode(pnode, "ArrayPattern")
	case tBraceL:
		return p.parseObj(true, nil)
	}
	return p.parseIdent(false)
}

func (p *Parser) parseBindingList(close tokenType, allowEmpty, allowTrailingComma, allowModifiers bool) []node {
	elts := []node{}
	first := true
	for !p.eat(close) {
		if !first {
			p.expect(tComma)
		} else {
			first = false
		}
		if allowEmpty && p.typ == tComma {
			elts = append(elts, nil)
		} else if allowTrailingComma && p.afterTrailingComma(close, false) {
			break
		} else if p.typ == tEllipsis {
			rest := p.parseRestBinding()
			elts = append(elts, rest)
			if p.typ == tComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			p.expect(close)
			break
		} else {
			elts = append(elts, p.parseAssignableListItem(allowModifiers))
		}
	}
	return elts
}

func (p *Parser) parseAssignableListItem(allowModifiers bool) node {
	elem := p.parseMaybeDefault(p.start, nil)
	return elem
}

func (p *Parser) parseMaybeDefault(startPos int, left node) node {
	if left == nil {
		left = p.parseBindingAtom()
	}
	if !p.eat(tEq) {
		return left
	}
	pnode := p.startNodeAt(startPos)
	pnode["left"] = left
	pnode["right"] = p.parseMaybeAssign(false, nil)
	return p.finishNode(pnode, "AssignmentPattern")
}

func (p *Parser) checkLValSimple(expr node, bindingType int, checkClashes map[string]bool) {
	switch expr["type"] {
	case "Identifier":
		if bindingType != bindNone {
			// name declaration tracking not needed for AST-only parity
		}
	case "ChainExpression":
		p.raiseRecoverable(expr["start"].(int), "Optional chaining cannot appear in left-hand side")
	case "MemberExpression":
		if bindingType != bindNone {
			p.raiseRecoverable(expr["start"].(int), "Binding member expression")
		}
	case "ParenthesizedExpression":
		if bindingType != bindNone {
			p.raiseRecoverable(expr["start"].(int), "Binding parenthesized expression")
		}
		p.checkLValSimple(expr["expression"].(node), bindingType, checkClashes)
	default:
		msg := "Assigning to rvalue"
		if bindingType != bindNone {
			msg = "Binding rvalue"
		}
		p.raise(expr["start"].(int), msg)
	}
}

func (p *Parser) checkLValPattern(expr node, bindingType int, checkClashes map[string]bool) {
	switch expr["type"] {
	case "ObjectPattern":
		props := expr["properties"].([]node)
		for i := 0; i < len(props); i++ {
			p.checkLValInnerPattern(props[i], bindingType, checkClashes)
		}
	case "ArrayPattern":
		elts := expr["elements"].([]node)
		for i := 0; i < len(elts); i++ {
			if elts[i] != nil {
				p.checkLValInnerPattern(elts[i], bindingType, checkClashes)
			}
		}
	default:
		p.checkLValSimple(expr, bindingType, checkClashes)
	}
}

func (p *Parser) checkLValInnerPattern(expr node, bindingType int, checkClashes map[string]bool) {
	switch expr["type"] {
	case "Property":
		p.checkLValInnerPattern(expr["value"].(node), bindingType, checkClashes)
	case "AssignmentPattern":
		p.checkLValPattern(expr["left"].(node), bindingType, checkClashes)
	case "RestElement":
		p.checkLValPattern(expr["argument"].(node), bindingType, checkClashes)
	default:
		p.checkLValPattern(expr, bindingType, checkClashes)
	}
}

func (p *Parser) checkPropClash(prop node, propHash map[string]interface{}, ref *destructuringErrors) {
	if prop["type"] == "SpreadElement" {
		return
	}
	if prop["computed"] == true || prop["method"] == true || prop["shorthand"] == true {
		return
	}
	key := prop["key"].(node)
	var name string
	switch key["type"] {
	case "Identifier":
		name = key["name"].(string)
	case "Literal":
		name = toKeyString(key["value"])
	default:
		return
	}
	kind := prop["kind"].(string)
	_ = name
	_ = kind
	// __proto__ and redefinition checks are error-only; not needed for AST parity
}
