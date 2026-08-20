# acorn-go

A Go port of a well-scoped **core subset** of the [acorn](https://github.com/acornjs/acorn) 8.x
JavaScript parser. It parses JavaScript source as an ES module at the latest ECMAScript version
and produces an **ESTree AST** that is byte-for-byte structurally identical to real acorn's output,
verified by a JS-oracle parity test.

## API

```go
import "github.com/jclyons52/acorn-go"

ast, err := acorn.Parse(src)          // -> ESTree AST (map[string]interface{} trees)
jsonStr, err := acorn.ParseJSON(src)  // -> AST marshaled to JSON
```

`Parse` always uses `{ecmaVersion: "latest", sourceType: "module"}`. Nodes are
`map[string]interface{}` with `"type"`, `"start"`, `"end"` plus per-construct fields, matching
acorn's ESTree output exactly. Positions are 0-based character offsets into the input (Go byte
offsets, which equal JS UTF-16 code-unit offsets for ASCII input).

## Parity

**PARITY PASS** (as observed from `go test ./...`):

```
parity_test.go: PARITY PASS/total: 85/85 cases, 0 mismatches
```

`parity_test.go` runs the **vendored real acorn 8.15.0** (`original/acorn-8.15.0.js`) under
Node as the oracle for the same source, then deep-compares the two ASTs
(structure + `start`/`end`, JSON-number normalized). It `t.Skip`s cleanly if `node` is
unavailable. The corpus (85 inline programs) covers every construct listed below.

## Scope (ported)

* **Lexing**: full tokenizer — numbers (decimal/hex/octal/binary/exponent/separators), strings
  with escapes, template literals (incl. `\n` normalization and `${ }` interpolation), regex
  literals, identifiers, keywords, punctuators and operators, comments, and the token-context
  machinery (`exprAllowed`) that decides regex-vs-division.
* **Expressions**: literals, identifier/`this`/`super`, arrays (incl. holes + spread), objects
  (shorthand, computed, methods, getters/setters, spread, numeric keys, `__proto__`), function &
  arrow expressions (incl. async, generator, destructuring params, expression/block bodies),
  unary/binary/logical/coalesce/conditional/assignment/update/sequence, member/call/new/
  tagged-template, optional chaining (`?.`, `?.[]`, `?.()`), `new.target`, `import()`.
* **Statements**: variable declarations (`var`/`let`/`const`), blocks, empty, expression,
  `if/else`, `return`, loops (`for`, `for-in`, `for-of`, `while`, `do-while`), `break/continue`,
  `switch`, `try/catch/finally` (incl. optional catch), `throw`, `debugger`, labeled statements,
  function/class declarations, destructuring patterns, implicit semicolon insertion, directive
  prologues (`"use strict"`).
* **Modules**: `import` (default/named/namespace/side-effect), `export` (default declaration,
  named from declarations and specifier lists, `export *`, `export * as ns`, re-exports),
  with the `attributes` field.

## Known limitations (not ported / intentionally weaker)

* **Error-accurate parsing is not the goal**: the port parses *valid* programs to identical ASTs.
  Some invalid-input diagnostics diverge (e.g. mixing `??` with `&&`/`||` without parens is
  *not* re-checked and may parse instead of error; several strict-mode/binding redeclaration
  checks are relaxed). Do not rely on it as a linter/validator.
* **Scope/binding bookkeeping** is minimal: it is enough to drive `yield`/`await`/`super`/
  `new.target` context, but not full `declareName` duplicate-name semantics.
* **Unicode identifier tables** are a conservative hand-maintained BMP subset (Latin/Greek/
  Cyrillic/CJK plus combining marks). Astral-plane (>0xFFFF) identifier characters are not
  supported; some rare scripts may not tokenize as identifiers. The full generated tables were
  out of scope.
* **Positions** match for ASCII input. For non-BMP (astral) characters, JS UTF-16 code-unit
  offsets differ from Go byte offsets; the corpus is ASCII-only.
* **`-->`/`<!--` line comments and hashbang** are not specially handled.
* A few rarely-hit tokenizer/parser edge paths (e.g. numeric separators error positions,
  legacy octal in strings, `\u{...}` code-point bounds) are simplified; they do not change
  AST structure for the covered corpus.

## Original source

The oracle is the real published npm package **acorn@8.15.0** (a concrete 8.x within the
`^8.11.0` that ESLint 8.57's graph resolves via), vendored at
`original/acorn-8.15.0.js` (+ `original/LICENSE`), downloaded from the npm registry. No source
is fabricated.

## Repository layout

```
acorn-go/
  api.go            Public Parse / ParseJSON entry points
  token.go          Token types, keyword table, identifier predicates
  parser.go         Tokenizer + token-context machinery + node/error plumbing
  statement.go      Statement parsing
  expression.go     Expression parsing + pattern conversion
  function.go       Function / class / import / export parsing
  unicode.go        Identifier character tables
  parity_test.go    JS-oracle AST parity harness (the acceptance gate)
  original/         Vendored acorn@8.15.0 (oracle)
  LICENSE           MIT
```

## License

MIT.
