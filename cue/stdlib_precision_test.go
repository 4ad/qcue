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
	"cuelang.org/go/cue/format"
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
		{"sort", `f(A):func(xs:[...A],cmp:{x:_,y:_,less:bool})->[...A]:list.Sort(xs,cmp)`, true},
		{"sort_stable", `f(A):func(xs:[...A],cmp:{x:_,y:_,less:bool})->[...A]:list.SortStable(xs,cmp)`, true},
		{"sort_strings", `f(A:string):func(xs:[...A])->[...A]:list.SortStrings(xs)`, true},
		{"sort_capability", `f:list.Sort & (forall(A) func([...A],{x:_,y:_,less:bool})->[...A])`, true},
		{"sort_incompatible_capability", `f:list.Sort & (func([...string],{x:int,y:int,less:bool})->[...string])`, false},
		{"sort_incompatible_inputs", `f:func(xs:[...string],cmp:{x:int,y:int,less:bool})->[...string]:list.Sort(xs,cmp)`, false},
		{"sort_record_fields", `f(A:{a:int}):func(xs:[...A])->[...A]:list.Sort(xs,{x:{},y:{},less:x.a<y.a})`, true},
		{"sort_wrong_field", `f(A:{a:int}):func(xs:[...A])->[...A]:list.Sort(xs,{x:{},y:{},less:x.b<y.b})`, false},
		{"sort_named", `f:func(xs:[...int])->[...int]:list.Sort(cmp:list.Ascending,list:xs)`, true},
		{"sort_wrong_elements", `f:func(xs:[...int])->[...string]:list.Sort(xs,list.Ascending)`, false},
		{"sort_missing_comparator", `f:func(xs:[...int])->_:list.Sort(xs,{})`, false},
		{"sort_bad_comparator", `f:func(xs:[...int])->_:list.Sort(xs,{x:_,y:_,less:1})`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkOperatorDefinition(t, "import \"list\"\n"+tc.source, tc.valid)
		})
	}
}

