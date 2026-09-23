// Copyright 2026 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cue_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestQuantifiedAliasWitness(t *testing.T) {
	for _, alias := range []string{
		`let Y = x & int`,
		`let Z = x & int; let Y = Z`,
		`let Y = {a: x & int}`,
	} {
		for _, runtimeFirst := range []bool{false, true} {
			t.Run(alias, func(t *testing.T) {
				ctx := cuecontext.New()
				argument := "2"
				if alias == `let Y = {a: x & int}` {
					argument = "{a: 2}"
				}
				use := "f: func(y: Y) -> string: \"ok\"\nout: f(" + argument + ")\n"
				if runtimeFirst {
					use = "runtime: Y\n" + use
				} else {
					use += "runtime: Y\n"
				}
				v := ctx.CompileString("x: int\n" + strings.ReplaceAll(alias, ";", "\n") + "\n" + use)
				out := v.LookupPath(cue.ParsePath("out"))
				if err := out.Validate(); err != nil {
					t.Fatalf("pending witness: %v", err)
				}
				if _, err := out.MarshalJSON(); err == nil {
					t.Fatal("unresolved alias witness was erased")
				}
				for _, n := range []int{1, 2} {
					r := v.FillPath(cue.ParsePath("x"), n).LookupPath(cue.ParsePath("out"))
					if _, err := r.MarshalJSON(); (err == nil) != (n == 2) {
						t.Fatalf("witness %d: %v", n, err)
					}
				}
			})
		}
	}
	// Boolean predicate abbreviations keep their ordinary predicate meaning.
	v := cuecontext.New().CompileString(`
x: int
let Nat = int & >=0
let Either = x | string
f: func(y: Nat) -> int: y
g: func(y: Either) -> string: "ok"
out: [f(2), g("s")]
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[2,"ok"]` {
		t.Fatalf("predicate abbreviation: %s, %v", got, err)
	}
}

func TestQuantifiedAliasCodeOrigin(t *testing.T) {
	for _, source := range []string{
		"let F = func(x: int) -> int: x\nf: func(cb: F) -> int: cb(1)\nout: f(F)",
		"let F = forall (A) func(x: A) -> A: x\nf: func(cb: F) -> int: cb(1)\nout: f(F)",
		"let R = {f: func(x: int) -> int: x}\nf: func(cb: R) -> int: cb.f(1)\nout: f(R)",
		"let R = forall (A) {f: func(x: A) -> A: x}\nf: func(cb: R) -> int: cb.f(1)\nout: f(R)",
		"F(A) = func(x: A) -> A: x\nf: func(cb: F(int)) -> int: cb(1)\nout: f(F(int))",
		"R(A) = {f: func(x: A) -> A: x}\nf: func(cb: R(int)) -> int: cb.f(1)\nout: f(R(int))",
	} {
		v := cuecontext.New().CompileString(source)
		if got, err := v.LookupPath(cue.ParsePath("out")).Int64(); err != nil || got != 1 {
			t.Fatalf("source %s: %d, %v (compile: %v)", source, got, err, v.Err())
		}
	}
	v := cuecontext.New().CompileString(`
xs: [for n in [1, 2] {
	let F = func() -> int: n
	f: func(cb: F) -> int: cb()
	out: f(F)
}]
out: [xs[0].out, xs[1].out]
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[1,2]` {
		t.Fatalf("iteration captures: %s, %v", got, err)
	}
}

func TestParametricAliasWitness(t *testing.T) {
	for _, alias := range []string{
		`Alias(A) = x & A`,
		`Inner(B) = x & B; Alias(A) = Inner(A)`,
		`Alias(A) = {a: x & A}`,
	} {
		for _, aliasFirst := range []bool{false, true} {
			for _, runtimeFirst := range []bool{false, true} {
				t.Run(alias, func(t *testing.T) {
					argument := "2"
					if strings.Contains(alias, "{a:") {
						argument = "{a: 2}"
					}
					use := "f: func(y: Alias(int)) -> string: \"ok\"\nout: f(" + argument + ")\n"
					if runtimeFirst {
						use = "runtime: Alias(int)\n" + use
					} else {
						use += "runtime: Alias(int)\n"
					}
					decl := strings.ReplaceAll(alias, ";", "\n") + "\n"
					if aliasFirst {
						use = decl + use
					} else {
						use += decl
					}
					v := cuecontext.New().CompileString("x: int\n" + use)
					out := v.LookupPath(cue.ParsePath("out"))
					if err := out.Validate(); err != nil {
						t.Fatalf("pending witness: %v", err)
					}
					if _, err := out.MarshalJSON(); err == nil {
						t.Fatal("unresolved alias witness was erased")
					}
					for _, n := range []int{1, 2} {
						r := v.FillPath(cue.ParsePath("x"), n).LookupPath(cue.ParsePath("out"))
						if _, err := r.MarshalJSON(); (err == nil) != (n == 2) {
							t.Fatalf("witness %d: %v", n, err)
						}
					}
				})
			}
		}
	}
}

func TestQuantifiedAliasErasureSelectionContexts(t *testing.T) {
	for _, tt := range []struct{ name, decl, use string }{
		{"direct", "At(A) = xs[A]", "At"},
		{"nested", "At(A) = xs[A]\nWrap(B) = At(B)", "Wrap"},
		{"scoped", "At(A) = xs[A]\nWrap(B) = {out: At(B)}.out", "Wrap"},
	} {
		for _, fixedFirst := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				generic := "f(T): func() -> int: " + tt.use + "(T)\n"
				fixed := "g: func() -> int: " + tt.use + "(0)\n"
				functions := generic + fixed
				if fixedFirst {
					functions = fixed + generic
				}
				v := cuecontext.New().CompileString("xs: [10, 20]\n" + tt.decl + "\n" + functions + "out: g()\nbad: f[0]()")
				if got, err := v.LookupPath(cue.ParsePath("out")).Int64(); err != nil || got != 10 {
					t.Fatalf("fixed alias application inherited another use's erasure check: %d, %v", got, err)
				}
				if err := v.LookupPath(cue.ParsePath("bad")).Err(); err == nil || !strings.Contains(err.Error(), "runtime index") {
					t.Fatalf("erased argument used as runtime index: %v", err)
				}
			})
		}
	}
	v := cuecontext.New().CompileString(`
id(A): func(x: A) -> A: x
Select(B) = id[B]
f(T): func(x: T) -> T: Select(T)(x)
out: f[int](3)
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).Int64(); err != nil || got != 3 {
		t.Fatalf("erased alias argument prevented type selection: %d, %v", got, err)
	}
}
