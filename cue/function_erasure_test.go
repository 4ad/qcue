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
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
)

// A result annotation is a proposition about the independently evaluated
// body. It cannot supply fields, choose alternatives, or solve body cycles.
func TestQuantifiedResultErasure(t *testing.T) {
	for _, tt := range []struct{ name, source string }{
		{"inferred", `f(A): func(xs: [...A]) -> {b: A}: {a: xs[0], c: self.b}
out: f([1, 2, 3])`},
		{"selected", `f(A): func() -> {b: A}: {c: self.b}
out: f[1]()`},
		{"universal", `f(A): func(x: A) -> {a: 1}: x
out: f({x: "x"})`},
		{"field", `out: (func() -> {a: 1}: {})()`},
		{"nested", `out: (func() -> {n: {a: 1}}: {n: {}})()`},
		{"list", `out: (func() -> [{a: 1}]: [{}])()`},
		{"choice", `out: (func() -> 1: 1 | 2)()`},
		{"default", `out: (func() -> (*1 | int): int)()`},
		{"cycle", `out: (func() -> {a: 1}: {a: self.a})()`},
		{"attached", `f: (func() -> {}: {}) & (func() -> {a: 1})
out: f()`},
		{"partial", `f: (func(x: int) -> {}: {})(1, ...) & (func() -> {a: 1})
out: f()`},
		{"builtin", `import "encoding/json"
f: json.Unmarshal & (func(string) -> {a: 1})
out: f("{}")`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := semanticValue(t, tt.source)
			out := v.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("result annotation constructed a successful result")
			}
			if b, err := out.MarshalJSON(); err == nil {
				t.Fatalf("result annotation materialized %s", b)
			}
		})
	}
}

// Parameter predicates must not manufacture a packet either, even if the
// body ignores it. Defaults belong to the implementation's omission protocol;
// defaults inside a predicate cannot select an argument on the caller's behalf.
func TestQuantifiedArgumentErasure(t *testing.T) {
	for _, tt := range []struct{ name, predicate, argument string }{
		{"field", `{a: 1}`, `{}`},
		{"nested", `{n: {a: 1}}`, `{n: {}}`},
		{"list", `[{a: 1}]`, `[{}]`},
		{"choice", `1`, `1 | 2`},
		{"default", `*1 | int`, `int`},
		{"cycle", `{a: 1}`, `{a: self.a}`},
	} {
		for _, body := range []string{"x", "0"} {
			for _, call := range []string{"f(arg)", "f(arg, ...)()", "f(x: arg)"} {
				t.Run(tt.name+"/"+body+"/"+call, func(t *testing.T) {
					v := semanticValue(t, fmt.Sprintf("f: func(x: (%s)) -> _: %s\narg: %s\nout: %s",
						tt.predicate, body, tt.argument, call))
					out := v.LookupPath(cue.ParsePath("out"))
					if _, err := out.MarshalJSON(); err == nil {
						t.Fatal("parameter predicate manufactured an admitted packet")
					}
				})
			}
		}
	}
}

// Type selection, inference, saved packets, and equivalent source exports
// must implement the same identity operation on every admitted concrete value.
// Compare against the supplied data, not against another annotated call.
func TestQuantifiedCallErasureIdentity(t *testing.T) {
	for _, argument := range []string{"0", "1", `"s"`, "true", "null", "{}", "{n: 1}", "[1, 2]"} {
		for _, selected := range []string{"id", "id[_]", "id[" + argument + "]"} {
			for _, call := range []string{"f(arg)", "f(arg, ...)()", "f(x: arg)"} {
				t.Run(argument+"/"+selected+"/"+call, func(t *testing.T) {
					v := semanticValue(t, "id(A): func(x: A) -> A: x\nf: "+selected+"\narg: "+argument+"\nout: "+call)
					want, err := v.LookupPath(cue.ParsePath("arg")).MarshalJSON()
					if err != nil {
						t.Fatal(err)
					}
					semanticJSON(t, v, "out", string(want))
					for _, options := range [][]cue.Option{nil, {cue.Final()}} {
						source, err := format.Node(v.LookupPath(cue.ParsePath("f")).Syntax(options...))
						if err != nil {
							t.Fatal(err)
						}
						rebuilt := semanticValue(t, "f: "+string(source)+"\narg: "+argument+"\nout: "+call)
						semanticJSON(t, rebuilt, "out", string(want))
					}
				})
			}
		}
	}
}

