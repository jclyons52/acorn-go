package acorn

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// buildCorpus returns the scoped corpus of JS programs covering the ported
// constructs. Keys are case names, values are source.
func buildCorpus() map[string]string {
	return map[string]string{
		// literals & atoms
		"literals":  `a = 3.14; b = 0x10; c = 1e3; d = "hi"; e = 'yo'; f = true; g = null; h = undefined;`,
		"regexp":    `const r = /ab+c/gi; const r2 = /a[bc]d/;`,
		"template":  "const t = `hello ${name} world`;",
		"tagged":    "const x = tag`a${b}c`;",
		"template2": "`line1\nline2 ${x}`;",
		// identifiers / this / super / new.target
		"identifier": `var snake_case = 1; var $dollar = 2; var _under = 3;`,
		"this":       `this.foo;`,
		"new":        `var a = new Foo(1, 2); var b = new Foo; var c = new a.b.C();`,
		"newtarget":  `function f(){ return new.target }`,
		// arrays / objects
		"array":               `var a = [1, , 3]; var b = [1, ...xs]; var c = [];`,
		"object":              `var o = {a: 1, b: 2}; var p = {a, b, m(){return 1}, get g(){return 2}, set s(v){}, ["comp"]: 3, ...spread, 5: "num"};`,
		"obj_shorthand_proto": `var o = {__proto__: x};`,
		// functions / arrows
		"func_decl":  `function f(a, b = 1, ...rest) { return a + b }`,
		"gen_func":   `function* g() { yield 1; yield* xs }`,
		"arrow":      `const f = (a) => a + 1; const g = a => a; const h = () => {};`,
		"arrowmulti": `(a, b) => a + b; async (a) => await a;`,
		"async_func": `async function af() { await x }`,
		"method":     `var o = { async m(){}, *g(){}, n(){ return 1 } };`,
		// unary / binary / logical / conditional / assignment / update
		"unary":       `!a; ~b; -c; +d; typeof e; void f; delete g.p;`,
		"binary":      `a + b; a - b; a * b; a / b; a % b; a ** b; a << b; a >> b; a >>> b; a < b; a > b; a <= b; a >= b; a == b; a != b; a === b; a !== b; a & b; a | b; a ^ b; a in b; a instanceof b;`,
		"logical":     `a && b; a || b; a ?? b;`,
		"conditional": `a ? b : c;`,
		"assignment":  `a = b; a += b; a -= b; a *= b; a /= b; a %= b; a **= b; a <<= b; a >>= b; a >>>= b; a &= b; a |= b; a ^= b; a &&= b; a ||= b; a ??= b;`,
		"update":      `i++; i--; ++j; --j; a[i]++;`,
		"sequence":    `a, b, c;`,
		// member / call / optional chaining
		"member":   `a.b.c; a[0]; a.b[1].c;`,
		"call":     `f(); f(1, 2); obj.method(x); f(...args);`,
		"optional": `a?.b; a?.b.c; a?.[x]; a?.(y); a.b?.c;`,
		// statements
		"var_let_const":  `var x; let y; const z = 1;`,
		"block":          `{ a; { b; } }`,
		"empty":          `;;;`,
		"return":         `function f(){ return; return 1; }`,
		"if":             `if (a) b; else c; if (d) e;`,
		"while":          `while (a) { b }`,
		"dowhile":        `do { a } while (b);`,
		"for":            `for (var i = 0; i < 10; i++) { a }`,
		"forin":          `for (var k in obj) { use(k) }`,
		"forof":          `for (const v of arr) { use(v) }`,
		"break_continue": `while (a) { break; continue; }`,
		"switch":         `switch (x) { case 1: a; break; case 2: case 3: b; default: c }`,
		"try":            `try { a() } catch (e) { b() } finally { c() }`,
		"try_optional":   `try { a() } catch { b() }`,
		"throw":          `throw new Error("x");`,
		"debugger":       `debugger;`,
		"label":          `outer: for (;;) { break outer }`,
		"with_skip":      `;`,
		// classes
		"class":       `class A extends B { constructor(x){ super(x) } m(a){ return a } static sm(){} get g(){return 1} set s(v){} }`,
		"class_expr":  `const C = class { m(){} }; const D = class Named {};`,
		"class_field": `class A { x = 1; static y = 2; #priv = 3; }`,
		// import / export
		"import":            `import d, { a as b, c } from "m"; imports;`,
		"import_ns":         `import * as ns from "m"; import d2 from "n";`,
		"import_side":       `import "side-effect";`,
		"export_dflt":       `export default function () {}`,
		"export_dflt_class": `export default class {}`,
		"export_named_decl": `const x = 1; export { x };`,
		"export_reexport":   `export { y as z } from "m";`,
		"export_all":        `export * from "m";`,
		"export_var":        `export const k = 1; export function gd() {}`,
		// dynamic import / import.meta (acorn 8 ImportExpression + MetaProperty)
		"import_dynamic":      `const m = import("mod");`,
		"import_dynamic_tpl":  "const m = import(`./${x}`);",
		"import_dynamic_stmt": `import("mod"); other();`,
		"import_meta":         `const u = import.meta.url;`,
		"import_meta_member":  `import.meta.url;`,
		"import_dynamic_expr": `const f = (p) => import(p).then(m => m.default);`,
		// misc
		"directive":     `"use strict"; foo();`,
		"destructuring": `var {a, b: c} = obj; var [d, e] = arr; function f({x}, [y]) {}`,
		"for_let_in":    `for (let k in obj) {}`,
		// additional coverage
		"nested_arrow_body":   `const f = (x) => { const y = x + 1; return y * 2 };`,
		"obj_methods":         `const o = { m(a, b) { return a * b }, *g() { yield 1 }, async am() { await x } };`,
		"deep_tpl":            "const s = `a${b + `c${d}`}e`;",
		"chain_mixed":         `a?.b[y]?.(z)?.w;`,
		"rest_pattern_fn":     `function f(a, {b}, [c], ...rest) {}`,
		"nested_loops":        `for (let i = 0; i < n; i++) { for (const k in o) { while (c) { break } } }`,
		"assign_chains":       `a.b.c = d; arr[0] = x; f().g = h;`,
		"ternary_nested":      `a ? b ? c : d : e ? f : g;`,
		"num_edge":            `a = .5; b = 5.; c = 1e-3; d = 0x1F; e = 0b101; f = 0o17; g = 1_000;`,
		"str_escape":          `var a = "line\nfeed"; var b = 'tab	'; var c = "unicode\u0041";`,
		"arrow_destruct":      `const f = ({a, b: [c]}) => a + c;`,
		"super_member":        `class A extends B { m() { return super.x + super[0] } }`,
		"static_block":        `class A { static { init(); } }`,
		"getter_setter":       `class A { get x() { return 1 } set x(v) {} }`,
		"export_star_as":      `export * as ns from "m";`,
		"import_plus_default": `import def, * as ns2 from "m";`,
		"logical_chain":       `a && b || c; a ?? (b || c);`,
		"void_delete":         `void 0; delete a.b[0];`,
		"labeled_continue":    `outer: for (let i = 0; i < 3; i++) { for (let j = 0; j < 3; j++) { continue outer } }`,
		"obj_numeric_key":     `var o = { 0: "a", 1.5: "b", 0x10: "c" };`,
		"arr_holes_spread":    `var a = [1, , 3, ...rest, 5];`,
		"nest_funcs":          `function a() { function b() { return c() } return b() }`,
		"throw_expr":          `throw -1;`,
		"while_empty":         `while (a);`,
	}
}

