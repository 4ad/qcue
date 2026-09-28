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
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

func TestStdlibSelectedComparers(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"int", `f:func(xs:[...int])->[...int]:list.Sort[int](xs,list.Ascending)`, true},
		{"generic", `f(A:number):func(xs:[...A])->[...A]:list.Sort[A](xs,list.Ascending)`, true},
		{"stable", `f:func(xs:[...string])->[...string]:list.SortStable[string](xs,list.Ascending)`, true},
		{"alias", `sort:list.Sort[int]
f:func(xs:[...int])->[...int]:sort(cmp:list.Ascending,list:xs)`, true},
		{"record", `f(A:{a:int}):func(xs:[...A])->[...A]:list.Sort[A](xs,{x:{},y:{},less:x.a<y.a})`, true},
		{"wrong_field", `f(A:{a:int}):func(xs:[...A])->[...A]:list.Sort[A](xs,{x:{},y:{},less:x.b<y.b})`, false},
		{"wrong_selected_type", `f:func(xs:[...string])->[...string]:list.Sort[int](xs,list.Ascending)`, false},
		{"wrong_result", `f:func(xs:[...int])->[...string]:list.Sort[int](xs,list.Ascending)`, false},
		{"wrong_comparer_domain", `f:func(xs:[...string])->[...string]:list.Sort[string](xs,{x:int,y:int,less:x<y})`, false},
		{"wrong_comparer_result", `f:func(xs:[...int])->[...int]:list.Sort[int](xs,{x:int,y:int,less:42})`, false},
		{"mixed_elements", `f:func(xs:[...(int|string)])->[...(int|string)]:list.Sort[int|string](xs,list.Ascending)`, false},
		{"tuple", `f:func(xs:[int,int])->[int,int]:list.Sort[int](xs,list.Ascending)`, true},
		{"saved_list", `f:func(xs:[...int])->[...int]:list.Sort[int](xs,...)(list.Ascending)`, true},
		{"saved_list_generic", `f(A:number):func(xs:[...A])->[...A]:list.Sort[A](xs,...)(cmp:list.Ascending)`, true},
		{"saved_list_wrong_comparer", `f:func(xs:[...string])->[...string]:list.Sort[string](xs,...)({x:int,y:int,less:x<y})`, false},
		{"saved_both", `f:func(xs:[...int])->[...int]:list.Sort[int](xs,list.Ascending,...)()`, true},
		{"saved_comparer", `f:func(xs:[...int])->[...int]:list.Sort(cmp:list.Ascending,...)(xs)`, true},
		{"saved_comparer_generic", `f(A:number):func(xs:[...A])->[...A]:list.Sort[A](cmp:list.Ascending,...)(xs)`, true},
		{"saved_comparer_wrong_domain", `f:func(xs:[...string])->[...string]:list.Sort(cmp:{x:int,y:int,less:x<y},...)(xs)`, false},
		{"saved_comparer_wrong_result", `f:func(xs:[...int])->[...string]:list.Sort(cmp:list.Ascending,...)(xs)`, false},
		{"saved_comparer_interface", `f:list.Sort(cmp:list.Ascending,...) & (func([...int])->[...int])`, true},
		{"saved_comparer_false_interface", `f:list.Sort(cmp:list.Ascending,...) & (func([...bool])->[...bool])`, false},
		{"saved_comparer_universal_interface", `f:list.Sort(cmp:list.Ascending,...) & (forall(A) func([...A])->[...A])`, false},
		{"saved_comparer_missing_less", `f:list.Sort(cmp:{x:int,y:int},...)`, false},
		{"saved_comparer_bad_operation", `f:list.Sort(cmp:{less:true+1},...)`, false},
		{"empty", `f:func()->[]:list.Sort[int]([],{less:42})`, true},
		{"empty_bad_operation", `f:func()->[]:list.Sort[int]([],{less:true+1})`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, "import \"list\"\n"+tc.source, tc.valid) })
	}
}