func TestStdlibNumericRefinements(t *testing.T) {
	for _, tc := range []struct {
		pkg, params, body, result, wrong, args string
	}{
		{"math", "x:number", "math.Abs(x)", "number & >=0", "number & >0", "0"},
		{"math", "x:int", "math.Abs(x)", "int & >=0", "int & >0", "0"},
		{"math", "x:number", "math.Acosh(x)", "number & >=0", "number & >0", "1"},
		{"math", "x:number", "math.Acos(x)", "number & >=0", "number & >0", "1"},
		{"math", "x:number", "math.Cosh(x)", "number & >=1", "number & >1", "0"},
		{"math", "x:number,y:number", "math.Dim(x,y)", "number & >=0", "number & >0", "0,0"},
		{"math", "x:number", "math.Erf(x)", "number & >=-1 & <=1", "number & >=0", "-1"},
		{"math", "x:number", "math.Erfc(x)", "number & >=0 & <=2", "number & <=1", "-1"},
		{"math", "x:number", "math.Exp(x)", "number & >=0", "number & >0", "-1000"},
		{"math", "x:number", "math.Exp2(x)", "number & >=0", "number & >0", "-2000"},
		{"math", "x:number", "math.Expm1(x)", "number & >=-1", "number & >-1", "-1000"},
		{"math", "x:number,y:number", "math.Hypot(x,y)", "number & >=0", "number & >0", "0,0"},
		{"math", "x:int", "math.Pow10(x)", "number & >=0", "number & >0", "-1000"},
		{"math", "x:number", "math.Cos(x)", "number & >=-1 & <=1", "number & <1", "0"},
		{"math", "x:number", "math.Sin(x)", "number & >=-1 & <=1", "number & >=0", "-1"},
		{"math", "x:number", "math.Sqrt(x)", "number & >=0", "number & >0", "0"},
		{"math", "x:number", "math.Tanh(x)", "number & >=-1 & <=1", "number & >=0", "-1"},
		{"math", "x:int,y:int", "math.Jacobi(x,y)", "-1|0|1", "0|1", "2,3"},
		{"math/bits", "x:int,i:int", "bits.At(x,i)", "0|1", "1", "0,0"},
		{"math/bits", "x:int", "bits.OnesCount(x)", "int & >=0", "int & >0", "0"},
		{"math/bits", "x:int", "bits.Len(x)", "int & >=0", "int & >0", "0"},
		{"list", "xs:[...int]", "list.Sum(xs)", "int", "int & >0", "[]"},
		{"list", "xs:[...int]", "list.Product(xs)", "int", "int & >1", "[]"},
		{"list", "x:int,y:int,n:int", "list.Range(x,y,n)", "[...int]", "[...string]", "0,3,1"},
		{"strings", "s:string|bytes", "strings.ByteAt(s,0)", "int & >=0 & <=255", "int & <255", `'\xff'`},
		{"strings", "s:string", "strings.Count(s,\"a\")", "int & >=0", "int & >0", `""`},
		{"strings", "s:string", "strings.Index(s,\"a\")", "int & >=-1", "int & >=0", `""`},
		{"strings", "s:string", "strings.LastIndex(s,\"a\")", "int & >=-1", "int & >=0", `""`},
		{"strings", "s:string", "strings.IndexAny(s,\"a\")", "int & >=-1", "int & >=0", `""`},
		{"strings", "s:string", "strings.LastIndexAny(s,\"a\")", "int & >=-1", "int & >=0", `""`},
		{"strings", "s:string,t:string", "strings.Compare(s,t)", "-1|0|1", "0|1", `"a","b"`},
		{"strconv", "s:string", "strconv.ParseUint(s,10,0)", "int & >=0", "int & >0", `"0"`},
		{"net", "s:string,t:string", "net.CompareIP(s,t)", "-1|0|1", "1", `"127.0.0.1","127.0.0.1"`},
		{"uuid", "s:string", "uuid.ToInt(s)", "int & >=0", "int & >0", `""`},
		{"uuid", "s:string", "uuid.Version(s)", "int & >=0 & <=15", "int & >0", `"00000000-0000-0000-0000-000000000000"`},
		{"uuid", "s:string", "uuid.Variant(s)", "int & >=0 & <=4", "int & >2", `"00000000-0000-0000-0000-000000000000"`},
	} {
		t.Run(tc.body+"/"+tc.params, func(t *testing.T) {
			source := fmt.Sprintf("import %q\nf:func(%s)->(%s):%s", tc.pkg, tc.params, tc.result, tc.body)
			checkOperatorDefinition(t, source, true)
			checkOperatorDefinition(t, fmt.Sprintf("import %q\nf:func(%s)->(%s):%s", tc.pkg, tc.params, tc.wrong, tc.body), false)
			v := cuecontext.New().CompileString(source + "\nout:f(" + tc.args + ")")
			if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil {
				t.Fatalf("native result violates its contract: %v", err)
			}
		})
	}
}

