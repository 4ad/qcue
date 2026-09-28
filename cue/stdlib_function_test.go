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

func TestStdlibSelectedIdentity(t *testing.T) {
	for _, expr := range []string{
		`list.Reverse[int] & list.Reverse[string]`,
		`list.Reverse[string] & list.Reverse[int]`,
		`(list.Reverse[int] & list.Reverse[string]) & list.Reverse[int]`,
		`list.Reverse[int] & (list.Reverse[string] & list.Reverse[int])`,
	} {
		t.Run(expr, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"list\"\nf: " + expr + "\nout:[f([1,2]),f([\"a\",\"b\"])]")
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", `[[2,1],["b","a"]]`)
				for _, bad := range []string{`bad:f([true])`, `bad:f[bool]`} {
					if v.Unify(ctx.CompileString("f:_\n"+bad)).Validate() == nil {
						t.Fatalf("accepted %s", bad)
					}
				}
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
				if err := v.Validate(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
			}
		})
	}
	for _, expr := range []string{
		`list.Reverse & list.Reverse[int]`,
		`list.Reverse[int] & list.Reverse`,
	} {
		checkOperatorDefinition(t, "import \"list\"\nf:"+expr, true)
	}
	checkOperatorDefinition(t, "import \"list\"\nf:list.Reverse[int] & list.Take[int]", false)
	ctx := cuecontext.New()
	// Separate compilations use distinct operation contexts. Native identity
	// comes from the registered implementation, not the checking adapter.
	v := ctx.CompileString("import \"list\"\nf:list.Reverse[int]")
	w := ctx.CompileString("import \"list\"\nf:list.Reverse[string]")
	merged := v.Unify(w).Unify(ctx.CompileString(`f:_
out:[f([1,2]),f(["a","b"])]`))
	if err := merged.Validate(); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, merged, "out", `[[2,1],["b","a"]]`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		source, err := format.Node(merged.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := ctx.CompileBytes(source)
		if err := rebuilt.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
		semanticJSON(t, rebuilt, "out", `[[2,1],["b","a"]]`)
	}
}

func TestStdlibTypeApplication(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"select", `f:list.Reverse[int]`, true},
		{"generic_forward", `f(A):func(xs:[...A])->[...A]:list.Reverse[A](xs)`, true},
		{"generic_wrong", `f(A):func(xs:[...A])->[...string]:list.Reverse[A](xs)`, false},
		{"selected_call", `f:func(xs:[...int])->[...int]:list.Reverse[int](xs)`, true},
		{"selected_wrong_argument", `f:func(xs:[...string])->_:list.Reverse[int](xs)`, false},
		{"selected_wrong_result", `f:func(xs:[...int])->[...string]:list.Reverse[int](xs)`, false},
		{"selected_partial", `f:func(xs:[...int])->func()->[...int]:list.Reverse[int](xs,...)`, true},
		{"selected_partial_wrong_argument", `f:func(xs:[...string])->func()->[...int]:list.Reverse[int](xs,...)`, false},
		{"bound", `f:list.Max[int]`, true},
		{"wrong_bound", `f:list.Max[string]`, false},
		{"extra_argument", `f:list.Reverse[int][string]`, false},
		{"nongeneric", `f:list.Sum[int]`, false},
		{"alias", `r:list.Reverse
f:func(xs:[...int])->[...int]:r[int](xs)`, true},
		{"capability", `f:list.Reverse[int] & (func([...int])->[...int])`, true},
		{"false_capability", `f:list.Reverse[int] & (func([...int])->[...string])`, false},
		{"retained_obligation", `r:list.Reverse & (forall(A) func([...A])->[...string])
f:r[string]`, false},
		{"tuple_capability", `r:list.Reverse & (forall(A,B) func([A,B])->[B,A])
f:r[int][string]`, true},
		{"tuple_call", `r:list.Reverse & (forall(A,B) func([A,B])->[B,A])
f:func(xs:[int,string])->[string,int]:r[int][string](xs)`, true},
		{"primitive_capability", `merge:and & (forall(A) func([A,...A])->A)
f:func(xs:[int,...int])->int:merge[int](xs)`, true},
		{"primitive_wrong_argument", `merge:and & (forall(A) func([A,...A])->A)
f:func(xs:[string,...string])->int:merge[int](xs)`, false},
		{"primitive_wrong_result", `merge:and & (forall(A) func([A,...A])->A)
f:func(xs:[int,...int])->string:merge[int](xs)`, false},
		{"primitive_partial", `merge:and & (forall(A) func([A,...A])->A)
f:func(xs:[int,...int])->func()->int:merge[int](xs,...)`, true},
		{"primitive_partial_empty", `merge:and & (forall(A) func([A,...A])->A)
f:func()->func()->int:merge[int]([],...)`, false},
		{"primitive_partial_wrong_argument", `merge:and & (forall(A) func([A,...A])->A)
f:func(xs:[string,...string])->func()->int:merge[int](xs,...)`, false},
		{"native_identity_argument", `accept:func(r:list.Reverse[int])->[...int]:r([1,2])
f:func()->[...int]:accept(list.Reverse)`, true},
		{"native_identity_instance", `accept:func(r:list.Reverse[int])->[...int]:r([1,2])
f:func()->[...int]:accept(list.Reverse[int])`, true},
		{"native_identity_domain", `accept:func(r:list.Reverse)->[...int]:r([1,2])
f:func()->[...int]:accept(list.Reverse[int])`, false},
		{"native_identity_wrong_argument", `accept:func(r:list.Reverse)->[...int]:r([1,2])
f:func()->[...int]:accept(list.Take[int])`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source
			if strings.Contains(source, "list.") {
				source = "import \"list\"\n" + source
			}
			checkOperatorDefinition(t, source, tc.valid)
		})
	}
}