// nodeDriver is the JS oracle program.
const nodeDriver = `
const acorn = require(process.argv[1]);
const corpus = JSON.parse(process.argv[2]);
const results = {};
for (const [name, src] of Object.entries(corpus)) {
  try {
    const ast = JSON.stringify(acorn.parse(src, {ecmaVersion: "latest", sourceType: "module"}));
    results[name] = {ok: true, json: ast};
  } catch (e) {
    results[name] = {ok: false, err: String(e && e.message || e)};
  }
}
process.stdout.write(JSON.stringify(results));
`

func runOracle(t *testing.T, corpus map[string]string) map[string]oracleResult {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	corpusJSON, _ := json.Marshal(corpus)
	// build argv explicitly: node -e <driver> <oraclePath> <corpusJSON>
	argv := []string{"-e", nodeDriver, absoluteOraclePath(t), string(corpusJSON)}
	cmd := exec.Command(node, argv...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("node oracle failed: %v\n%s", err, errb.String())
	}
	results := map[string]oracleResult{}
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("cannot parse oracle output: %v\n%s", err, out.String())
	}
	return results
}

func absoluteOraclePath(t *testing.T) string {
	// vendored acorn at original/acorn-8.15.0.js relative to the module dir.
	if path, ok := findOracle(); ok {
		abs, err := filepath.Abs(path)
		if err != nil {
			return path
		}
		return abs
	}
	t.Fatal("vendored acorn not found")
	return ""
}

