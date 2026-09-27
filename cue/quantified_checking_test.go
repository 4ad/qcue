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
)

func TestQuantifiedDefinitionChecking(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"identity", `f(A):func(x:A)->A:x`, true},
		{"explicit_failure", `f(A):func(x:A)->A:_|_`, true},
		{"assertion", `f:func(x:int)->(int&>0):x&>0`, true},
		{"recursion", `f:func(x:int)->int:f(x)`, true},
		{"inventory", `f:func(r:{a:int})->int:r.a`, true},
		{"width", `get:func(r:{a:int})->int:r.a
f:func()->int:get({a:1,b:true})`, true},
		{"import_hypothesis", `source:func(int)->string
f:func()->string:source(3)`, true},
		{"builtin_import", "import \"strings\"\nf:func(x:string)->string:strings.ToUpper(x)", true},
		{"unused_optional_parameter", `f:func(a!:int,note?:string)->int:a`, true},
		{"optional_parameter_default", `f:func(x?:int)->int
f:func(x:int=0)->int:x`, true},
		{"unguarded_optional_parameter", `f:func(note?:string)->string:note`, false},
		{"optional_default_not_universal", `f:func(x?:int)->0
f:func(x:int=0)->int:x`, false},
		{"wrong_generic", `f(A):func(x:A)->A:({value:0}).value`, false},
		{"wrong_kind", `f:func(x:int)->string:x`, false},
		{"wrong_literal", `f:func(x:int)->2:1`, false},
		{"unknown_field", `f:func(r:{a:int})->int:r.b`, false},
		{"optional_field", `f:func(r:{a?:int})->int:r.a`, false},
		{"missing_argument_field", `get:func(r:{a:int,b:bool})->int:r.a
f:func()->int:get({a:1})`, false},
		{"narrow_callback", `apply:func(g:func(number)->string)->string:g(1.5)
ints:func(x:int)->string:"ok"
f:func()->string:apply(ints)`, false},
		{"bad_unused_field", `f:func(r:{a:int})->int:{unused:r.b,value:0}.value`, false},
		{"failure_does_not_supply_fields", `f:func(r:{a:int})->int:{bad:r.b,conflict:1&2,value:0}.value`, false},
		{"failure_does_not_supply_fields_reordered", `f:func(r:{a:int})->int:{conflict:1&2,bad:r.b,value:0}.value`, false},
		{"union_body", `f:(func(x:int)->int:x)|(func(x:int)->int:"wrong")`, false},
		{"default_body", `f:*(func(x:int)->int:x)|(func(x:int)->int:"wrong")`, false},
		{"optional_body", `f:{optional?:func(x:int)->int:"wrong"}`, false},
		{"required_body", `f:{required!:func(x:int)->int:"wrong"}`, false},
		{"absent_body", `f:{optional?:(func(x:int)->int:"wrong")&int}`, true},
		{"invalid_empty_domain", `f:func(x:({}.missing))->int:0`, false},
		{"invalid_empty_result", `f:func(x:int&string)->({}.missing):0`, false},
		{"missing_alias_bound", `Box(A:int)={value:A}
f(A):func(x:A)->Box(A):{value:x}`, false},
		{"missing_hidden_inventory", `f:func(r:_)->(func()->int):func()->int:r._secret`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := cuecontext.New().CompileString(tt.source)
			f := root.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid test source: %v", root.Err())
			}
			if err := f.Validate(); (err == nil) != tt.valid {
				t.Fatalf("uncalled definition: valid=%v: %v", tt.valid, err)
			}
			if err := root.Validate(); (err == nil) != tt.valid {
				t.Fatalf("module validation: valid=%v: %v", tt.valid, err)
			}
		})
	}
}

func TestQuantifiedDefinitionCheckingPreservesConcreteDemand(t *testing.T) {
	v := cuecontext.New().CompileString(`
config: {value: int}
choice: *{saved: config} | {saved: "other"}
`).LookupPath(cue.ParsePath("choice"))
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("a static visit discharged a shared field's concrete obligation")
	}
}