func TestStdlibSelectedComparerExecution(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
sort:list.Sort[int]
saved:sort([3,1,2],...)
f(A:number):func(xs:[...A])->[...A]:list.Sort[A](xs,list.Ascending)
out:[sort([3,1,2],list.Ascending),f([4,2,3]),list.Sort[{a:int}]([{a:2},{a:1}],{x:{},y:{},less:x.a<y.a}),saved(cmp:list.Ascending)]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[[1,2,3],[2,3,4],[{"a":1},{"a":2}],[1,2,3]]`)
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
	}
}

func TestStdlibComparerConstructors(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"ascending", `f:func()->[...]:list.IsSorted(list.Ascending)`, true},
		{"named", `f:func()->[...]:list.IsSorted(cmp:list.Ascending)`, true},
		{"record", `f:func()->[...]:list.IsSorted({x:{a:int},y:{a:int},less:x.a<y.a})`, true},
		{"wrong_label", `f:func()->[...]:list.IsSorted(list:list.Ascending)`, false},
		{"wrong_kind", `f:func()->[...]:list.IsSorted(42)`, false},
		{"missing_less", `f:func()->[...]:list.IsSorted({x:int,y:int})`, false},
		{"optional_less", `f:func()->[...]:list.IsSorted({x:int,y:int,less?:bool})`, false},
		{"bad_operation", `f:func()->[...]:list.IsSorted({less:true+1})`, false},
		{"wrong_result", `f:func()->string:list.IsSorted(list.Ascending)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, "import \"list\"\n"+tc.source, tc.valid) })
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
f:func()->[...]:list.IsSorted(cmp:list.Ascending)
out:[1,2,3] & f()
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[1,2,3]`)
		bad := v.Unify(ctx.CompileString(`f:_
bad:[3,2,1] & f()`))
		if bad.Validate() == nil {
			t.Fatal("sorted validator accepted descending input")
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
}

func TestStdlibSavedComparerExecution(t *testing.T) {
	for _, declaration := range []string{
		`sort:list.Sort(cmp:list.Ascending,...)`,
		`sort:list.Sort[int](cmp:list.Ascending,...)`,
		`views:list.Sort[int] & list.Sort[string]
sort:views(cmp:list.Ascending,...)`,
		`view:list.Sort & (func(cmp!:{x:_,y:_,less:bool},list!:[...int])->[...int])
saved:view(cmp:list.Ascending,...)
sort:func(xs:[...int])->[...int]:saved(list:xs)`,
		`make:func(n:int)->func([...int])->[...int]:list.Sort(cmp:{x:int,y:int,less:x+n<y+n},...)
sort:make(3)`,
		`make:func(n:int)->func([...int])->[...int]:list.Sort(cmp:{x:int,y:int,less:x+n<y+n},...)
sort:make(3) & make(3)`,
		`make:func(n:int,unused:string)->func([...int])->[...int]:list.Sort(cmp:{x:int,y:int,less:x+n<y+n},...)
sort:make(3,"first") & make(3,"second")`,
		`make:func(n:int)->func([...int])->[...int]:{
cmp:{x:int,y:int,less:x+n<y+n}
out:list.Sort(cmp:cmp,...)
}.out
sort:make(3) & make(3)`,
		`make:func(n:int)->func([...int])->[...int]:{
cmp:{x:int,y:int,less:x+n<y+n}
first:list.Sort[int](...)
view:first & (func(cmp!:{x:int,y:int,less:bool},list!:[...int])->[...int])
out:list.Sort[int](cmp:cmp,...) & view(cmp:cmp,...)
}.out
sort:make(3)`,
		`wrap:func(f:func([...int])->[...int])->func([...int])->[...int]:func(xs:[...int])->[...int]:f(xs)
sort:wrap(list.Sort(cmp:list.Ascending,...))`,
		`make:func(n:int)->func([...int])->[...int]:list.Sort(cmp:{x:int,y:int,less:x+n<y+n},...)
wrap:func(f:func([...int])->[...int])->func([...int])->[...int]:func(xs:[...int])->[...int]:f(xs)
sort:wrap(make(3))`,
	} {
		t.Run(declaration, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"list\"\n" + declaration + "\nout:sort([3,1,2])")
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", `[1,2,3]`)
				bad := v.Unify(ctx.CompileString(`sort:_
