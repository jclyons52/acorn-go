package acorn

// tokenType enumerates acorn's token types.
type tokenType int

const (
	tNum tokenType = iota
	tRegexp
	tString
	tName
	tPrivateID
	tEOF
	tBracketL
	tBracketR
	tBraceL
	tBraceR
	tParenL
	tParenR
	tComma
	tSemi
	tColon
	tDot
	tQuestion
	tQuestionDot
	tArrow
	tTemplate
	tInvalidTemplate
	tEllipsis
	tBackQuote
	tDollarBraceL
	tEq
	tAssign
	tIncDec
	tPrefix
	tLogicalOR
	tLogicalAND
	tBitwiseOR
	tBitwiseXOR
	tBitwiseAND
	tEquality
	tRelational
	tBitShift
	tPlusMin
	tModulo
	tStar
	tSlash
	tStarStar
	tCoalesce
	tBreak
	tCase
	tCatch
	tContinue
	tDebugger
	tDefault
	tDo
	tElse
	tFinally
	tFor
	tFunction
	tIf
	tReturn
	tSwitch
	tThrow
	tTry
	tVar
	tConst
	tWhile
	tWith
	tNew
	tThis
	tSuper
	tClass
	tExtends
	tExport
	tImport
	tNull
	tTrue
	tFalse
	tIn
	tInstanceof
	tTypeof
	tVoid
	tDelete
	tNumTokens
)

type ttInfo struct {
	label      string
	keyword    string
	beforeExpr bool
	startsExpr bool
	isLoop     bool
	isAssign   bool
	prefix     bool
	postfix    bool
	binop      int // -1 = none
}

func binopType(name string, prec int) ttInfo {
	return ttInfo{label: name, beforeExpr: true, binop: prec}
}

