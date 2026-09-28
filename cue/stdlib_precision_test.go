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

func TestStdlibGenericLists(t *testing.T) {
	for _, tc := range []struct {
		name, params, body, args, want string
	}{
		{"Reverse", "xs:[...A]", "list.Reverse(xs)", "[1,2,3]", "[3,2,1]"},
		{"Drop", "xs:[...A],n:int", "list.Drop(xs,n)", "[1,2,3],1", "[2,3]"},
		{"Take", "xs:[...A],n:int", "list.Take(xs,n)", "[1,2,3],2", "[1,2]"},
		{"Slice", "xs:[...A],i:int,j:int", "list.Slice(xs,i,j)", "[1,2,3],1,3", "[2,3]"},
		{"Repeat", "xs:[...A],n:int", "list.Repeat(xs,n)", "[1,2],2", "[1,2,1,2]"},
		{"Concat", "xs:[...[...A]]", "list.Concat(xs)", "[[1,2],[3]]", "[1,2,3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf("import \"list\"\nf(A):func(%s)->[...A]:%s\nout:f(%s)", tc.params, tc.body, tc.args)
			v := cuecontext.New().CompileString(source)
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %s, %v; want %s", got, err, tc.want)
			}
			bad := fmt.Sprintf("import \"list\"\nf(A):func(%s)->[...string]:%s", tc.params, tc.body)
			checkOperatorDefinition(t, bad, false)
		})
	}
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"alias", `r:list.Reverse
f(A):func(xs:[...A])->[...A]:r(xs)`, true},
		{"labels", `f(A):func(xs:[...A])->[...A]:list.Take(n:2,x:xs)`, true},
		{"capability", `f:list.Reverse & (forall(A) func([...A])->[...A])`, true},
		{"false_capability", `f:list.Reverse & (forall(A) func([...A])->[...string])`, false},
		{"bad_list", `f:func(x:string)->_:list.Reverse(x)`, false},
		{"bad_count", `f:func(xs:[...int],n:string)->_:list.Take(xs,n)`, false},
		{"bad_nested_list", `f:func(xs:[...int])->_:list.Concat(xs)`, false},
		{"records", `f(A:{x:int}):func(xs:[...A])->[...A]:list.Reverse(xs)`, true},
		{"callbacks", `f(A:func(int)->int):func(xs:[...A])->[...A]:list.Reverse(xs)`, true},
		{"max", `f(A:number):func(xs:[...A])->A:list.Max(xs)`, true},
		{"min", `f(A:number):func(xs:[...A])->A:list.Min(xs)`, true},
		{"max_wrong", `f(A:number):func(xs:[...A])->string:list.Max(xs)`, false},
		{"min_bad_elements", `f:func(xs:[...string])->_:list.Min(xs)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkOperatorDefinition(t, "import \"list\"\n"+tc.source, tc.valid)
		})
	}
}
