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

func TestStdlibPartialChecking(t *testing.T) {
	for _, tc := range []struct {
		pkg, source string
		valid       bool
	}{
		{"strings", `f:strings.Repeat(count:2,...) & (func(string)->string)`, true},
		{"strings", `f:strings.Repeat(count:2,...) & (func(string)->int)`, false},
		{"strings", `f:strings.Repeat(count:"bad",...)`, false},
		{"strings", `f:strings.Repeat(missing:2,...)`, false},
		{"strings", `f:strings.Repeat(count:2,...)("a",count:3,...)`, false},
		{"strings", `f:strings.Repeat("a",2,3,...)`, false},
		{"strings", `f:func(s:string)->func(int)->string:strings.Repeat(s,...)`, true},
		{"strings", `f:func(s:string)->func(int)->int:strings.Repeat(s,...)`, false},
		{"strings", `f:strings.Contains("abc",...) & (func(string)->bool)`, true},
		{"strings", `f:func()->bool:strings.Contains("abc",...)()`, false},
		{"list", `f(A):func(xs:[...A])->func(int)->[...A]:list.Take(xs,...)`, true},
		{"list", `f(A):func(xs:[...A])->func(int)->[...string]:list.Take(xs,...)`, false},
		{"list", `take:list.Take(n:2,...)
f(A):func(xs:[...A])->[...A]:take(xs)`, true},
		{"list", `take:list.Take(n:2,...)
f:func(xs:[...string])->[...int]:take(xs)`, false},
		{"list", `f:list.Take(n:2,...)[int] & (func([...int])->[...int])`, true},
		{"list", `f:list.Take[int](n:2,...) & (func([...int])->[...int])`, true},
		{"list", `f:func(xs:[...string])->func()->[...int]:list.Take[int](xs,n:2,...)`, false},
		{"path", `f:path.Base(...) & (func(string)->string)`, true},
		{"path", `f:path.Base(os:"windows",...) & (func(string)->string)`, true},
		{"path", `f:path.Base(os:"bad",...)`, false},
		{"encoding/json", `f:json.Valid(...) & (func(string|bytes)->bool)`, true},
		{"encoding/json", `f:json.Valid(1,...)`, false},
		{"encoding/json", `v:json.Valid()
f:v(...)`, false},
		{"encoding/csv", `f:func(s:bytes|string)->[...[...string]]:csv.Decode(s,...)()`, true},
		{"encoding/csv", `f:func(s:bytes|string)->[...[...int]]:csv.Decode(s,...)()`, false},
		{"list", `f:func(a:number,b:number,c:number)->[...number]:list.Range(a,b,c,...)()`, true},
		{"list", `f:func(a:number,b:number,c:number)->[...int]:list.Range(a,b,c,...)()`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			checkOperatorDefinition(t, "import \""+tc.pkg+"\"\n"+tc.source, tc.valid)
		})
	}
}

func TestStdlibPartialExecution(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
import "list"
import "path"
import "encoding/json"
repeat:strings.Repeat(count:2,...)
contains:strings.Contains("abc",...)
take:list.Take(n:2,...)
selected:take[int]
reversed:list.Reverse[int]([1,2],...)
base:path.Base(...)
valid:json.Valid(...)
out:[repeat("ab"),contains("b"),take([1,2,3]),take(["a","b","c"]),selected([1,2,3]),reversed(),base("a/b"),valid("{}")]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `["abab",true,[1,2],["a","b"],[1,2],[2,1],"b",true]`)
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
		bad := v.Unify(ctx.CompileString(`selected:_
bad:selected(["wrong"])`))
		if bad.Validate() == nil {
			t.Fatalf("export lost the selected element type: %s", source)
		}
	}
}