bad:sort([true,false])`))
				if bad.Validate() == nil {
					t.Fatal("saved comparator accepted incompatible list elements")
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
}

func TestStdlibComparerRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
reverse: bool
sort: list.Sort(cmp: {x: int, y: int, less: (x < y) != reverse}, ...)
wrap: func(f: func([...int])->[...int])->func([...int])->[...int]: func(xs:[...int])->[...int]: f(xs)
out: wrap(sort)([3,1,2])
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
			t.Fatal("checking completed an unresolved comparator capture")
		}
		semanticJSON(t, v.FillPath(cue.ParsePath("reverse"), false), "out", `[1,2,3]`)
		semanticJSON(t, v.FillPath(cue.ParsePath("reverse"), true), "out", `[3,2,1]`)
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Err(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
	}
}

func TestStdlibPartialIntermediateComparer(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
make:func(n:int)->func()->[...int]:{
cmp:{x:int,y:int,less:x+n<y+n}
first:list.Sort[int](list:[3,1,2],...)
view:first & (func(cmp!:{x:int,y:int,less:bool})->[...int])
out:list.Sort[int](list:[3,1,2],cmp:cmp,...) & view(cmp:cmp,...)
}.out
saved:make(3)
out:saved()`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}, nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[1,2,3]`)
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
	}
}

func TestStdlibSharedComparerCaptures(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
template:{
reverse:bool
sort:{
cmp:{x:int,y:int,less:(x<y)!=reverse}
first:list.Sort[int](...)
view:first & (func(cmp!:{x:int,y:int,less:bool},list!:[...int])->[...int])
out:list.Sort[int](cmp:cmp,...) & view(cmp:cmp,...)
}.out
out:sort([3,1,2])
}
a:template
b:template`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		filled := v.FillPath(cue.ParsePath("a.reverse"), false).FillPath(cue.ParsePath("b.reverse"), true)
		semanticJSON(t, filled, "a.out", `[1,2,3]`)
		semanticJSON(t, filled, "b.out", `[3,2,1]`)
		if _, err := v.LookupPath(cue.ParsePath("template.out")).MarshalJSON(); err == nil {
			t.Fatal("export completed a template's live capture")
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
}

func TestStdlibComparerDistinctTemplates(t *testing.T) {
	for _, source := range []string{
		`a:list.Sort(cmp:list.Ascending,...)
b:list.Sort(cmp:list.Descending,...)`,
		`make:func(reverse:bool)->func([...int])->[...int]:list.Sort(cmp:{x:int,y:int,less:(x<y)!=reverse},...)
a:make(false)
b:make(true)`,
		`make:func(reverse:bool)->func([...int])->[...int]:{
cmp:{x:int,y:int,less:(x<y)!=reverse}
out:list.Sort(cmp:cmp,...)
}.out
a:make(false)
b:make(true)`,
	} {
		t.Run(source, func(t *testing.T) {
			v := cuecontext.New().CompileString("import \"list\"\n" + source + "\nout:(a & b)([3,1,2])")
			semanticJSON(t, v.Context().CompileString("import \"list\"\n"+source+"\nout:a([3,1,2])"), "out", `[1,2,3]`)
			semanticJSON(t, v.Context().CompileString("import \"list\"\n"+source+"\nout:b([3,1,2])"), "out", `[3,2,1]`)
			if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
				t.Fatal("different saved comparator templates acquired the same identity")
			}
		})
	}
}

func TestStdlibIsSortedRefinement(t *testing.T) {
	for _, call := range []string{
		`list.IsSorted([1,2],cmp)`,
		`list.IsSorted(cmp:cmp,...)([1,2])`,
		`[1,2] & list.IsSorted(cmp)`,
	} {
		t.Run(call, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(`import "list"
reverse:bool
cmp:{x:int,y:int,less:(x<y)!=reverse}
out:` + call)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
					t.Fatal("IsSorted completed an unresolved comparison")
				}
				if err := v.Validate(); err != nil {
					t.Fatalf("unresolved comparison became a permanent error: %v", err)
				}
				good := v.FillPath(cue.ParsePath("reverse"), false)
				if call[0] == '[' {
					semanticJSON(t, good, "out", `[1,2]`)
					if v.FillPath(cue.ParsePath("reverse"), true).Validate() == nil {
						t.Fatal("IsSorted validator accepted descending order")
					}
				} else {
					semanticJSON(t, good, "out", `true`)
					semanticJSON(t, v.FillPath(cue.ParsePath("reverse"), true), "out", `false`)
				}
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
				if err := v.Err(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
			}
		})
	}
}
