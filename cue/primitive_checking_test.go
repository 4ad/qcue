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
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"testing"
)

func TestPrimitiveDefinitionChecking(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"and_capability", `f:and & (forall(A) func([A,...A])->A)`, true},
		{"and_false_capability", `f:and & (forall(A) func([...A])->A)`, false},
		{"or_capability", `f:or & (forall(A) func([...A])->A)`, true},
		{"len_capability", `f:len & (func(string)->(int&>=0))`, true},
		{"close_capability", `f:close & (func({a:int})->{a:int})`, true},

		{"validator_constructor", `import "strings"
f:func(n:int)->string:strings.MinRunes(n)`, true},
		{"validator_constructor_bad_input", `import "strings"
f:func(n:string)->string:strings.MinRunes(n)`, false},
		{"validator_constructor_bad_label", `import "strings"
f:func(n:int)->string:strings.MinRunes(min:n)`, false},
		{"validator_constructor_bad_result", `import "strings"
f:func(n:int)->int:strings.MinRunes(n)`, false},
		{"validator_constructor_list", `import "list"
f:func(n:int)->[...]:list.MinItems(n)`, true},
		{"bare_validator_constructor", `import "encoding/json"
f:func()->(string|bytes):json.Valid()`, true},
		{"close_generic", `f(A:{a:int}):func(x:A)->A:close(x)`, true},
		{"close_generic_bad_result", `f(A:{a:int}):func(x:A)->string:close(x)`, false},

		{"and_nonempty", `f(A):func(xs:[A,...A])->A:and(xs)`, true},
		{"and_maybe_empty", `f(A):func(xs:[...A])->A:and(xs)`, false},
		{"and_empty", `f:func()->_:and([])`, true},
		{"and_empty_wrong", `f:func()->int:and([])`, false},
		{"and_tuple", `f(A,B):func(xs:[A,B])->(A&B):and(xs)`, true},
		{"and_optional_tail", `f:func(xs:[int,...string])->int:and(xs)`, true},
		{"and_optional_tail_wrong", `f:func(xs:[int,...string])->string:and(xs)`, false},
		{"and_records", `f:func()->{a:1,b:2}:and([{a:1},{b:2}])`, true},
		{"and_bad_input", `f:func(x:int)->_:and(x)`, false},
		{"or_generic", `f(A):func(xs:[...A])->A:or(xs)`, true},
		{"or_tuple", `f(A,B):func(xs:[A,B])->(A|B):or(xs)`, true},
		{"or_tail", `f:func(xs:[int,...string])->(int|string):or(xs)`, true},
		{"or_tail_wrong", `f:func(xs:[int,...string])->int:or(xs)`, false},
		{"or_empty", `f:func()->int:or([])`, true},
		{"or_bad_input", `f:func(x:int)->_:or(x)`, false},
		{"len", `f:func(x:bytes|string|[...]|{...})->(int&>=0):len(x)`, true},
		{"len_bad_input", `f:func(x:int)->int:len(x)`, false},
		{"len_bad_result", `f:func(x:string)->string:len(x)`, false},
		{"close", `f:func(x:{a:int})->{a:int}:close(x)`, true},
		{"close_bad_input", `f:func(x:int)->_:close(x)`, false},
		{"error", `f(A):func(x:string)->A:error(x)`, true},
		{"error_bad_input", `f:func(x:int)->_:error(x)`, false},
		{"div", `f:func(x:int,y:int)->int:div(x,y)`, true},
		{"mod", `f:func(x:int,y:int)->int:mod(x,y)`, true},
		{"quo", `f:func(x:int,y:int)->int:quo(x,y)`, true},
		{"rem", `f:func(x:int,y:int)->int:rem(x,y)`, true},
		{"div_bad_input", `f:func(x:string,y:int)->int:div(x,y)`, false},
		{"native_labels", `import "strings"
f:func(x:string)->string:strings.Trim(cutset: " ", s:x)`, true},
		{"native_default", `import "path"
f:func(x:string)->string:path.Base(x)`, true},
		{"native_wrong_label", `import "strings"
f:func(x:string)->string:strings.ToUpper(wrong:x)`, false},
		{"native_wrong_arity", `import "strings"
f:func(x:string)->string:strings.ToUpper(x,x)`, false},
		{"native_bad_result", `import "math"
f:func(x:number)->string:math.Sqrt(x)`, false},
		{"native_claim", `import "math"
f:math.Sqrt & (func(number)->number)`, true},
		{"native_false_claim", `import "math"
f:math.Sqrt & (func(number)->42)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tc.source)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid test: %v", v.Err())
			}
			if err := f.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestPrimitiveFoldCalls(t *testing.T) {
	v := cuecontext.New().CompileString(`
_f(A):func(xs:[A,...A])->A:and(xs)
x:_f([{a:1},{b:1}])
choose(A):func(xs:[...A])->A:or(xs)
y:choose(["ok"])
`)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"x": `{"a":1,"b":1}`, "y": `"ok"`} {
		data, err := v.LookupPath(cue.ParsePath(field)).MarshalJSON()
		if err != nil || string(data) != want {
			t.Fatalf("%s: %s, %v; want %s", field, data, err, want)
		}
	}
}
