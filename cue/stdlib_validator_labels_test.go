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

func TestStdlibValidatorLabels(t *testing.T) {
	for _, tc := range []struct {
		pkg, source string
		valid       bool
	}{
		{"encoding/json", `f:func(s:string)->bool:json.Valid(data:s)`, true},
		{"encoding/json", `f:func(s:int)->bool:json.Valid(data:s)`, false},
		{"encoding/json", `f:func(s:string)->bool:json.Valid(wrong:s)`, false},
		{"encoding/json", `f:func(s:string)->bool:json.Valid(s,data:s)`, false},
		{"encoding/json", `f:func(s:string)->bool:json.Valid(data:s,data:s)`, false},
		{"encoding/json", `f:func(s:string)->bool:json.Valid(data:s,...)()`, true},
		{"encoding/json", `f:func(s:string)->bool:json.Valid()(data:s)`, false},
		{"net", `f:func(s:string)->bool:net.IPv4(ip:s)`, true},
		{"net", `f:func(s:int)->bool:net.IPv4(ip:s)`, false},
		{"list", `f:func(xs:[...string])->bool:list.IsSortedStrings(a:xs)`, true},
		{"list", `f:func(xs:[...int])->bool:list.IsSortedStrings(a:xs)`, false},
		{"uuid", `f:func(s:string)->true:uuid.Valid(s:s)`, true},
		{"uuid", `f:func(s:string)->false:uuid.Valid(s:s)`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			checkOperatorDefinition(t, "import \""+tc.pkg+"\"\n"+tc.source, tc.valid)
		})
	}
}

func TestStdlibValidatorLabelExecution(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "encoding/json"
import "net"
import "list"
import "uuid"
valid:json.Valid
saved:valid(data:"{}",...)
out:[valid(data:"{}"),valid(data:"invalid"),saved(),net.IPv4(ip:"127.0.0.1"),list.IsSortedStrings(a:["a","b"]),uuid.Valid(s:"00000000-0000-0000-0000-000000000000")]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `[true,false,true,true,true,true]`)
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
		if err := v.Validate(); err != nil {
			t.Fatalf("%s\n%v", source, err)
		}
	}
	for _, call := range []string{
		`json.Valid(wrong:"{}")`,
		`json.Valid("{}",data:"{}")`,
		`json.Valid(data:"{}",data:"{}")`,
		`json.Valid(data:1)`,
		`json.Valid()(data:"{}")`,
	} {
		if v := ctx.CompileString("import \"encoding/json\"\nbad:" + call); v.Err() == nil {
			t.Fatalf("accepted invalid runtime call %s", call)
		}
	}
}

func TestStdlibValidatorLabelRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "encoding/json"
data:string
valid:json.Valid(data:data)
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := v.LookupPath(cue.ParsePath("valid")).Bool(); err == nil {
			t.Fatal("named call completed an unknown argument")
		}
		semanticJSON(t, v.FillPath(cue.ParsePath("data"), "{}"), "valid", "true")
		semanticJSON(t, v.FillPath(cue.ParsePath("data"), "invalid"), "valid", "false")
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
