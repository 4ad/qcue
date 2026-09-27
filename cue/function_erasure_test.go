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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

// A result annotation is a proposition about the independently evaluated
// body. It cannot supply fields, choose alternatives, or solve body cycles.
func TestQuantifiedResultErasure(t *testing.T) {
	for _, tt := range []struct{ name, source string }{
		{"inferred", `f(A): func(xs: [...A]) -> {b: A}: {a: xs[0], c: self.b}
out: f([1, 2, 3])`},
		{"inferred_refined", `f(A): func(xs: [...A]) -> {b: A}: {a: xs[0], c: self.b}
out: f([1, 2, 3]) & {b: 1}`},
		{"selected", `f(A): func() -> {b: A}: {c: self.b}
out: f[1]()`},
		{"universal", `f(A): func(x: A) -> {a: 1}: x
out: f({x: "x"})`},
		{"field", `out: (func() -> {a: 1}: {})()`},
		{"hidden", `out: (func() -> {_a: 1}: {})()`},
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
	t.Run("definition", func(t *testing.T) {
		v := semanticValue(t, `out: (func() -> {#A: 1}: {})()`)
		semanticJSON(t, v, "out", `{}`)
		if v.LookupPath(cue.ParsePath("out.#A")).Exists() {
			t.Fatal("result annotation materialized a definition")
		}
	})
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

// Empty packets, omission defaults, and captured empty records supply no
// inhabitants of A. Different admitted selections must stay observationally
// identical, including when checked views are conjoined in either order.
func TestQuantifiedCallErasureConstant(t *testing.T) {
	for _, selection := range []string{"f[1]", "f[2]", "f[1] & f[2]", "f[2] & f[1]"} {
		for _, implementation := range []string{
			`func() -> {}: {}`,
			`func(x: {v?: A} = {}) -> _: x`,
			`func(x: {v?: A} = {}) -> _: (func() -> _: x)()`,
		} {
			v := semanticValue(t, "f(A): "+implementation+"\ng: "+selection+"\nout: g() & {v: true}")
			semanticJSON(t, v, "out", `{"v":true}`)
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
			out := v.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(); err == nil {
				t.Fatal("ordinary validation accepted an uncertified call")
			}
			if _, err := out.MarshalJSON(); err == nil {
				t.Fatal("a successful instance bypassed the universal proof")
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
		t.Run(expr, func(t *testing.T) {
			v := semanticValue(t, `
#M: exists A {x: A}
id: func(p: #M) -> _: p
make: func() -> #M: {x: 1}
capture: func(p: #M) -> _: func() -> _: p
saved: capture({x: 1})
copy: make() & {extra: 2}
value: `+expr+`
out: (open value as (A, P) {value: 7}).value
raw: value.x
`)
			semanticJSON(t, v, "out", "7")
			semanticJSON(t, v, "raw", "1")
			for _, path := range []string{"saved", "value"} {
				for mode, options := range [][]cue.Option{nil, {cue.Final()}, {cue.Concrete(true)}} {
					source, err := format.Node(v.LookupPath(cue.ParsePath(path)).Syntax(options...))
					if err != nil {
						t.Fatal(err)
					}
					use := path
					if path == "saved" {
						use += "()"
					}
					rebuilt := semanticValue(t, path+": "+string(source)+"\nout: (open "+use+" as (A, P) {value: 7}).value")
					t.Logf("%s mode %d: %s", path, mode, source)
					semanticJSON(t, rebuilt, "out", "7")
				}
			}
		})
	}
}

// Evidence can become inapplicable after refinement without becoming a new
// constraint. Export must not resurrect it as an obligation on the value.
func TestQuantifiedCallErasureRefinedEvidence(t *testing.T) {
	v := semanticValue(t, `
#M: exists A {x: A, optional?: 1}
make: func() -> #M: {x: 1}
out: make() & {optional: 2}
`)
	semanticJSON(t, v, "out", `{"x":1,"optional":2}`)
	for _, options := range [][]cue.Option{nil, {cue.Final()}} {
		source, err := format.Node(v.LookupPath(cue.ParsePath("out")).Syntax(options...))
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := semanticValue(t, "out: "+string(source))
		semanticJSON(t, rebuilt, "out", `{"x":1,"optional":2}`)
	}
}

// An independent finite denotation checks both acceptance and erasure. A
// packet has two presence bits and fields drawn from a finite alphabet; a
// predicate has two presence requirements and sets of permitted field values.
// Expectations use only those bits and sets, never CUE unification or proofs.
func TestQuantifiedCallErasureMembershipModel(t *testing.T) {
	width := 2
	if extendedQuantifiedOracle(t) {
		width = 3
	}
	ctx := cuecontext.New()
	cases := 0
	for maskX := 1; maskX < 1<<width; maskX++ {
		for maskY := 1; maskY < 1<<width; maskY++ {
			for optional := range 4 {
				var fields []string
				for i, mask := range []int{maskX, maskY} {
					var values []string
					for v := range width {
						if mask&(1<<v) != 0 {
							values = append(values, fmt.Sprint(v))
						}
					}
					mark := ""
					if optional&(1<<i) != 0 {
						mark = "?"
					}
					fields = append(fields, fmt.Sprintf("%s%s: %s", []string{"x", "y"}[i], mark, strings.Join(values, " | ")))
				}
				predicate := "{" + strings.Join(fields, ", ") + "}"
				for x := -1; x < width; x++ {
					for y := -1; y < width; y++ {
						packet := map[string]int{}
						admitted := true
						for i, v := range []int{x, y} {
							if v < 0 {
								admitted = admitted && optional&(1<<i) != 0
							} else {
								packet[[]string{"x", "y"}[i]] = v
								admitted = admitted && []int{maskX, maskY}[i]&(1<<v) != 0
							}
						}
						data, err := json.Marshal(packet)
						if err != nil {
							t.Fatal(err)
						}
						for _, program := range []string{
							"out: (func(p: " + predicate + ") -> _: p)(" + string(data) + ")",
							"out: (func() -> " + predicate + ": " + string(data) + ")()",
							"f: (func(p: " + predicate + ") -> _: p) & (func(" + predicate + ") -> " + predicate + ")\nout: f(" + string(data) + ")",
							"f: (func(p: " + predicate + ") -> _: p) & (func(" + predicate + ") -> " + predicate + ")\nout: f(" + string(data) + ", ...)()",
						} {
							if cases%256 == 0 {
								ctx = cuecontext.New()
							}
							cases++
							out := ctx.CompileString(program).LookupPath(cue.ParsePath("out"))
							got, err := out.MarshalJSON()
							if admitted {
								if err != nil || string(got) != string(data) {
									t.Fatalf("admitted packet changed: got %s, %v; want %s\n%s", got, err, data, program)
								}
							} else if err == nil || out.Validate() == nil {
								t.Fatalf("contract must reject this complete packet: got %s, %v\n%s", got, err, program)
							}
						}
						// These original unrestricted identities promised the
						// record result for every input. A matching example packet
						// cannot justify that invalid universal obligation.
						for _, call := range []string{"f(" + string(data) + ")", "f(" + string(data) + ", ...)()"} {
							program := "f: (func(p: _) -> _: p) & (func(_) -> " + predicate + ")\nout: " + call
							v := ctx.CompileString(program)
							if v.LookupPath(cue.ParsePath("f")).Validate() == nil || v.LookupPath(cue.ParsePath("out")).Validate() == nil {
								t.Fatalf("an unrestricted identity acquired a record guarantee: %s", program)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("checked %d call boundaries over %d field values", cases, width)
}
