package acorn

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

var commentSources = []string{
	"// leading\nlet a = 1;",
	"let a = 1; // trailing",
	"/* block */ let a = 1;",
	"/* multi\nline\nblock */ foo();",
	"let a = 1; // one\nlet b = 2; // two",
	"/* a */ let x = 1; /* b */ let y = 2; /* c */",
	"// only a comment\n",
	"",
	"let a = 1; /* c1 */ let b = 2; // c2",
	"/*! license */ export const x = 1;",
}

func runAcornComments(t *testing.T, src string) [][4]any {
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
const cs=[];
try { acorn.parse(process.argv[1],{ecmaVersion:'latest',sourceType:'module',onComment:(b,t,s,e)=>{cs.push([b,t,s,e]);}}); }
catch(e){}
process.stdout.write(JSON.stringify(cs));`
	origAbs, _ := filepath.Abs(orig)
	argv := []string{"-e", driver, src}
	cmd := exec.Command(nodeBin, argv...)
	cmd.Env = append(os.Environ(), "ACORN_ORIG="+origAbs)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	var res [][4]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out)
	}
	return res
}

func goComments(src string) [][4]any {
	_, cs, err := ParseComments(src)
	if err != nil {
		return nil
	}
	out := make([][4]any, 0, len(cs))
	for _, c := range cs {
		out = append(out, [4]any{c.Block, c.Text, float64(c.Start), float64(c.End)})
	}
	return out
}

func TestCommentCaptureParity(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available; skipping comment parity")
	}
	_ = nodeBin
	for _, src := range commentSources {
		want := runAcornComments(t, src)
		got := goComments(src)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("src %q:\n  go =%v\n  js =%v", src, got, want)
		}
	}
	t.Logf("comment capture parity OK: %d sources", len(commentSources))
}

func TestParseCommentsReturnsValidAST(t *testing.T) {
	v, cs, err := ParseComments("/* c */ export const x = 1;\n// note\n")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("nil AST")
	}
	if len(cs) != 2 {
		t.Fatalf("want 2 comments, got %d: %+v", len(cs), cs)
	}
	if !cs[0].Block || cs[1].Block {
		t.Fatalf("expected block then line comment: %+v", cs)
	}
}