var ttTable = func() [tNumTokens]ttInfo {
	var t [tNumTokens]ttInfo
	t[tNum] = ttInfo{label: "num", startsExpr: true}
	t[tRegexp] = ttInfo{label: "regexp", startsExpr: true}
	t[tString] = ttInfo{label: "string", startsExpr: true}
	t[tName] = ttInfo{label: "name", startsExpr: true}
	t[tPrivateID] = ttInfo{label: "privateId", startsExpr: true}
	t[tEOF] = ttInfo{label: "eof"}
	t[tBracketL] = ttInfo{label: "[", beforeExpr: true, startsExpr: true}
	t[tBracketR] = ttInfo{label: "]"}
	t[tBraceL] = ttInfo{label: "{", beforeExpr: true, startsExpr: true}
	t[tBraceR] = ttInfo{label: "}"}
	t[tParenL] = ttInfo{label: "(", beforeExpr: true, startsExpr: true}
	t[tParenR] = ttInfo{label: ")"}
	t[tComma] = ttInfo{label: ",", beforeExpr: true}
	t[tSemi] = ttInfo{label: ";", beforeExpr: true}
	t[tColon] = ttInfo{label: ":", beforeExpr: true}
	t[tDot] = ttInfo{label: "."}
	t[tQuestion] = ttInfo{label: "?", beforeExpr: true}
	t[tQuestionDot] = ttInfo{label: "?."}
	t[tArrow] = ttInfo{label: "=>", beforeExpr: true}
	t[tTemplate] = ttInfo{label: "template"}
	t[tInvalidTemplate] = ttInfo{label: "invalidTemplate"}
	t[tEllipsis] = ttInfo{label: "...", beforeExpr: true}
	t[tBackQuote] = ttInfo{label: "`", startsExpr: true}
	t[tDollarBraceL] = ttInfo{label: "${", beforeExpr: true, startsExpr: true}
	t[tEq] = ttInfo{label: "=", beforeExpr: true, isAssign: true}
	t[tAssign] = ttInfo{label: "_=", beforeExpr: true, isAssign: true}
	t[tIncDec] = ttInfo{label: "++/--", prefix: true, postfix: true, startsExpr: true}
	t[tPrefix] = ttInfo{label: "!/~", beforeExpr: true, prefix: true, startsExpr: true}
	t[tLogicalOR] = binopType("||", 1)
	t[tLogicalAND] = binopType("&&", 2)
	t[tBitwiseOR] = binopType("|", 3)
	t[tBitwiseXOR] = binopType("^", 4)
	t[tBitwiseAND] = binopType("&", 5)
	t[tEquality] = binopType("==/!=/===/!==", 6)
	t[tRelational] = binopType("</>/<=/>=", 7)
	t[tBitShift] = binopType("<</>>/>>>", 8)
	t[tPlusMin] = ttInfo{label: "+/-", beforeExpr: true, binop: 9, prefix: true, startsExpr: true}
	t[tModulo] = binopType("%", 10)
	t[tStar] = binopType("*", 10)
	t[tSlash] = binopType("/", 10)
	t[tStarStar] = ttInfo{label: "**", beforeExpr: true}
	t[tCoalesce] = binopType("??", 1)

	// keywords
	t[tBreak] = ttInfo{label: "break", keyword: "break"}
	t[tCase] = ttInfo{label: "case", keyword: "case", beforeExpr: true}
	t[tCatch] = ttInfo{label: "catch", keyword: "catch"}
	t[tContinue] = ttInfo{label: "continue", keyword: "continue"}
	t[tDebugger] = ttInfo{label: "debugger", keyword: "debugger"}
	t[tDefault] = ttInfo{label: "default", keyword: "default", beforeExpr: true}
	t[tDo] = ttInfo{label: "do", keyword: "do", isLoop: true, beforeExpr: true}
	t[tElse] = ttInfo{label: "else", keyword: "else", beforeExpr: true}
	t[tFinally] = ttInfo{label: "finally", keyword: "finally"}
	t[tFor] = ttInfo{label: "for", keyword: "for", isLoop: true}
	t[tFunction] = ttInfo{label: "function", keyword: "function", startsExpr: true}
	t[tIf] = ttInfo{label: "if", keyword: "if"}
	t[tReturn] = ttInfo{label: "return", keyword: "return", beforeExpr: true}
	t[tSwitch] = ttInfo{label: "switch", keyword: "switch"}
	t[tThrow] = ttInfo{label: "throw", keyword: "throw", beforeExpr: true}
	t[tTry] = ttInfo{label: "try", keyword: "try"}
	t[tVar] = ttInfo{label: "var", keyword: "var"}
	t[tConst] = ttInfo{label: "const", keyword: "const"}
	t[tWhile] = ttInfo{label: "while", keyword: "while", isLoop: true}
	t[tWith] = ttInfo{label: "with", keyword: "with"}
	t[tNew] = ttInfo{label: "new", keyword: "new", beforeExpr: true, startsExpr: true}
	t[tThis] = ttInfo{label: "this", keyword: "this", startsExpr: true}
	t[tSuper] = ttInfo{label: "super", keyword: "super", startsExpr: true}
	t[tClass] = ttInfo{label: "class", keyword: "class", startsExpr: true}
	t[tExtends] = ttInfo{label: "extends", keyword: "extends", beforeExpr: true}
	t[tExport] = ttInfo{label: "export", keyword: "export"}
	t[tImport] = ttInfo{label: "import", keyword: "import", startsExpr: true}
	t[tNull] = ttInfo{label: "null", keyword: "null", startsExpr: true}
	t[tTrue] = ttInfo{label: "true", keyword: "true", startsExpr: true}
	t[tFalse] = ttInfo{label: "false", keyword: "false", startsExpr: true}
	t[tIn] = ttInfo{label: "in", keyword: "in", beforeExpr: true, binop: 7}
	t[tInstanceof] = ttInfo{label: "instanceof", keyword: "instanceof", beforeExpr: true, binop: 7}
	t[tTypeof] = ttInfo{label: "typeof", keyword: "typeof", beforeExpr: true, prefix: true, startsExpr: true}
	t[tVoid] = ttInfo{label: "void", keyword: "void", beforeExpr: true, prefix: true, startsExpr: true}
	t[tDelete] = ttInfo{label: "delete", keyword: "delete", beforeExpr: true, prefix: true, startsExpr: true}
	// Non-binop tokens default to binop: -1 (none). Legit binops use 1..10.
	for i := range t {
		if t[i].binop == 0 {
			t[i].binop = -1
		}
	}
	return t
}()

