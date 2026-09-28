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

func checkOperatorDefinition(t *testing.T, source string, valid bool) {
	t.Helper()
	v := cuecontext.New().CompileString(source)
	f := v.LookupPath(cue.ParsePath("f"))
	if !f.Exists() {
		t.Fatalf("invalid test source: %s: %v", source, v.Err())
	}
	if err := f.Validate(); (err == nil) != valid {
		t.Fatalf("%s\nvalid=%v: %v", source, valid, err)
	}
}

// The matrix records the language's operand domains independently of the
// inference implementation. Both accepted and rejected pairs are important:
// returning top must not conceal an invalid operation.
func TestOperatorOperandDomains(t *testing.T) {
	types := []string{"int", "float", "string", "bytes", "bool", "null", "{a:int}", "[...int]", "func(int)->int"}
	for _, op := range []string{"&", "|", "+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=", "=~", "!~", "&&", "||"} {
		for i, a := range types {
			for j, b := range types {
				numeric := i < 2 && j < 2
				sameText := i == j && (i == 2 || i == 3)
				result := "_"
				valid := false
				switch op {
				case "&", "|":
					valid = true
				case "+":
					valid = numeric || sameText
					if numeric {
						result = "number"
					} else {
						result = a
					}
				case "-", "/":
					valid = numeric
					result = "number"
				case "*":
					valid = numeric || i == 0 && (j == 2 || j == 3) || j == 0 && (i == 2 || i == 3)
					result = "number"
					if i == 2 || j == 2 {
						result = "string"
					}
					if i == 3 || j == 3 {
						result = "bytes"
					}
				case "==", "!=":
					valid = i < 8 && j < 8 || i == 5 || j == 5
					result = "bool"
				case "<", "<=", ">", ">=":
					valid = numeric || sameText
					result = "bool"
				case "=~", "!~":
					valid = i == 2 && (j == 2 || j == 3)
					result = "bool"
				case "&&", "||":
					valid = i == 4 && j == 4
					result = "bool"
				}
				t.Run(fmt.Sprintf("%s/%s/%s", op, a, b), func(t *testing.T) {
					source := fmt.Sprintf("f:func(x:%s,y:%s)->(%s):x %s y", a, b, result, op)
					checkOperatorDefinition(t, source, valid)
					if valid && result != "_" {
						checkOperatorDefinition(t, fmt.Sprintf("f:func(x:%s,y:%s)->{wrong:42}:x %s y", a, b, op), false)
					}
				})
			}
		}
	}
	for _, op := range []string{"+", "-", "!", "<", "<=", ">", ">=", "!=", "=~", "!~"} {
		for i, typ := range types {
			valid := false
			result := typ
			switch op {
			case "+", "-":
				valid = i < 2
			case "!":
				valid = i == 4
			case "<", "<=", ">", ">=":
				valid = i < 4
				if i < 2 {
					result = "number"
				}
			case "!=":
				valid = i < 8
				if i == 6 {
					result = "{...}"
				}
				if i == 7 {
					result = "[...]"
				}
				if i < 2 {
					result = "number"
				}
				if i == 5 {
					result = "_"
				}
			case "=~", "!~":
				valid = i == 2 || i == 3
			}
			t.Run("unary/"+op+"/"+typ, func(t *testing.T) {
				checkOperatorDefinition(t, fmt.Sprintf("f:func(x:%s)->(%s):%sx", typ, result, op), valid)
			})
		}
	}
}

