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
	"cuelang.org/go/cue/cuecontext"
)

func TestStdlibListShapes(t *testing.T) {
	for _, tc := range []struct {
		name, params, body, result, wrong, args, want string
	}{
		{"reverse", "xs:[int,string]", "list.Reverse(xs)", "[string,int]", "[int,string]", `[1,"a"]`, `["a",1]`},
		{"reverse_minimum", "xs:[int,...int]", "list.Reverse(xs)", "[int,...int]", "[int]", `[1,2]`, `[2,1]`},
		{"drop", "xs:[int,string,bool]", "list.Drop(xs,1)", "[string,bool]", "[bool,string]", `[1,"a",true]`, `["a",true]`},
		{"drop_past_end", "xs:[int,string]", "list.Drop(xs,3)", "[]", "[int]", `[1,"a"]`, `[]`},
		{"drop_open", "xs:[bool,string,...int]", "list.Drop(xs,1)", "[string,...int]", "[string,int,...int]", `[true,"a"]`, `["a"]`},
		{"take", "xs:[int,string,bool]", "list.Take(xs,2)", "[int,string]", "[string,int]", `[1,"a",true]`, `[1,"a"]`},
		{"take_past_end", "xs:[int,string]", "list.Take(xs,3)", "[int,string]", "[int,string,bool]", `[1,"a"]`, `[1,"a"]`},
		{"take_open", "xs:[int,...string]", "list.Take(xs,2)", "[int]|[int,string]", "[int,string]", `[1]`, `[1]`},
		{"take_unknown", "xs:[int,string],n:int", "list.Take(xs,n)", "[]|[int]|[int,string]", "[]|[int]", `[1,"a"],2`, `[1,"a"]`},
		{"slice", "xs:[int,string,bool]", "list.Slice(xs,1,3)", "[string,bool]", "[bool,string]", `[1,"a",true]`, `["a",true]`},
		{"slice_open", "xs:[...string]", "list.Slice(xs,0,2)", "[string,string]", "[string]", `["a","b","c"]`, `["a","b"]`},
		{"slice_unknown_end", "xs:[int,string,bool],j:int", "list.Slice(xs,1,j)", "[]|[string]|[string,bool]", "[...(int|string)]", `[1,"a",true],3`, `["a",true]`},
		{"slice_unknown_start", "xs:[int,string,bool],i:int", "list.Slice(xs,i,2)", "[]|[string]|[int,string]", "[]|[int]|[int,string]", `[1,"a",true],1`, `["a"]`},
		{"slice_open_unknown_end", "xs:[int,...string],j:int", "list.Slice(xs,1,j)", "[...string]", "[...int]", `[1,"a","b"],3`, `["a","b"]`},
		{"repeat", "xs:[int,string]", "list.Repeat(xs,2)", "[int,string,int,string]", "[int,string,string,int]", `[1,"a"]`, `[1,"a",1,"a"]`},
		{"repeat_zero", "xs:[...int]", "list.Repeat(xs,0)", "[]", "[int]", `[1]`, `[]`},
		{"repeat_minimum", "xs:[int,...int]", "list.Repeat(xs,2)", "[int,int,...int]", "[int,int,int,...int]", `[1]`, `[1,1]`},
		{"concat", "xs:[[int],[string,bool]]", "list.Concat(xs)", "[int,string,bool]", "[int,bool,string]", `[[1],["a",true]]`, `[1,"a",true]`},
		{"concat_open", "xs:[[int,...int],[string]]", "list.Concat(xs)", "[int,int|string,...(int|string)]", "[int,string,...(int|string)]", `[[1,2],["a"]]`, `[1,2,"a"]`},
		{"flatten_zero", "xs:[int,[string]]", "list.FlattenN(xs,0)", "[int,[string]]", "[int,string]", `[1,["a"]]`, `[1,["a"]]`},
		{"flatten_one", "xs:[[int,[string]],[bool]]", "list.FlattenN(xs,1)", "[int,[string],bool]", "[int,string,bool]", `[[1,["a"]],[true]]`, `[1,["a"],true]`},
		{"flatten_all", "xs:[[int,[string]],[bool]]", "list.FlattenN(xs,-1)", "[int,string,bool]", "[int,[string],bool]", `[[1,["a"]],[true]]`, `[1,"a",true]`},
		{"flatten_open", "xs:[...[...int]]", "list.FlattenN(xs,1)", "[...int]", "[...string]", `[[1,2],[],[3]]`, `[1,2,3]`},
		{"flatten_union", "xs:[...(int|[...int])]", "list.FlattenN(xs,1)", "[...int]", "[...[...int]]", `[1,[2,3]]`, `[1,2,3]`},
		{"sort_length", "xs:[int,int]", "list.Sort(xs,list.Ascending)", "[int,int]", "[int]", `[2,1]`, `[1,2]`},
		{"sort_strings_length", "xs:[string,string]", "list.SortStrings(xs)", "[string,string]", "[string]", `["b","a"]`, `["a","b"]`},
		{"sort_empty", "", "list.Sort([],list.Ascending)", "[]", "[int]", ``, `[]`},
		{"sort_singleton", "xs:[{a:int}]", "list.Sort(xs,list.Ascending)", "[{a:int}]", "[{a:string}]", `[{a:1}]`, `[{"a":1}]`},
		{"sort_empty_unused_comparer", "", "list.Sort([],{less:42})", "[]", "[int]", ``, `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf("import \"list\"\nf:func(%s)->(%s):%s", tc.params, tc.result, tc.body)
			checkOperatorDefinition(t, source, true)
			checkOperatorDefinition(t, fmt.Sprintf("import \"list\"\nf:func(%s)->(%s):%s", tc.params, tc.wrong, tc.body), false)
			v := cuecontext.New().CompileString(source + "\nout:f(" + tc.args + ")")
			got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			if err != nil || string(got) != tc.want {
				t.Fatalf("out = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"generic_reverse", `f(A,B):func(xs:[A,B])->[B,A]:list.Reverse(xs)`, true},
		{"generic_flatten", `f(A):func(xs:[...[...A]])->[...A]:list.FlattenN(xs,1)`, true},
		{"generic_flatten_zero", `f(A:[...]):func(xs:A)->A:list.FlattenN(xs,0)`, true},
		{"generic_repeat_once", `f(A:[...]):func(xs:A)->A:list.Repeat(xs,1)`, true},
		{"generic_drop_zero", `f(A:[...]):func(xs:A)->A:list.Drop(xs,0)`, true},
		{"generic_concat_one", `f(A:[...]):func(xs:A)->A:list.Concat([xs])`, true},
		{"generic_whole_slice", `f(A:[int,string]):func(xs:A)->A:list.Slice(xs,0,2)`, true},
		{"bounded_list_elements", `f(A:[...int]):func(xs:A)->[...int]:list.Reverse(xs)`, true},
		{"reversal_changes_subtype", `f(A:[...int]):func(xs:A)->A:list.Reverse(xs)`, false},
		{"identity_keeps_element_bound", `f(A:[...int]):func(xs:A)->[...string]:list.Repeat(xs,1)`, false},
		{"large_take", `f(A):func(xs:[...A])->[...A]:list.Take(xs,100000)`, true},
		{"large_repeat", `f(A):func(xs:[...A])->[...A]:list.Repeat(xs,100000)`, true},
		{"large_empty_repeat", `f:func()->[]:list.Repeat([],100000)`, true},
		{"generic_flatten_wrong_depth", `f(A):func(xs:[...[...A]])->[...A]:list.FlattenN(xs,0)`, false},
		{"flatten_bad_list", `f:func(xs:int)->_:list.FlattenN(xs,1)`, false},
		{"flatten_bad_depth", `f:func(xs:[...int])->_:list.FlattenN(xs,"1")`, false},
		{"reverse_capability", `f:list.Reverse & (forall(A,B) func([A,B])->[B,A])`, true},
		{"reverse_wrong_capability", `f:list.Reverse & (forall(A,B) func([A,B])->[A,B])`, false},
		{"flatten_capability", `f:list.FlattenN & (forall(A) func([...[...A]],1)->[...A])`, true},
		{"flatten_wrong_capability", `f:list.FlattenN & (forall(A) func([...[...A]],0)->[...A])`, false},
		{"named_take", `f(A,B):func(xs:[A,B])->[A]:list.Take(n:1,x:xs)`, true},
		{"count_alternatives", `f:func(xs:[int,string],n:0|1)->([]|[int]):list.Take(xs,n)`, true},
		{"depth_alternatives", `f:func(xs:[[int]],n:0|1)->([[int]]|[int]):list.FlattenN(xs,n)`, true},
		{"flatten_unknown_depth", `f:func(xs:[[int]],n:int)->[...(int|[int])]:list.FlattenN(xs,n)`, true},
		{"flatten_unknown_depth_not_always_flat", `f:func(xs:[[int]],n:int)->[...int]:list.FlattenN(xs,n)`, false},
		{"flatten_empty_inner", `f:func(xs:[...[]])->[]:list.FlattenN(xs,-1)`, true},
		{"concat_empty_inner", `f:func(xs:[...[]])->[]:list.Concat(xs)`, true},
		{"take_length_bound", `f:func(xs:[...int])->(int&>=0&<=2):len(list.Take(xs,2))`, true},
		{"take_length_not_exact", `f:func(xs:[...int])->2:len(list.Take(xs,2))`, false},
		{"repeat_length", `f:func(xs:[int,string])->4:len(list.Repeat(xs,2))`, true},
		{"repeat_wrong_length", `f:func(xs:[int,string])->2:len(list.Repeat(xs,2))`, false},
		{"empty_sort_missing_less", `f:func()->[]:list.Sort([],{})`, false},
		{"empty_sort_bad_expression", `f:func()->[]:list.Sort([],{less:true+1})`, false},
		{"two_elements_use_comparer", `f:func(xs:[{a:int},{a:int}])->[{a:int},{a:int}]:list.Sort(xs,list.Ascending)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, "import \"list\"\n"+tc.source, tc.valid) })
	}
}