func TestStdlibStructuralContracts(t *testing.T) {
	for _, tc := range []struct {
		pkg, params, body, result, wrong, args string
	}{
		{"net", "s:string", "net.SplitHostPort(s)", "[string,string]", "[int,string]", `"localhost:80"`},
		{"net", "s:string", "net.ToIP4(s)", "[int,int,int,int]", "[int,int,int]", `"127.0.0.1"`},
		{"net", "s:string", "len(net.ToIP16(s))", "16", "4", `"::1"`},
		{"net", "s:string", "len(net.ParseIP(s))", "4|16", "4", `"::1"`},
		{"net", "s:string", "net.ParseCIDR(s).prefix_len", "int & >=0 & <=128", "int & <=32", `"::1/128"`},
		{"time", "s:string", "time.Split(s).month", "int & >=1 & <=12", "int & <12", `"2026-12-31T23:59:59Z"`},
		{"time", "s:string", "time.Split(s).day", "int & >=1 & <=31", "int & <31", `"2026-12-31T23:59:59Z"`},
		{"time", "s:string", "time.Split(s).hour", "int & >=0 & <=23", "int & <23", `"2026-12-31T23:59:59Z"`},
		{"time", "s:string", "time.Split(s).minute", "int & >=0 & <=59", "int & <59", `"2026-12-31T23:59:59Z"`},
		{"time", "s:string", "time.Split(s).second", "int & >=0 & <=59", "int & <59", `"2026-12-31T23:59:59Z"`},
		{"time", "s:string", "time.Split(s).nanosecond", "int & >=0 & <1000000000", "int & <999999999", `"2026-12-31T23:59:59.999999999Z"`},
		{"strings", "s:string", "strings.Runes(s)", "[...(int & >=0 & <=0x10ffff)]", "[...(int & <=127)]", `"λ"`},
		{"encoding/json", "s:string", "json.UnmarshalStream(s)", "[...]", "[...int]", `"1\n\"x\""`},
		{"encoding/yaml", "s:string", "yaml.UnmarshalStream(s)", "[...]", "[...int]", `"1\n---\nx\n"`},
		{"encoding/toml", "s:string", "toml.Unmarshal(s)", "{...}", "{x:int}", `"name = 'Ada'"`},
		{"list", "xs:[...int]", "list.Sort(xs,list.Ascending)", "[...int]", "[...string]", "[3,1,2]"},
		{"list", "xs:[...string]", "list.SortStrings(xs)", "[...string]", "[...int]", `["z","a"]`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			source := fmt.Sprintf("import %q\nf:func(%s)->(%s):%s", tc.pkg, tc.params, tc.result, tc.body)
			checkOperatorDefinition(t, source, true)
			checkOperatorDefinition(t, fmt.Sprintf("import %q\nf:func(%s)->(%s):%s", tc.pkg, tc.params, tc.wrong, tc.body), false)
			v := cuecontext.New().CompileString(source + "\nout:f(" + tc.args + ")")
			if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil {
				t.Fatalf("native result violates its contract: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		pkg, source string
		valid       bool
	}{
		{"encoding/csv", `f:func(xs:[...[...string]])->string:csv.Encode(xs)`, true},
		{"encoding/csv", `f:func(xs:[...string])->string:csv.Encode(xs)`, false},
		{"encoding/json", `f:func(xs:[...int])->string:json.MarshalStream(xs)`, true},
		{"encoding/json", `f:func(xs:{x:int})->string:json.MarshalStream(xs)`, false},
		{"encoding/yaml", `f:func(xs:[...int])->string:yaml.MarshalStream(xs)`, true},
		{"encoding/yaml", `f:func(xs:{x:int})->string:yaml.MarshalStream(xs)`, false},
		{"encoding/toml", `f:func(x:{name:string})->string:toml.Marshal(x)`, true},
		{"encoding/toml", `f:func(x:[...string])->string:toml.Marshal(x)`, false},
		{"net", `f:func(x:string|bytes|[...int])->string:net.IPString(x)`, true},
		{"net", `f:func(x:[...string])->string:net.IPString(x)`, false},
		{"strconv", `f:func(x:number,format:string|int)->string:strconv.FormatFloat(x,format,-1,64)`, true},
		{"strconv", `f:func(x:number,format:bytes)->string:strconv.FormatFloat(x,format,-1,64)`, false},
		{"text/tabwriter", `f:func(x:string|bytes|[...(string|bytes)])->string:tabwriter.Write(x)`, true},
		{"text/tabwriter", `f:func(x:[...int])->string:tabwriter.Write(x)`, false},
		{"encoding/json", `f:func(x:func(int)->int)->string:json.Marshal(x)`, false},
		{"encoding/yaml", `f:func(x:func(int)->int)->string:yaml.Marshal(x)`, false},
	} {
		t.Run(tc.pkg+"/"+tc.source, func(t *testing.T) {
			checkOperatorDefinition(t, fmt.Sprintf("import %q\n%s", tc.pkg, tc.source), tc.valid)
		})
	}
}

func TestStdlibValidatorContracts(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"constructor", `f:func(n:int)->validator(string):strings.MinRunes(min:n)`, true},
		{"constructor_wrong_domain", `f:func(n:int)->validator(int):strings.MinRunes(min:n)`, false},
		{"constructor_wrong_argument", `f:func(n:string)->_:strings.MinRunes(min:n)`, false},
		{"constructor_wrong_label", `f:func(n:int)->_:strings.MinRunes(s:n)`, false},
		{"constructor_capability", `f:strings.MinRunes & (func(min:int)->validator(string))`, true},
		{"constructor_false_capability", `f:strings.MinRunes & (func(min:int)->validator(int))`, false},
		{"constructor_wrong_capability_label", `f:strings.MinRunes & (func(s:int)->validator(string))`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, "import \"strings\"\n"+tc.source, tc.valid) })
	}
	v := cuecontext.New().CompileString(`import "strings"
f:strings.MinRunes & (func(min:int)->validator(string))
good:"abc" & f(min:3)
bad:"ab" & f(min:3)`)
	if got, err := v.LookupPath(cue.ParsePath("good")).String(); err != nil || got != "abc" {
		t.Fatalf("good = %q, %v", got, err)
	}
	if err := v.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
		t.Fatal("validator accepted a short string")
	}
}