func TestStdlibPartialCaptures(t *testing.T) {
	for _, declaration := range []string{
		`repeat:strings.Repeat(count:count,...)`,
		`make:func(n:int)->func(string)->string:strings.Repeat(count:n,...)
repeat:make(count)`,
		`wrap:func(r:func(string)->string)->func(string)->string:func(s:string)->string:r(s)
repeat:wrap(strings.Repeat(count:count,...))`,
	} {
		t.Run(declaration, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"strings\"\ncount:int\n" + declaration + "\nout:repeat(\"a\")")
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
					t.Fatal("native partial application supplied an unknown saved argument")
				}
				filled := v.FillPath(cue.ParsePath("count"), 2)
				semanticJSON(t, filled, "out", `"aa"`)
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

func TestStdlibPartialDefaultsAndStages(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
import "path"
first:strings.Repeat("ab",...)
second:first(count:2,...)
exact:strings.Repeat(count:2,...) & (func("a")->"aa")
volume:path.VolumeName(path:"C:\\a",...)
out:[second(),first(3),exact("a"),volume()]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `["abab","ababab","aa","C:"]`)
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

func TestStdlibPartialConcreteCaptures(t *testing.T) {
	for _, declaration := range []string{
		`make:func(n:int)->func(string)->string:strings.Repeat(count:n,...)
repeat:make(2)`,
		`make:func(n:int)->func(string)->string:strings.Repeat(count:n+1,...)
repeat:make(1)`,
		`wrap:func(r:func(string)->string)->func(string)->string:func(s:string)->string:r(s)
repeat:wrap(strings.Repeat(count:2,...))`,
	} {
		t.Run(declaration, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"strings\"\n" + declaration + "\nout:repeat(\"a\")")
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", `"aa"`)
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

func TestStdlibPartialSelectedViews(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
views:list.Take[int] & list.Take[string]
saved:views(n:1,...)
merged:saved & list.Take[int](n:1,...)
out:[merged([1,2]),merged(["a","b"])]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[[1],["a"]]`)
		bad := v.Unify(ctx.CompileString(`merged:_
bad:merged([true])`))
		if bad.Validate() == nil {
			t.Fatal("partial application reopened the selected type domains")
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

func TestStdlibPartialIdentity(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
a:int
b:int
f:strings.Repeat(count:a,...) & strings.Repeat(count:b,...)
g:f("a",...)
out:g()
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		good := v.FillPath(cue.ParsePath("a"), 2).FillPath(cue.ParsePath("b"), 2)
		semanticJSON(t, good, "out", `"aa"`)
		bad := v.FillPath(cue.ParsePath("a"), 2).FillPath(cue.ParsePath("b"), 3)
		if bad.Validate() == nil {
			source, _ := format.Node(v.Syntax())
			t.Fatalf("different saved arguments acquired the same identity: %s", source)
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
}

func TestStdlibPartialIntermediateInterfaces(t *testing.T) {
	for _, tc := range []struct {
		name, declaration, call, badCall string
	}{
		{"successive", `first:strings.Replace(n:1,...)
view:first & (func(new!:string,old!:string,s!:string)->string)
saved:view(old:"a",...)`, `saved(s:"aa",new:"b")`, `saved(s:"aa",new:1)`},
		{"merged", `first:strings.Replace(n:1,...)
view:first & (func(new!:string,old!:string,s!:string)->string)
saved:strings.Replace(old:"a",n:1,...) & view(old:"a",...)`, `saved(s:"aa",new:"b")`, `saved(s:"aa",new:1)`},
		{"reverse_merge", `first:strings.Replace(n:1,...)
view:first & (func(new!:string,old!:string,s!:string)->string)
saved:view(old:"a",...) & strings.Replace(old:"a",n:1,...)`, `saved(s:"aa",new:"b")`, `saved(s:"aa",new:1)`},
		{"positional", `first:strings.Replace(n:1,...)
view:first & (func(string,string,string)->string)
saved:strings.Replace(old:"a",n:1,...) & view(old:"a",...)`, `saved(s:"aa",new:"b")`, `saved(s:"aa",new:1)`},
		{"multiple_interfaces", `first:strings.Replace(n:1,...)
view:first & (func(new!:string,old!:string,s!:string)->string)
second:view(s:"aa",...) & (func(new!:string,old!:string)->string)
saved:strings.Replace(old:"a",s:"aa",n:1,...) & second(old:"a",...)`, `saved(new:"b")`, `saved(new:1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"strings\"\n" + tc.declaration + "\nout:" + tc.call)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}, nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", `"ba"`)
				bad := v.Unify(ctx.CompileString("saved:_\nbad:" + tc.badCall))
				if bad.Validate() == nil {
					t.Fatal("saved native accepted an incompatible remaining argument")
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

func TestStdlibPartialIntermediateRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
count:int
first:strings.Replace(n:count,...)
view:first & (func(new!:"b",old!:"a",s!:"aa")->"ba")
saved:strings.Replace(old:"a",n:count,...) & view(old:"a",...)
out:saved(s:"aa",new:"b")`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		good := v.FillPath(cue.ParsePath("count"), 1)
		if err := good.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, good, "out", `"ba"`)
		if err := v.FillPath(cue.ParsePath("count"), 2).Validate(); err == nil {
			t.Fatal("saved native lost its intermediate result obligation")
		}
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
	}
}

func TestStdlibPartialIntermediateGeneric(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
first:list.Slice(i:1,...)
view:first & (forall(A) func(j!:int,x!:[...A])->[...A])
saved:list.Slice(i:1,j:2,...) & view(j:2,...)
out:[saved(x:[0,1,2]),saved(x:["a","b","c"])]`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[[1],["b"]]`)
		if err := v.Unify(ctx.CompileString("saved:_\nbad:saved(x:42)")).Validate(); err == nil {
			t.Fatal("saved generic native accepted an incompatible argument")
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

func TestStdlibPartialIntermediateObligations(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
a:string
b:string
first:strings.Replace(n:1,...)
left:first & (func(new!:"b",old!:"a",s!:"aa")->a)
right:first & (func(new!:"b",old!:"a",s!:"aa")->b)
saved:strings.Replace(n:1,old:"a",...) & left(old:"a",...) & right(old:"a",...)
out:saved(s:"aa",new:"b")`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		good := v.FillPath(cue.ParsePath("a"), "ba").FillPath(cue.ParsePath("b"), "ba")
		if err := good.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, good, "out", `"ba"`)
		for _, field := range []string{"a", "b"} {
			if err := v.FillPath(cue.ParsePath(field), "wrong").Validate(); err == nil {
				t.Fatalf("saved native lost its %s result obligation", field)
			}
		}
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
	}
}
