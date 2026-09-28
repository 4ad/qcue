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

func TestStdlibListPredicateRefinement(t *testing.T) {
	for _, tc := range []struct {
		name, declarations, result1, result2 string
	}{
		{"contains", `out:list.Contains([x],1)`, "true", "false"},
		{"contains_function", `f:func(n:int)->bool:list.Contains([n],1)
out:f(x)`, "true", "false"},
		{"contains_partial", `f:list.Contains(v:1,...)
out:f([x])`, "true", "false"},
		{"contains_record", `out:list.Contains([{a:x}],{a:1})`, "true", "false"},
		{"contains_closure", `f:func()->int:x
out:list.Contains([f],f)`, "true", "true"},
		{"unique", `out:list.UniqueItems([x,1])`, "false", "true"},
		{"unique_function", `f:func(n:int)->bool:list.UniqueItems([n,1])
out:f(x)`, "false", "true"},
		{"unique_partial", `f:list.UniqueItems(...)
out:f([x,1])`, "false", "true"},
		{"unique_record", `out:list.UniqueItems([{a:x},{a:1}])`, "false", "true"},
		{"unique_closure", `f:func()->int:x
out:list.UniqueItems([f,f])`, "false", "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"list\"\nx:int\n" + tc.declarations)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
					t.Fatal("list predicate observed an unresolved value")
				}
				if err := v.Validate(); err != nil {
					t.Fatalf("an incomplete comparison became a permanent error: %v", err)
				}
				semanticJSON(t, v.FillPath(cue.ParsePath("x"), 1), "out", tc.result1)
				semanticJSON(t, v.FillPath(cue.ParsePath("x"), 2), "out", tc.result2)
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

func TestStdlibListPredicateObservations(t *testing.T) {
	for _, tc := range []struct {
		name, source, result string
	}{
		{"contains_empty", `out:list.Contains([],1)`, "false"},
		{"contains_witness", `out:list.Contains([int,1],1)`, "true"},
		{"contains_disjoint", `out:list.Contains([string],1)`, "false"},
		{"contains_defaults", `out:list.Contains([{a:*1|2}],{a:1})`, "true"},
		{"contains_default_list", `out:list.Contains([[1,...int]],[1])`, "true"},
		{"contains_default_empty", `out:list.Contains([...int],1)`, "false"},
		{"contains_hidden", `out:list.Contains([{a:1,_h:int}],{a:1})`, "true"},
		{"contains_selected_closure", `f:func(x:int)->int:x
g:f & (func(1)->1)
out:list.Contains([f],g)`, "true"},
		{"contains_distinct_closure", `f:func(x:int)->int:x
g:func(x:int)->int:x
out:list.Contains([f],g)`, "false"},
		{"unique_empty", `out:list.UniqueItems([])`, "true"},
		{"unique_singleton", `out:list.UniqueItems([int])`, "true"},
		{"unique_witness", `out:list.UniqueItems([int,1,1])`, "false"},
		{"unique_disjoint", `out:list.UniqueItems([int,string,bool])`, "true"},
		{"unique_defaults", `out:list.UniqueItems([{a:*1|2},{a:1}])`, "false"},
		{"unique_default_list", `out:list.UniqueItems([[1,...int],[1]])`, "false"},
		{"unique_default_empty", `out:list.UniqueItems([...int])`, "true"},
		{"unique_hidden", `out:list.UniqueItems([{a:1,_h:int},{a:1}])`, "false"},
		{"unique_selected_closure", `f:func(x:int)->int:x
g:f & (func(1)->1)
out:list.UniqueItems([f,g])`, "false"},
		{"unique_distinct_closure", `f:func(x:int)->int:x
g:func(x:int)->int:x
out:list.UniqueItems([f,g])`, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"list\"\n" + tc.source)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				semanticJSON(t, v, "out", tc.result)
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
			}
		})
	}
}

func TestStdlibListPredicateIncompleteInputs(t *testing.T) {
	for _, source := range []string{
		`out:list.Contains([int],1)`,
		`out:list.Contains([{a:int,b:string}],{a:1,b:"x"})`,
		`out:list.UniqueItems([int,1])`,
		`out:list.UniqueItems([{a:int,b:string},{a:1,b:"x"}])`,
		`f:func(int)->int
out:list.Contains([f],f)`,
		`f:func(int)->int
out:list.UniqueItems([f,f])`,
	} {
		t.Run(source, func(t *testing.T) {
			v := cuecontext.New().CompileString("import \"list\"\n" + source)
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			if _, err := v.LookupPath(cue.ParsePath("out")).Bool(); err == nil {
				t.Fatal("list predicate produced a Boolean from unresolved inputs")
			}
		})
	}
}

func TestStdlibUniqueValidatorRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
x:int
out:[x,1] & list.UniqueItems`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatalf("an unresolved comparison rejected the list: %v", err)
		}
		if err := v.FillPath(cue.ParsePath("x"), 1).Validate(); err == nil {
			t.Fatal("duplicate elements passed validation")
		}
		semanticJSON(t, v.FillPath(cue.ParsePath("x"), 2), "out", "[2,1]")
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
	}
}

func TestStdlibListPredicateOperandErrors(t *testing.T) {
	for _, expr := range []string{
		`list.Contains([_|_],1)`,
		`list.Contains([{a:_|_}],{a:1})`,
		`list.Contains([int,{a:_|_}],{a:1})`,
		`list.Contains([1,{a:_|_}],1)`,
		`list.UniqueItems([_|_,1])`,
		`list.UniqueItems([{a:_|_},{a:1}])`,
		`list.UniqueItems([int,{a:_|_}])`,
		`list.UniqueItems([1,1,{a:_|_}])`,
	} {
		t.Run(expr, func(t *testing.T) {
			v := cuecontext.New().CompileString("import \"list\"\nout:" + expr)
			out := v.LookupPath(cue.ParsePath("out"))
			if _, err := out.Bool(); err == nil {
				t.Fatal("a failed operand produced a Boolean result")
			}
			if err := out.Validate(); err == nil {
				t.Fatal("a failed operand was treated as incomplete")
			}
		})
	}
}