func TestStdlibTypeApplicationExecution(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
r:list.Reverse[int]
reverse(A):func(xs:[...A])->[...A]:list.Reverse[A](xs)
apply(A,B):func(fn:func(A)->B,x:A)->B:fn(x)
out:[r(x:[1,2]),reverse(["a","b"]),apply(r,[3,4]),list.Max[int]([1,2])]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
		want := `[[2,1],["b","a"],[4,3],2]`
		if err != nil || string(got) != want {
			t.Fatalf("got %s, %v; want %s", got, err, want)
		}
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
		bad := v.Unify(ctx.CompileString(`r:_
bad:r(["wrong"])`))
		if bad.Validate() == nil {
			t.Fatalf("export lost the selected domain: %s", source)
		}
	}
}

func TestStdlibTypeApplicationCaptures(t *testing.T) {
	for _, callee := range []string{
		`list.Reverse[int]`,
		`list.Reverse & list.Reverse[int]`,
		`list.Reverse[int] & list.Reverse`,
	} {
		t.Run(callee, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(`import "list"
make:func(r:func([...int])->[...int])->func([...int])->[...int]:func(xs:[...int])->[...int]:r(xs)
f:make(` + callee + `)
input:[int,int]
out:f(input)`)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
					t.Fatal("checking metadata materialized an incomplete operand")
				}
				filled := v.FillPath(cue.ParsePath("input"), []int{1, 2})
				semanticJSON(t, filled, "out", `[2,1]`)
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
				if err := v.Validate(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
			}
		})
	}
}

func TestStdlibCallbackRecordAnnotations(t *testing.T) {
	for _, tc := range []struct {
		source string
		valid  bool
	}{
		{`C2(T) = {c2:func(int)->int, f2:func(x:int)->int:c2(x)}
C1(T) = {c1:func(int)->int, f1:({e:C2(_) & {c2:c1}, result:e.f2}).result}
f:func(cb:func(int)->int)->func(int)->int:(C1(_) & {c1:cb}).f1`, true},
		{`f:func(cb:func(int)->int)->int:({r:func(int)->int} & {r:cb}).r(1)`, true},
		{`f:func(cb:func(int)->int)->int:({r:cb} & {r:func(int)->int}).r(1)`, true},
		{`f:func(cb:func(int)->int)->string:({r:func(int)->string} & {r:cb}).r(1)`, false},
		{`f:func(cb:func(int)->int)->string:({r:cb} & {r:func(int)->string}).r(1)`, false},
		{`f:func()->int:({r:func(int)->int} & {r:func(int)->int}).r(1)`, false},
		{`f:func()->int:({r:func(int)->int} & {r:func(x:int)->int:"bad"}).r(1)`, false},
		{`f:func()->int:({r:func(x:int)->int:"bad"} & {r:func(int)->int}).r(1)`, false},
	} {
		t.Run(tc.source, func(t *testing.T) { checkOperatorDefinition(t, tc.source, tc.valid) })
	}
}

func TestStdlibNativePacketOrder(t *testing.T) {
	for _, callee := range []string{`list.Drop`, `list.Drop[int]`} {
		for _, packet := range []struct {
			args  string
			valid bool
		}{
			{`xs,1`, true},
			{`x:xs,n:1`, true},
			{`xs,n:1`, true},
			{`n:1,x:xs`, true},
			{`xs,x:xs`, false},
			{`n:1,n:1`, false},
			{`xs,unknown:1`, false},
			{`x:xs,n:"bad"`, false},
			{`n:1,x:"bad"`, false},
			{`n:1`, false},
			{`xs,1,1`, false},
		} {
			source := "import \"list\"\nf:func(xs:[...int])->[...int]:" + callee + "(" + packet.args + ")"
			t.Run(callee+"/"+packet.args, func(t *testing.T) {
				checkOperatorDefinition(t, source, packet.valid)
				if packet.valid {
					v := cuecontext.New().CompileString(source + "\nout:f([1,2,3])")
					semanticJSON(t, v, "out", `[2,3]`)
				}
			})
		}
	}
}