// keywordTypes maps keyword strings to token types.
var keywordTypes map[string]tokenType

func init() {
	keywordTypes = make(map[string]tokenType)
	for i := 0; i < int(tNumTokens); i++ {
		if ttTable[i].keyword != "" {
			keywordTypes[ttTable[i].keyword] = tokenType(i)
		}
	}
	// ecma6 keyword set includes const class extends export import super
}

var ecma6Keywords = map[string]tokenType{
	"break": tBreak, "case": tCase, "catch": tCatch, "continue": tContinue,
	"debugger": tDebugger, "default": tDefault, "do": tDo, "else": tElse,
	"finally": tFinally, "for": tFor, "function": tFunction, "if": tIf,
	"return": tReturn, "switch": tSwitch, "throw": tThrow, "try": tTry,
	"var": tVar, "while": tWhile, "with": tWith, "null": tNull,
	"true": tTrue, "false": tFalse, "instanceof": tInstanceof,
	"typeof": tTypeof, "void": tVoid, "delete": tDelete, "new": tNew,
	"in": tIn, "this": tThis,
	// ecma6 additions
	"const": tConst, "class": tClass, "extends": tExtends,
	"export": tExport, "import": tImport, "super": tSuper,
}

// tokens is the set of token labels used in error/context code.
func (p *Parser) tokName(tt tokenType) string { return ttTable[tt].label }

func lineBreakBetween(s string, from, to int) bool {
	for i := from; i < to; i++ {
		switch s[i] {
		case '\n', '\r':
			return true
		case 0xe2:
			if i+2 < to && s[i+1] == 0x80 && (s[i+2] == 0xa8 || s[i+2] == 0xa9) {
				return true
			}
		}
	}
	return false
}

// isNewLineRune reports whether the given code point is a JS line terminator.
func isNewLineCode(code int) bool {
	return code == 10 || code == 13 || code == 0x2028 || code == 0x2029
}

func codePointToString(code int) string {
	if code <= 0xFFFF {
		return string(rune(code))
	}
	code -= 0x10000
	return string(rune((code>>10)+0xD800)) + string(rune((code&1023)+0xDC00))
}

// isIdentifierStartCode mirrors acorn's isIdentifierStart for the common case.
func isIdentifierStartCode(code int) bool {
	if code < 65 {
		return code == 36
	}
	if code < 91 {
		return true
	}
	if code < 97 {
		return code == 95
	}
	if code < 123 {
		return true
	}
	if code <= 0xffff {
		return code >= 0xaa && nonASCIIidStart(code)
	}
	return isInAstralStart(code)
}

// isIdentifierChar mirrors acorn's isIdentifierChar (astral supported).
func isIdentifierCharAST(code int, astral bool) bool {
	if code < 48 {
		return code == 36
	}
	if code < 58 {
		return true
	}
	if code < 65 {
		return false
	}
	if code < 91 {
		return true
	}
	if code < 97 {
		return code == 95
	}
	if code < 123 {
		return true
	}
	if code <= 0xffff {
		return code >= 0xaa && (nonASCIIidStart(code) || isNonASCIIIDPart(code))
	}
	if !astral {
		return false
	}
	return isInAstralStart(code) || isInAstralPart(code)
}

// isIdentifierChar with astral support always on (parser uses ecmaVersion>=6).
func isIdentifierCharCode(code int) bool { return isIdentifierCharAST(code, true) }

func isInAstralStart(code int) bool { return false }
func isInAstralPart(code int) bool  { return false }

// UTF-8 decode of the input, used by the tokenizer. Input is ASCII for the
// parity corpus so byte positions equal JS code-unit positions.
func (p *Parser) charCodeAt(i int) int {
	if i < 0 || i >= len(p.input) {
		return -1
	}
	b := p.input[i]
	if b < 0x80 {
		return int(b)
	}
	r, _ := decodeRuneInString(p.input[i:])
	return int(r)
}