// Erasure includes negative information and future refinements, not just JSON
// fields. An absent optional field or pattern in a contract cannot become a
// constraint on the returned record or on data captured by a returned closure.
func TestQuantifiedCallErasureRefinement(t *testing.T) {
	for _, predicate := range []string{`{a?: A}`, `{[string]: A}`, `{#T: A}`} {
		argument := "{}"
		if predicate == "{#T: A}" {
			argument = "{#T: _}"
		}
		for _, body := range []string{"x", "(func() -> _: x)()"} {
			t.Run(predicate+"/"+body, func(t *testing.T) {
				v := semanticValue(t, fmt.Sprintf(`
f(A): func(x: %s) -> _: %s
out: [f[1](%s), f[2](%s)]
`, predicate, body, argument, argument))
				for i := range 2 {
					out := v.LookupPath(cue.MakePath(cue.Str("out"), cue.Index(i)))
					refinement := `{a: "independent"}`
					if argument != "{}" {
						refinement = "{#T: 3}"
					}
					if err := out.Unify(out.Context().CompileString(refinement)).Validate(cue.Concrete(true)); err != nil {
						t.Fatalf("selected predicate leaked into the value: %v", err)
					}
				}
			})
		}
	}
}

// A successful instance cannot discharge a universal implementation proof.
// This is checked before and after execution and on exported selected views.
func TestQuantifiedCallErasureUniversalProof(t *testing.T) {
	for _, source := range []string{
		`f(A): func() -> A: {v: 1}.v
g: f[1]
out: g()`,
		`f(A): func(x: A) -> {a: 1}: x
g: f[{a: 1}]
out: g({a: 1})`,
		`f(A): func(x: A) -> A: {v: 1}.v
g: f[1]
out: g(1)`,
	} {
		v := semanticValue(t, source)
		for range 2 {
			for _, path := range []string{"f", "g"} {
				f := v.LookupPath(cue.ParsePath(path))
				if err := f.Validate(cue.Concrete(true)); err == nil {
					t.Fatalf("certified an invalid universal at %s: %s", path, source)
				}
			}
			if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil {
				t.Fatalf("the selected instance should satisfy the runtime check: %v", err)
			}
		}
		for _, options := range [][]cue.Option{nil, {cue.Final()}} {
			src, err := format.Node(v.LookupPath(cue.ParsePath("g")).Syntax(options...))
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := semanticValue(t, "g: "+string(src)).LookupPath(cue.ParsePath("g"))
			if rebuilt.Validate(cue.Concrete(true)) == nil {
				t.Fatalf("export discharged an invalid universal: %s", src)
			}
		}
	}
}

// Pure membership evidence may justify elimination, but cannot become a new
// seal or hide fields on the supplied value. Copies and call results preserve
// the evidence needed to open a covariant existential again.
func TestQuantifiedCallErasureExistentialEvidence(t *testing.T) {
	for _, expr := range []string{"id({x: 1})", "make()", "id({x: 1}, ...)()", "saved()", "copy"} {
		v := semanticValue(t, `
#M: exists A {x: A}
id: func(p: #M) -> _: p
make: func() -> #M: {x: 1}
capture: func(p: #M) -> _: func() -> _: p
saved: capture({x: 1})
copy: make() & {extra: 2}
out: (open (`+expr+`) as (A, P) {value: 7}).value
raw: (`+expr+`).x
`)
		semanticJSON(t, v, "out", "7")
		semanticJSON(t, v, "raw", "1")
		for _, options := range [][]cue.Option{nil, {cue.Final()}} {
			source, err := format.Node(v.LookupPath(cue.ParsePath("saved")).Syntax(options...))
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := semanticValue(t, "saved: "+string(source)+"\nout: (open saved() as (A, P) {value: 7}).value")
			semanticJSON(t, rebuilt, "out", "7")
		}
	}
}