func TestStdlibPathContracts(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"os_domain", `f:func(s:string,os:path.#OS)->string:path.Clean(s,os)`, true},
		{"os_too_broad", `f:func(s:string,os:string)->string:path.Clean(s,os)`, false},
		{"wrong_os", `f:func(s:string)->string:path.Clean(s,"unknown")`, false},
		{"labels", `f:func(s:string)->[string,string]:path.Split(os:"unix",path:s)`, true},
		{"wrong_label", `f:func(s:string)->_:path.Split(unknown:s)`, false},
		{"required_os", `f:func(s:string)->string:path.ToSlash(path:s,os:"windows")`, true},
		{"missing_os", `f:func(s:string)->string:path.ToSlash(path:s)`, false},
		{"windows_default", `f:path.VolumeName & (func(path:string,os:("unix"|"windows")="windows")->string)`, true},
		{"interface_default", `f:path.VolumeName & (func(path:string,os:("unix"|"windows")="unix")->string)`, true},
		{"cannot_add_default", `f:path.ToSlash & (func(path:string,os:("unix"|"windows")="unix")->string)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { checkOperatorDefinition(t, "import \"path\"\n"+tc.source, tc.valid) })
	}
	v := cuecontext.New().CompileString(`import "path"
split: path.Split(path:"a/b")
view: path.VolumeName & (func(path:string,os:("unix"|"windows")="unix")->string)
volume: view(path:"C:\\a\\b")`)
	if got, err := v.LookupPath(cue.ParsePath("split")).MarshalJSON(); err != nil || string(got) != `["a/","b"]` {
		t.Fatalf("split = %s, %v", got, err)
	}
	if got, err := v.LookupPath(cue.ParsePath("volume")).String(); err != nil || got != "C:" {
		t.Fatalf("volume = %q, %v", got, err)
	}
}

func TestStdlibPrecisionRoundTrip(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "list"
import "math"
reverse(A):func(xs:[...A])->[...A]:list.Reverse(xs)
abs:func(x:int)->(int&>=0):math.Abs(x)
input: int
out: reverse([abs(input), 2])`)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, options := range [][]cue.Option{nil, {cue.Raw()}} {
		source, err := format.Node(v.Syntax(options...))
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := cuecontext.New().CompileBytes(source)
		filled := rebuilt.FillPath(cue.ParsePath("input"), -3)
		if got, err := filled.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[2,3]` {
			t.Fatalf("%s\nout = %s, %v", source, got, err)
		}
		if err := rebuilt.Unify(rebuilt.Context().CompileString(`abs:func(int)->string`)).Validate(); err == nil {
			t.Fatalf("lost native obligation after export: %s", source)
		}
	}
}
