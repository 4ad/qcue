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

func TestStdlibBase64EncodingDomain(t *testing.T) {
	for _, fn := range []struct {
		name, input, result string
	}{
		{"EncodedLen", "int", "int"},
		{"DecodedLen", "int", "int"},
		{"Encode", "string|bytes", "string"},
		{"Decode", "string", "bytes"},
	} {
		for _, encoding := range []string{"null", "string", "int", "null|string"} {
			for _, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/partial=%v", fn.name, encoding, partial), func(t *testing.T) {
					body := fmt.Sprintf("base64.%s(e,x)", fn.name)
					if partial {
						body = fmt.Sprintf("base64.%s(encoding:e,...)(x)", fn.name)
					}
					source := fmt.Sprintf("import \"encoding/base64\"\nf:func(e:%s,x:%s)->%s:%s", encoding, fn.input, fn.result, body)
					checkOperatorDefinition(t, source, encoding == "null")
				})
			}
		}
	}
}

func TestStdlibCodecDataDomains(t *testing.T) {
	for _, tc := range []struct {
		pkg, source string
		valid       bool
	}{
		{"text/template", `f:func(x:{name:string})->string:template.Execute("{{.name}}",x)`, true},
		{"text/template", `f:func(x:null|bool|number|string|bytes|[...]|{...})->string:template.Execute("{{.}}",x)`, true},
		{"text/template", `f:func(x:func(int)->int)->string:template.Execute("{{.}}",x)`, false},
		{"text/template", `f:func(x:string|func(int)->int)->string:template.Execute("{{.}}",x)`, false},
		{"text/template", `f:func(x:func(int)->int)->string:template.Execute(data:x,...)("{{.}}")`, false},
		{"encoding/csv", `f:func(xs:[...[...(null|bool|number|string|bytes|[...]|{...})]])->string:csv.Encode(xs)`, true},
		{"encoding/csv", `f:func(xs:[...[...func(int)->int]])->string:csv.Encode(xs)`, false},
		{"encoding/csv", `f:func(xs:[...[...(string|func(int)->int)]])->string:csv.Encode(xs)`, false},
		{"encoding/csv", `f:func(xs:[...[...func(int)->int]])->string:csv.Encode(xs,...)()`, false},
	} {
		t.Run(tc.pkg+"/"+tc.source, func(t *testing.T) {
			checkOperatorDefinition(t, fmt.Sprintf("import %q\n%s", tc.pkg, tc.source), tc.valid)
		})
	}
}

func TestStdlibCodecContractExecution(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import (
"encoding/base64"
"encoding/csv"
"text/template"
)
encode:base64.Encode(encoding:null,...)
decode:base64.Decode(encoding:null,...)
render:func(x:{name:string})->string:template.Execute("Hello {{.name}}",x)
table:func(x:[...[...(string|int)]])->string:csv.Encode(x)
out:[encode("hello"),"\(decode("aGVsbG8="))",base64.EncodedLen(null,5),base64.DecodedLen(null,8),render({name:"Ada"}),table([["a",1],["b",2]])]
`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", `["aGVsbG8=","hello",8,6,"Hello Ada","a,1\nb,2\n"]`)
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
