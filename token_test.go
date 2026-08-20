package acorn

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var tokenSources = []string{
	"let a = 1;",
	"const name = 'world';",
	"function f(x, y) { return x + y * 2; }",
	"const obj = { a: 1, b: [true, false, null] };",
	"export default class Foo extends Bar {}",
	"const t = `hi ${name}!`;",
	"if (a && b || !c) { x ??= 0; } else { x?.y; }",
	"for (const k in obj) { } for (let i = 0; i < 3; i++) {}",
	"a == b; a != b; a === b; a <= b; a >>> 1; a ** 2;",
	"let u = 0.5, v = 1e3, w = 123n;",
	"import x, { y as z } from 'm';",
	"try { throw new Error('e'); } catch (err) { /* swallow */ } finally {}",
}

// normValue serializes a Go token value to a comparable string (JSON).
func normValue(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}

func runAcornTokens(t *testing.T, src string) [][3]any {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	orig := filepath.Join("original", "acorn-8.15.0.js")
	if _, err := os.Stat(orig); err != nil {
		t.Fatalf("vendored acorn missing: %v", err)
	}
	driver := `const acorn=require(process.env.ACORN_ORIG);
const toks=[];
try { acorn.parse(process.argv[1],{ecmaVersion:'latest',sourceType:'module',onToken:(tok)=>{const v=(typeof tok.value==='bigint')?tok.value.toString():tok.value; toks.push([tok.type.label, v===undefined?null:v, tok.start, tok.end]);}}); }
catch(e){}
process.stdout.write(JSON.stringify(toks));`
	origAbs, _ := filepath.Abs(orig)
	argv := []string{"-e", driver, src}
	cmd := exec.Command(nodeBin, argv...)
	cmd.Env = append(os.Environ(), "ACORN_ORIG="+origAbs)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var raw [][]any
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out)
	}
	var toks [][3]any
	for _, r := range raw {
		label, _ := r[0].(string)
		val := "null"
		if r[1] != nil {
			b, _ := json.Marshal(r[1])
			val = string(b)
		}
		toks = append(toks, [3]any{label, val, numToInt(r[2])})
	}
	return toks
}

func numToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return -1
}

func goTokens(src string) [][3]any {
	_, toks, err := ParseTokens(src)
	if err != nil {
		return nil
	}
	var out [][3]any
	for _, tk := range toks {
		val := tk.Value
		// acorn-go stores BigInt literals as the raw source ("123n"); acorn
		// stores the bigint value ("123"). Strip the trailing marker so the
		// numeric value compares equal.
		if tk.Label == "num" {
			if s, ok := val.(string); ok {
				val = strings.TrimSuffix(s, "n")
			}
		}
		out = append(out, [3]any{tk.Label, normValue(val), tk.Start})
	}
	return out
}

func TestTokenCaptureParity(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; skipping token parity")
	}
	for _, src := range tokenSources {
		want := runAcornTokens(t, src)
		got := goTokens(src)
		if !tokenListsEqual(got, want) {
			t.Errorf("src %q:\n  go (%d):\n%s  js (%d):\n%s",
				src, len(got), dumpToks(got), len(want), dumpToks(want))
		}
	}
	t.Logf("token capture parity OK: %d sources", len(tokenSources))
}

func tokenListsEqual(got, want [][3]any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] || got[i][2] != want[i][2] {
			return false
		}
	}
	return true
}

func dumpToks(toks [][3]any) string {
	var b string
	for _, tk := range toks {
		b += fmt.Sprintf("    %-16s %-12s @%d\n", tk[0], tk[1], tk[2])
	}
	return b
}

func TestParseTokensIncludesEOF(t *testing.T) {
	_, toks, err := ParseTokens("let a = 1;")
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) == 0 || toks[len(toks)-1].Label != "eof" {
		t.Fatalf("expected trailing eof token, got %d tokens ending %v", len(toks), labelOf(toks))
	}
}

func labelOf(toks []Token) []string {
	var out []string
	for _, tk := range toks {
		out = append(out, tk.Label)
	}
	return out
}