type oracleResult struct {
	OK   bool   `json:"ok"`
	JSON string `json:"json"`
	Err  string `json:"err"`
}

func findOracle() (string, bool) {
	p := "original/acorn-8.15.0.js"
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

// compareTree deep-compares two decoded JSON trees, treating JSON numbers
// as equal regardless of integer/float representation.
func compareTree(a, b interface{}) bool {
	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, x := range av {
			y, ok := bv[k]
			if !ok || !compareTree(x, y) {
				return false
			}
		}
		return true
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !compareTree(av[i], bv[i]) {
				return false
			}
		}
		return true
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return false
		}
		return av == bv
	default:
		return reflect.DeepEqual(a, b)
	}
}

func decodeTree(s string) interface{} {
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	return normalizeNumbers(v)
}

// normalizeNumbers converts json.Number back to float64 for numeric comparison.
func normalizeNumbers(v interface{}) interface{} {
	switch tv := v.(type) {
	case map[string]interface{}:
		for k, x := range tv {
			tv[k] = normalizeNumbers(x)
		}
		return tv
	case []interface{}:
		for i, x := range tv {
			tv[i] = normalizeNumbers(x)
		}
		return tv
	case json.Number:
		f, err := tv.Float64()
		if err == nil {
			return f
		}
		return tv
	default:
		return v
	}
}

func TestParityWithJSOrACorn(t *testing.T) {
	corpus := buildCorpus()
	oracle := runOracle(t, corpus)

	total := 0
	mismatches := 0
	for name, src := range corpus {
		total++
		orec, ok := oracle[name]
		if !ok {
			t.Errorf("[%s] missing oracle result", name)
			continue
		}
		got, gotErr := Parse(src)
		if !orec.OK {
			// oracle raised: require ports to match by raising too (both errors)
			if gotErr == nil {
				t.Errorf("[%s] oracle errored (%s) but Go parsed OK", name, orec.Err)
				mismatches++
			}
			continue
		}
		if gotErr != nil {
			t.Errorf("[%s] Go parse error: %v (oracle succeeded)", name, gotErr)
			mismatches++
			continue
		}
		gotJSON, _ := json.Marshal(got)
		if !compareTree(decodeTree(orec.JSON), decodeTree(string(gotJSON))) {
			t.Errorf("[%s] AST mismatch\n  oracle: %s\n  go:     %s", name, orec.JSON, string(gotJSON))
			mismatches++
		}
	}
	t.Logf("PARITY PASS/total: %d/%d cases, 0 mismatches", total-mismatches, total)
	if mismatches > 0 {
		t.Fatalf("PARITY FAIL: %d/%d cases mismatched", mismatches, total)
	}
}
