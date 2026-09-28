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
f:func(n:int)->string:strings.MinRunes(s:n)`, false},
		{"validator_constructor_bad_result", `import "strings"
f:func(n:int)->int:strings.MinRunes(n)`, false},
		{"validator_constructor_list", `import "list"
f:func(n:int)->[...]:list.MinItems(n)`, true},
		{"bare_validator_constructor", `import "encoding/json"
f:func()->(string|bytes):json.Valid()`, true},
		{"close_generic", `f(A:{a:int}):func(x:A)->A:close(x)`, true},
		{"close_generic_bad_result", `f(A:{a:int}):func(x:A)->string:close(x)`, false},

		{"split_elements", `import "strings"
f:func(s:string)->[...string]:strings.Split(s,",")`, true},
		{"split_elements_wrong", `import "strings"
f:func(s:string)->[...int]:strings.Split(s,",")`, false},
		{"split_projection", `import "strings"
f:func(s:string)->string:strings.Split(s,",")[0]`, true},
		{"rune_elements", `import "strings"
f:func(s:string)->[...int]:strings.Runes(s)`, true},
		{"join_elements", `import "strings"
f:func(xs:[...string])->string:strings.Join(xs,",")`, true},
		{"join_elements_wrong", `import "strings"
f:func(xs:[...int])->string:strings.Join(xs,",")`, false},
		{"bare_elements", `import "list"
f:func(xs:[...string])->bool:list.IsSortedStrings(xs)`, true},
		{"bare_elements_wrong", `import "list"
f:func(xs:[...int])->bool:list.IsSortedStrings(xs)`, false},
		{"nested_list_elements", `import "encoding/csv"
f:func(s:string)->[...[...string]]:csv.Decode(s)`, true},
		{"native_map_elements", `import "regexp"
f:func(s:string)->[...{[string]:string}]:regexp.FindAllNamedSubmatch("(?P<x>.)",s,-1)`, true},
		{"native_record_fields", `import "time"
f:func(s:string)->int:time.Split(s).year`, true},
		{"native_record_optional", `import "net"
f:func(s:string)->{prefix_len:int,broadcast_addr?:string}:net.ParseCIDR(s)`, true},
		{"native_record_optional_access", `import "net"
f:func(s:string)->string:net.ParseCIDR(s).broadcast_addr`, false},
		{"native_precise_capability", `import "strings"
f:strings.Split & (func(string,string)->[...string])`, true},
		{"path_tuple", `import "path"
f:func(s:string)->[string,string]:path.Split(s)`, true},
		{"path_projection", `import "path"
f:func(s:string)->string:path.Split(s)[0]`, true},
		{"path_join_wrong", `import "path"
f:func(xs:[...int])->string:path.Join(xs)`, false},
		{"bare_validator_typed_result", `import "list"
f:func()->[...string]:list.IsSortedStrings()`, true},

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

func TestBuiltinPrecisionRoundTrip(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "strings"
f:func(s:string)->string:strings.Split(s,",")[0]
input:string
out:f(input)`)
	for _, options := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		source, err := format.Node(v.Syntax(options...))
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := ctx.CompileBytes(source)
		filled := rebuilt.FillPath(cue.ParsePath("input"), "first,second")
		got, err := filled.LookupPath(cue.ParsePath("out")).String()
		if err != nil || got != "first" {
			t.Fatalf("%s: got %q, %v", source, got, err)
		}
		bad := rebuilt.Unify(ctx.CompileString(`f:func(string)->int`))
		if bad.Validate() == nil {
			t.Fatalf("lost native result type after export: %s", source)
		}
	}
}

func TestBuiltinPrecisionPreservesIncompleteCalls(t *testing.T) {
	for _, declaration := range []string{"", "@experiment(functions=false,quantified=false)\n"} {
		v := cuecontext.New().CompileString(declaration + `import "strings"
x:_
y:strings.Join(x,",")`)
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := v.LookupPath(cue.ParsePath("y")).MarshalJSON(); err == nil {
			t.Fatal("checking metadata supplied the missing list argument")
		}
	}
}