func TestOperatorStructuralAndGeneric(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"generic_meet", `f(A,B):func(x:A,y:B)->(A&B):x&y`, true},
		{"generic_union", `f(A,B):func(x:A,y:B)->(A|B):x|y`, true},
		{"generic_number", `f(A:number):func(x:A)->number:x*2`, true},
		{"generic_unary_plus", `f(A:number):func(x:A)->A:+x`, true},
		{"generic_unary_minus", `f(A:number):func(x:A)->A:-x`, false},
		{"mixed_repeat", `f:func(x:string|bytes,n:int)->(string|bytes):x*n`, true},
		{"mixed_repeat_wrong", `f:func(x:string|bool,n:int)->_:x*n`, false},
		{"mixed_add", `f:func(x:int|string,y:int|string)->_:x+y`, false},
		{"union_number", `f:func(x:number,y:number)->(int|float):x/y`, true},
		{"refinement_in_union", `f:func(x:int&>=0)->((int&>=0)|string):x`, true},
		{"refinement_not_in_union", `f:func(x:int&>=0)->((int&>0)|string):x`, false},
		{"regex_union", `f:func(x:string,p:string|bytes)->bool:x=~p`, true},
		{"compare_records", `f:func(x:{a:int},y:{a:int})->bool:x==y`, true},
		{"compare_records_not_singletons", `f:func(x:{a:1},y:{a:1})->true:x==y`, false},
		{"compare_lists", `f(A):func(x:[...A],y:[...A])->bool:x!=y`, true},
		{"dynamic_index", `f(A):func(xs:[...A],n:int)->A:xs[n]`, true},
		{"tuple_index", `f(A,B):func(xs:[A,B],n:int)->(A|B):xs[n]`, true},
		{"tuple_index_wrong", `f:func(xs:[int,string],n:int)->int:xs[n]`, false},
		{"index_wrong_subject", `f:func(xs:string,n:int)->_:xs[n]`, false},
		{"index_wrong_index", `f:func(xs:[...int],n:float)->_:xs[n]`, false},
		{"record_index", `f:func(x:{a:int})->int:x["a"]`, true},
		{"record_index_union", `f:func(x:{a:int,b:string},k:"a"|"b")->(int|string):x[k]`, true},
		{"record_index_missing", `f:func(x:{a:int},k:"a"|"b")->_:x[k]`, false},
		{"record_selector", `f:func(x:{a:int})->int:x.a`, true},
		{"record_selector_missing", `f:func(x:{a:int})->_:x.b`, false},
		{"rigid_record_meet", `f(A:{a:int}):func(x:A)->int:(x&{}).a`, true},
		{"rigid_record_meet_missing", `f(A:{a:int}):func(x:A)->int:(x&{}).b`, false},
		{"generic_slice", `f(A):func(xs:[...A],lo:int,hi:int)->[...A]:xs[lo:hi]`, true},
		{"bounded_slice", `f(A:[...int]):func(xs:A,lo:int,hi:int)->[...int]:xs[lo:hi]`, true},
		{"bytes_slice", `f:func(xs:bytes,lo:int,hi:int)->bytes:xs[lo:hi]`, true},
		{"bad_slice_subject", `f:func(xs:string,lo:int,hi:int)->_:xs[lo:hi]`, false},
		{"bad_slice_index", `f:func(xs:bytes,lo:string)->_:xs[lo:]`, false},
		{"string_interpolation", `f:func(x:bytes)->string:"\(x)"`, true},
		{"bytes_interpolation", `f:func(x:bytes|int|bool|string)->bytes:'\(x)'`, true},
		{"bad_interpolation", `f:func(x:{a:int})->_:"\(x)"`, false},
		{"invalid_operand_with_failure", `f:func(x:bool)->int:(x+1)&_|_`, false},
		{"failed_operand", `f:func(x:int)->int:x+_|_`, true},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, tc.source, tc.valid) })
	}
}

func TestOperatorCheckedExecution(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{`f:func(s:string,p:string)->bool:s=~p
out:f("abc","^a")`, `true`},
		{`f:func(s:string,p:bytes)->bool:s!~p
out:f("abc",'^a')`, `false`},
		{`f:func(x:{a:int},y:{a:int})->bool:x==y
out:f({a:1},{a:2})`, `false`},
		{`f(A):func(xs:[...A],n:int)->A:xs[n]
out:f(["a","b"],1)`, `"b"`},
		{`f:func(r:{a:int,b:string},k:"a"|"b")->(int|string):r[k]
out:f({a:1,b:"ok"},"b")`, `"ok"`},
		{`f(A):func(xs:[...A],lo:int,hi:int)->[...A]:xs[lo:hi]
out:f([1,2,3],1,3)`, `[2,3]`},
		{`f:func(b:bytes)->string:"\(b)"
out:f('ok')`, `"ok"`},
		{`f:func(x:string|bytes,n:int)->(string|bytes):x*n
out:f("a",3)`, `"aaa"`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			v := cuecontext.New().CompileString(tc.source)
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			out, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			if err != nil || string(out) != tc.want {
				t.Fatalf("got %s, %v; want %s", out, err, tc.want)
			}
		})
	}
}
