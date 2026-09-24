package acorn

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// strictDirective recognises the "use strict" prologue by scanning raw text, and
// it has to look at BOTH quoting styles: Go reports -1 for a regex capture group
// that did not participate, so a double-quoted literal leaves the single-quoted
// group unmatched. Slicing by that -1 panics, and `"use strict";` is the most
// common first line in shipped npm code — this test exists because a dogfooding
// run over 772 real files crashed on it.
//
// Strictness is observed the way acorn exposes it: a legacy octal literal is a
// SyntaxError in strict code and fine in sloppy code, so the parse outcome tells
// us whether the directive was honoured. Each source is compared with the real
// acorn, so the expectation is measured rather than assumed.
func TestStrictDirectiveParityAndNoPanic(t *testing.T) {
	srcs := []string{
		"\"use strict\";\nvar a = 010;",                      // double quotes: strict
		"'use strict';\nvar a = 010;",                        // single quotes: strict
		"\"use strict\";\nvar a = 1;\nvar b = 2;",            // double quotes, valid either way
		"var a = \"x\";\n\"use strict\";\nvar b = 010;",      // not a prologue: sloppy
		"'use\\x20strict';\nvar a = 010;",                    // escaped: acorn compares raw text
		"\"use strict\" + \"\";\nvar a = 010;",               // expression, not a directive
		"\"use asm\";\n\"use strict\";\nvar a = 010;",        // second directive
		"#!/usr/bin/env node\n\"use strict\";\nvar a = 010;", // shebang first
	}

	oracle := strictOracleResults(t, srcs)
	for i, src := range srcs {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC on %q: %v", src, r)
				}
			}()
			_, err = newParserWithOptions(src, ParseOptions{SourceType: "script"}).run()
		}()

		got := "ok"
		if err != nil {
			got = "err"
		}
		if got != oracle[i] {
			t.Errorf("script %q:\n  acorn: %s\n  go   : %s", src, oracle[i], got)
		}
	}
}

// strictOracleResults asks the real acorn (script mode) whether each source
// parses, returning "ok" or "err" per source.
func strictOracleResults(t *testing.T, srcs []string) []string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; cannot compare the strict prologue with real acorn")
	}
	orig := filepath.Join("original", "acorn-8.15.0.js")
	if _, err := os.Stat(orig); err != nil {
		t.Skip("vendored acorn missing: cannot compare with real acorn")
	}
	payload, err := json.Marshal(srcs)
	if err != nil {
		t.Fatal(err)
	}
	driver := `const acorn = require(process.env.ACORN_ORIG);
const srcs = JSON.parse(process.argv[1]);
const out = srcs.map(s => {
  try { acorn.parse(s, {ecmaVersion: 'latest', sourceType: 'script'}); return 'ok'; }
  catch (e) { return 'err'; }
});
process.stdout.write(JSON.stringify(out));`
	origAbs, _ := filepath.Abs(orig)
	cmd := exec.Command("node", "-e", driver, string(payload))
	cmd.Env = append(os.Environ(), "ACORN_ORIG="+origAbs)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node oracle failed: %v\n%s", err, out)
	}
	var res []string
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("cannot parse oracle output: %v\n%s", err, out)
	}
	return res
}
