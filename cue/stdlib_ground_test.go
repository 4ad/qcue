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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestStdlibGroundResults(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"literal", `f:func()->"ABC":strings.ToUpper("abc")`, true},
		{"wrong_literal", `f:func()->"abc":strings.ToUpper("abc")`, false},
		{"singleton", `f:func(s:"abc")->"ABC":strings.ToUpper(s)`, true},
		{"alternatives", `f:func(s:"a"|"b")->("A"|"B"):strings.ToUpper(s)`, true},
		{"missing_alternative", `f:func(s:"a"|"b")->"A":strings.ToUpper(s)`, false},
		{"generic_singleton", `f(A:"abc"):func(s:A)->"ABC":strings.ToUpper(s)`, true},
		{"generic_not_singleton", `f(A:string):func(s:A)->"ABC":strings.ToUpper(s)`, false},
		{"default_is_not_domain", `f:func(s:string="abc")->"ABC":strings.ToUpper(s)`, false},
		{"alias", `upper:strings.ToUpper
f:func()->"ABC":upper("abc")`, true},
		{"labels", `f:func()->"aa":strings.Repeat(count:2,s:"a")`, true},
		{"native_default", `f:func()->"b":path.Base("a/b")`, true},
		{"wrong_argument", `f:func()->42:strings.ToUpper(1)`, false},
		{"wrong_label", `f:func()->42:strings.ToUpper(wrong:"a")`, false},
		{"wrong_arity", `f:func()->42:strings.ToUpper("a","b")`, false},
		{"numeric", `f:func()->2.0:math.Sqrt(4)`, true},
		{"numeric_wrong", `f:func()->4.0:math.Sqrt(4)`, false},
		{"numeric_wrong_kind", `f:func()->2:math.Sqrt(4)`, false},
		{"integer_division", `f:func()->2:div(5,2)`, true},
		{"integer_modulus", `f:func()->1:mod(5,2)`, true},
		{"integer_quotient", `f:func()->-2:quo(-5,2)`, true},
		{"integer_remainder", `f:func()->-1:rem(-5,2)`, true},
		{"integer_division_wrong", `f:func()->3:div(5,2)`, false},
		{"integer_division_bad_input", `f:func()->42:div("5",2)`, false},
		{"list", `f:func()->"a,b":strings.Join(["a","b"],",")`, true},
		{"list_parameter", `f:func(xs:["a","b"])->"a,b":strings.Join(xs,",")`, true},
		{"nested_alternatives", `f:func(xs:["a"|"b","c"])->("a,c"|"b,c"):strings.Join(xs,",")`, true},
		{"nested_alternative_missing", `f:func(xs:["a"|"b","c"])->"a,c":strings.Join(xs,",")`, false},
		{"open_list", `f:func(xs:["a",...string])->"a":strings.Join(xs,",")`, false},
		{"incomplete_list", `f:func(xs:[string])->"a":strings.Join(xs,",")`, false},
		{"record_constructor", `f:func()->"{\"a\":1}":json.Marshal({a:1})`, true},
		{"record_constructor_order", `f:func()->"{\"b\":2,\"a\":1}":json.Marshal({b:2,a:1})`, true},
		{"projected_constructor", `f:func(x:{a:1})->"{\"a\":1}":json.Marshal({a:x.a})`, true},
		{"nested_constructor", `f:func()->"[{\"a\":1}]":json.Marshal([{a:1}])`, true},
		{"open_record", `f:func(x:{a:1})->"{\"a\":1}":json.Marshal(x)`, false},
		{"record_order", `f:func(x:close({a:1,b:2}))->"{\"a\":1,\"b\":2}":json.Marshal(x)`, false},
		{"record_order_after_meet", `f:func(x:close({a:1,b:2}))->"{\"a\":1,\"b\":2}":json.Marshal(x & {})`, false},
		{"open_record_after_meet", `f:func(x:{a:1})->"{\"a\":1}":json.Marshal(x & {})`, false},
		{"optional_record", `f:func(x:close({a:1,b?:2}))->"{\"a\":1}":json.Marshal(x)`, false},
		{"nested_open_record", `f:func(xs:[{a:1}])->"[{\"a\":1}]":json.Marshal(xs)`, false},
		{"decoded_fields", `f:func()->{a:1}:json.Unmarshal("{\"a\":1}")`, true},
		{"decoded_wrong_fields", `f:func()->{a:2}:json.Unmarshal("{\"a\":1}")`, false},
		{"nested_calls", `f:func()->"{\"a\":1}":json.Marshal(json.Unmarshal("{\"a\":1}"))`, true},
		{"sort_strings", `f:func()->["a","b"]:list.SortStrings(["b","a"])`, true},
		{"sort_strings_wrong", `f:func()->["b","a"]:list.SortStrings(["b","a"])`, false},
		{"capability", `f:strings.Join & (func(["a","b"],",")->"a,b")`, true},
		{"false_capability", `f:strings.Join & (func(["a","b"],",")->"a")`, false},
		{"default_capability", `f:path.Base & (func("a/b")->"b")`, true},
		{"generic_capability", `f:strings.ToUpper & (forall(A:"abc") func(A)->"ABC")`, true},
		{"false_generic_capability", `f:strings.ToUpper & (forall(A:string) func(A)->"ABC")`, false},
		{"bytes", `f:func()->'bc':strings.ByteSlice("abcd",1,3)`, true},
		{"bytes_wrong", `f:func()->'ab':strings.ByteSlice("abcd",1,3)`, false},
		{"csv", `f:func()->[["a","b"]]:csv.Decode("a,b\n")`, true},
		{"csv_wrong", `f:func()->[["b","a"]]:csv.Decode("a,b\n")`, false},
		{"record_projection", `f:func()->2026:time.Split("2026-01-01T00:00:00Z").year`, true},
		{"record_projection_wrong", `f:func()->2025:time.Split("2026-01-01T00:00:00Z").year`, false},
		{"validator_call", `f:func()->true:strings.MinRunes("abc",2)`, true},
		{"validator_call_wrong", `f:func()->false:strings.MinRunes("abc",2)`, false},
		{"bare_validator_call", `f:func()->true:json.Valid("{}")`, true},
		{"bare_validator_call_wrong", `f:func()->false:json.Valid("{}")`, false},
		{"contains", `f:func()->true:list.Contains(["a","b"],"a")`, true},
		{"contains_wrong", `f:func()->false:list.Contains(["a","b"],"a")`, false},
		{"known_failure", `f:func()->string:strconv.Atoi("bad")`, true},
		{"range", `f:func()->[0,1,2]:list.Range(0,3,1)`, true},
		{"fractional_range", `f:func()->[0,0.25,0.50,0.75]:list.Range(0.0,1.0,0.25)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var imports string
			for _, pkg := range []string{"strings", "path", "math", "list", "encoding/json", "encoding/csv", "time", "strconv"} {
				name := pkg[strings.LastIndex(pkg, "/")+1:]
				if strings.Contains(tc.source, name+".") {
					imports += fmt.Sprintf("import %q\n", pkg)
				}
			}
			checkOperatorDefinition(t, imports+tc.source, tc.valid)
		})
	}
}

func TestStdlibGroundCaptureRefinement(t *testing.T) {
	v := cuecontext.New().CompileString(`import "encoding/json"
x: {a:1}
f: func()->"{\"a\":1}":json.Marshal(x)
`)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := v.FillPath(cue.ParsePath("x.b"), 2)
	if err := changed.LookupPath(cue.ParsePath("f")).Validate(); err == nil {
		t.Fatal("a refined capture retained its old serialization result")
	}
}

func TestStdlibGroundWorkBound(t *testing.T) {
	// The declared contract remains usable when nested alternatives make
	// exhaustive evaluation impractical. Incompleteness is not emptiness.
	params := "[" + strings.Repeat(`"a"|"b",`, 20) + "]"
	for _, tc := range []struct {
		result string
		valid  bool
	}{{"string", true}, {`"a"`, false}} {
		source := fmt.Sprintf("import \"strings\"\nf:func(xs:%s)->%s:strings.Join(xs,\",\")", params, tc.result)
		checkOperatorDefinition(t, source, tc.valid)
	}
	for _, source := range []string{
		`import "strings"
f:func()->string:strings.Repeat("a",1000000)`,
		`import "strings"
f:func()->string:strings.Repeat(count:1000000,s:"a")`,
		`import "list"
f:func()->[...int]:list.Repeat([1],1000000)`,
		`import "list"
f:func()->[...int]:list.Range(0,1000000000,1)`,
		`import "list"
f:func()->[...number]:list.Range(1e100,1e100+1,1)`,
		`import "math/bits"
f:func()->int:bits.Lsh(1,1000000000)`,
		`import "strconv"
f:func()->string:strconv.FormatFloat(1.0,"f",1000000000,64)`,
	} {
		checkOperatorDefinition(t, source, true)
	}
}

func TestStdlibGroundExecution(t *testing.T) {
	v := cuecontext.New().CompileString(`import (
"strings"
"encoding/json"
)
upper:func(s:"a"|"b")->("A"|"B"):strings.ToUpper(s)
join:func(xs:["a"|"b","c"])->("a,c"|"b,c"):strings.Join(xs,",")
encode:func()->"[{\"a\":1}]":json.Marshal([{a:1}])
out:[upper("a"),upper("b"),join(["a","c"]),join(["b","c"]),encode()]
`)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
	want := `["A","B","a,c","b,c","[{\"a\":1}]"]`
	if err != nil || string(got) != want {
		t.Fatalf("got %s, %v; want %s", got, err, want)
	}
}

func TestStdlibGroundFailure(t *testing.T) {
	v := cuecontext.New().CompileString(`import "strconv"
f:func()->string:strconv.Atoi("bad")
out:f()
`)
	if err := v.LookupPath(cue.ParsePath("f")).Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
		t.Fatal("a failing native call materialized its result annotation")
	}
}
