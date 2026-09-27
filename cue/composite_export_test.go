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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

func TestQuantifiedCompositeExport(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"alias", "Box(T) = {value: T}\nr(A): {f: func(x: A) -> Box(A): {value: x}}", `r[int].f(1).value`, `1`},
		{"record", `r(A): {f: func(x: A) -> A: x}`, `r[int].f(1)`, "1"},
		{"list", `r(A): [...A]`, `r[int]`, "[]"},
		{"telescope", `r(A, B): {f: func(x: A, y: B) -> [A, B]: [x, y]}`, `r[int][string].f(1, "s")`, `[1,"s"]`},
		{"selected", "base(A, B): {f: func(x: A, y: B) -> [A, B]: [x, y]}\nr: base[int]", `r[string].f(1, "s")`, `[1,"s"]`},
		{"excluded", "left(A: int): {f: func(x: A) -> A: x}\nright(B: string): {g: func(x: B) -> B: x}\nr: (left & right)[int]", `r.f(1)`, `1`},
		{"selected_refined", "base(A, B): {f: func(x: A, y: B) -> [A, B]: [x, y]}\nr: base[int] & {tag: 7}", `[r[string].f(1, "s"), r.tag]`, `[[1,"s"],7]`},
		{"refined", "base(A): {f: func(x: A) -> A: x}\nr: base & {tag: 7}", `[r[int].f(1), r.tag]`, `[1,7]`},
	} {
		for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
			t.Run(tt.name, func(t *testing.T) {
				ctx := cuecontext.New()
				v := ctx.CompileString(tt.source)
				src, err := format.Node(v.LookupPath(cue.ParsePath("r")).Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				r := ctx.CompileString("r: " + string(src) + "\nout: " + tt.call + "\ntooMany: r[int][int][int]")
				if r.LookupPath(cue.ParsePath("tooMany")).Validate(cue.Concrete(true)) == nil {
					t.Fatalf("consumed binders were reintroduced: %s", src)
				}
				if got, err := r.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != tt.want {
					t.Fatalf("export=%s: got %s, %v; want %s", src, got, err, tt.want)
				}
			})
		}
	}
}

func TestQuantifiedCompositeGraphExport(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
base(A): {f: func(x: A) -> A: x}
r: {first: base, copy: base, selected: base[int]}
`)
	for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
		src, err := format.Node(v.LookupPath(cue.ParsePath("r")).Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		r := ctx.CompileString("r: " + string(src) + `
out: [(r.first[int].f & r.copy[int].f)(1), (r.first[int].f & r.selected.f)(2), (r.copy[int].f & r.selected.f)(3)]`)
		if got, err := r.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != "[1,2,3]" {
			t.Fatalf("export=%s: got %s, %v", src, got, err)
		}
	}
}

func TestQuantifiedMixedCompositeExportIncomplete(t *testing.T) {
	for _, source := range []string{
		"base(A): {f: func(x: A) -> A: x}\nr: {whole: base, projected: base.f}",
		"base(A): {f: func(x: A) -> A: x}\nr: {projected: base.f, whole: base}",
		"x: int\nr(A): {f: func() -> int: x}",
	} {
		for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
			v := cuecontext.New().CompileString(source).LookupPath(cue.ParsePath("r"))
			if _, ok := v.Syntax(opts...).(*ast.BadExpr); !ok {
				src, _ := format.Node(v.Syntax(opts...))
				t.Fatalf("unrepresentable lexical graph was silently exported: %s", src)
			}
		}
	}
}

func TestQuantifiedCompositeFactoryExport(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`f: func(n: int) -> _: forall (A) {get: func(x: A) -> int: n}`)
	for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
		src, err := format.Node(v.LookupPath(cue.ParsePath("f")).Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		r := ctx.CompileString("f: " + string(src) + `
out: [(f(7))[int].get(1), (f(8))[string].get("s")]`)
		if got, err := r.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != "[7,8]" {
			t.Fatalf("export=%s: %s, %v", src, got, err)
		}
	}
}

func TestQuantifiedCompositeSelectionAcrossContexts(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`r: {
  let Q = forall (A, B) {f: func(x: A, y: B) -> [A, B]: [x, y]}
  Q[int]
 }`)
	for i := 0; i < 3; i++ {
		use := ctx.CompileString(`out: r[string].f(1, "s")`, cue.Scope(v))
		if got, err := use.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[1,"s"]` {
			t.Fatalf("context %d: %s, %v", i, got, err)
		}
		v = v.FillPath(cue.ParsePath("extra"), 1)
	}
}

// Captured declarations must survive repeated source conversion with enough
// type information to check the generated implementations independently.
func TestQuantifiedCaptureTypeExport(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"integer", `f:func(x:int)->(func(int)->int):func(y:int)->int:x+y`, `f(3)(2)`, `5`},
		{"record", `f:func(r:{a:int})->(func()->int):func()->int:r.a`, `f({a:3,b:true})()`, `3`},
		{"local_type_field", `f:func(r:{let N=int
a:N})->(func()->int):func()->int:r.a`, `f({a:3})()`, `3`},
		{"named_capture", `r:{a:3}
f:func()->int:r.a`, `f()`, `3`},
		{"alias", `N(A)={a:A}
f:func(r:N(int))->(func()->int):func()->int:r.a`, `f({a:3})()`, `3`},
		{"generic", `f(A):func(x:A)->(func()->A):func()->A:x`, `f(3)()`, `3`},
		{"nested_generic", `f(A):func(x:A)->(forall B func(B)->A):forall B func(y:B)->A:x`, `f(3)[string]("s")`, `3`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			source := tt.source
			for round := 0; round < 3; round++ {
				root := ctx.CompileString(source + "\nout: " + tt.call)
				f := root.LookupPath(cue.ParsePath("f"))
				if err := f.Validate(cue.Concrete(true)); err != nil {
					t.Fatalf("round %d: %v\n%s", round, err, source)
				}
				semanticJSON(t, root, "out", tt.want)
				text, err := format.Node(f.Syntax())
				if err != nil {
					t.Fatal(err)
				}
				source = "f: " + string(text)
			}
		})
	}
}

func TestQuantifiedCaptureExportPreservesOpenness(t *testing.T) {
	ctx := cuecontext.New()
	source := `f:func(r:{a:int})->(func()->close({a:int})):func()->close({a:int}):r`
	for round := 0; round < 3; round++ {
		root := ctx.CompileString(source)
		f := root.LookupPath(cue.ParsePath("f"))
		if !f.Exists() {
			t.Fatalf("invalid export: %v\n%s", root.Err(), source)
		}
		if err := f.Validate(cue.Concrete(true)); err == nil {
			t.Fatalf("round %d: an open capture acquired a closed type\n%s", round, source)
		}
		text, err := format.Node(f.Syntax())
		if err != nil {
			t.Fatal(err)
		}
		source = "f: " + string(text)
	}
}
