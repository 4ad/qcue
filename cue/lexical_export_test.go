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
	"cuelang.org/go/cue/format"
)

// Check observations after rebuilding exported source, including a hostile
// destination scope. Formatting alone cannot detect lost lexical constraints.
func TestQuantifiedLexicalExport(t *testing.T) {
	for _, tt := range []struct {
		name, source, extra, good, bad string
	}{
		{"closed", `#T: {a: int}; f: func(x: #T) -> int: x.a`, "", `f({a: 1})`, `f({a: 1, b: 2})`},
		{"optional", `#T: {a?: int}; f: func(x: #T) -> int: 0`, "", `f({})`, `f({a: "bad"})`},
		{"nested_closed", `#T: {r: {a: int}}; f: func(x: #T) -> int: x.r.a`, "", `f({r: {a: 1}})`, `f({r: {a: 1, b: 2}})`},
		{"schema_reference", `#U: int; #T: {a?: #U}; f: func(x: #T) -> int: 0`, "", `f({a: 2})`, `f({a: "bad"})`},
		{"schema_default", `#T: {a: *1 | 2}; f: func(x: #T) -> int: x.a`, "", `f({a: 2})`, `f({a: 3})`},
		{"pattern", `#T: {[string]: int}; f: func(x: #T) -> int: 0`, "", `f({a: 1})`, `f({a: "bad"})`},
		{"required", `#T: {a!: int}; f: func(x: #T) -> int: 0`, "", `f({a: 1})`, `f({})`},
		{"alias_result", `Box(A) = {value: A}; f(A): func(x: A) -> Box(A): {value: x}`, "", `f(1).value`, `f(1) & {value: "bad"}`},
		{"alias_body", `Box(A) = {value: A}; f: func(x: int) -> _: Box(int) & {value: x}`, "", `f(1).value`, `f(1) & {value: "bad"}`},
		{"alias_bound", `Box(A: int) = {value: A}; f(A): func(x: A) -> Box(A): {value: x}`, "", `f(1).value`, `f("bad")`},
		{"binder_alias_bound", `Box(A) = {value: A}; f(A: Box(int)): func(x: A) -> A: x`, "", `f({value: 1}).value`, `f({value: "bad"})`},
		{"alias_chain", `Box(A) = {value: A}; Wrap(A) = Box(A); f(A): func(x: A) -> Wrap(A): {value: x}`, "", `f(1).value`, `f(1) & {value: "bad"}`},
		{"alias_dependency", `x: 1; Box(A) = {value: A, tag: x}; f(A): func(x: A) -> Box(A): {value: x, tag: 1}`, "", `f(1).tag`, `f(1) & {tag: 2}`},
		{"let_witness", `x: 1; let Y = x & int; f: func(y: Y) -> int: y`, "", `f(1)`, `f(2)`},
		{"let_predicate", `let Nat = int & >=0; f: func(y: Nat) -> int: y`, "", `f(1)`, `f(-1)`},
	} {
		for _, final := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				ctx := cuecontext.New()
				v := ctx.CompileString(strings.ReplaceAll(tt.source, ";", "\n"))
				if err := v.Err(); err != nil {
					t.Fatal(err)
				}
				opts := []cue.Option{}
				if final {
					opts = append(opts, cue.Final())
				}
				src, err := format.Node(v.LookupPath(cue.ParsePath("f")).Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				rebuilt := ctx.CompileString("x: 2\n#T: _\nf: " + string(src) + "\n" + tt.extra + "\ngood: " + tt.good + "\nbad: " + tt.bad)
				if _, err := rebuilt.LookupPath(cue.ParsePath("good")).MarshalJSON(); err != nil {
					t.Fatalf("final=%v, export %s: good call: %v", final, src, err)
				}
				if err := rebuilt.LookupPath(cue.ParsePath("bad")).Validate(cue.Concrete(true)); err == nil {
					t.Fatalf("final=%v, export lost constraint: %s", final, src)
				}
			})
		}
	}
}

func TestQuantifiedContractExportScope(t *testing.T) {
	for _, tt := range []struct{ source, calls, want string }{
		{"x: 1\nf: func(x) -> 1", `[(f & (func(y: int) -> int: y))(1), (f & (func(y: int) -> int: y))(2)]`, `[1,2]`},
		{"#T: {a?: int}\nf: func(#T) -> 1", `[(f & (func(y: _) -> int: 1))({}), (f & (func(y: _) -> int: 2))({a: "bad"})]`, `[1,2]`},
	} {
		for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
			ctx := cuecontext.New()
			v := ctx.CompileString(tt.source)
			src, err := format.Node(v.LookupPath(cue.ParsePath("f")).Syntax(opts...))
			if err != nil {
				t.Fatal(err)
			}
			r := ctx.CompileString("x: 2\n#T: _\nf: " + string(src) + "\nout: " + tt.calls)
			if got, err := r.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != tt.want {
				t.Fatalf("contract %s: got %s, %v; want %s", src, got, err, tt.want)
			}
		}
	}
}

func TestQuantifiedUnresolvedWitnessExport(t *testing.T) {
	for _, source := range []string{
		"x: int\nlet Y = x & int\nf: func(y: Y) -> int: 0",
		"x: int\nf: func(x) -> 1",
		"x: int\nAlias(A) = x & A\nf: func(y: Alias(int)) -> int: 0",
	} {
		for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
			ctx := cuecontext.New()
			src, err := format.Node(ctx.CompileString(source).LookupPath(cue.ParsePath("f")).Syntax(opts...))
			if err != nil {
				t.Fatal(err)
			}
			r := ctx.CompileString("x: 2\nf: " + string(src) + "\nf: func(y: int) -> int: y\nout: f(2)")
			if r.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true)) == nil {
				t.Fatalf("unresolved witness acquired a destination binding: %s", src)
			}
		}
	}
}
