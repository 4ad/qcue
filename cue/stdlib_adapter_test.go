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

func TestStdlibAdapterRefinement(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		values       []any
	}{
		{"host", `import "net"
x:int
out:net.JoinHostPort([127,0,0,x],80)`, []any{1, 2}},
		{"saved_host", `import "net"
x:int
f:net.JoinHostPort(port:80,...)
out:f([127,0,0,x])`, []any{1, 2}},
		{"openapi_self_contained", `import "encoding/openapi"
x:bool
out:openapi.MarshalSchema({version:"3.0.0",selfContained:x},{#A:{a:string},#B:#A})`, []any{true, false}},
		{"openapi_expand_references", `import "encoding/openapi"
x:bool
out:openapi.MarshalSchema({version:"3.0.0",expandReferences:x},{#A:{a:string},#B:#A})`, []any{true, false}},
		{"saved_openapi", `import "encoding/openapi"
x:bool
f:openapi.MarshalSchema(config:{version:"3.0.0",expandReferences:x},...)
out:f({#A:{a:string},#B:#A})`, []any{true, false}},
		{"openapi_info", `import "encoding/openapi"
x:string
out:openapi.MarshalSchema({version:"3.0.0",info:{title:x,version:"1"}},{#A:{a:string}})`, []any{"First", "Second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.source)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if _, err := v.LookupPath(cue.ParsePath("out")).String(); err == nil {
					t.Fatal("adapter produced a string from unresolved input")
				}
				if err := v.Validate(); err != nil {
					t.Fatalf("unresolved input became a permanent error: %v", err)
				}
				for _, x := range tc.values {
					got, err := v.FillPath(cue.ParsePath("x"), x).LookupPath(cue.ParsePath("out")).String()
					if err != nil {
						t.Fatal(err)
					}
					want, err := ctx.CompileString(tc.source).FillPath(cue.ParsePath("x"), x).LookupPath(cue.ParsePath("out")).String()
					if err != nil || got != want {
						t.Fatalf("after refinement: got %q; want %q, %v", got, want, err)
					}
				}
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
			}
		})
	}
}

func TestStdlibOpenAPIConfigDomain(t *testing.T) {
	for _, tc := range []struct {
		info  string
		valid bool
	}{
		{"{title:string,version:string}", true},
		{"{...}", true},
		{"null", false},
		{"string", false},
		{"int", false},
		{"[...]", false},
		{"func()->int", false},
		{"string|{...}", false},
	} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%v", tc.info, partial), func(t *testing.T) {
				call := "openapi.MarshalSchema({version:\"3.0.0\",info:x},{#A:{a:string}})"
				if partial {
					call = "openapi.MarshalSchema(config:{version:\"3.0.0\",info:x},...)({#A:{a:string}})"
				}
				source := fmt.Sprintf("import \"encoding/openapi\"\nf:func(x:%s)->string:%s", tc.info, call)
				checkOperatorDefinition(t, source, tc.valid)
			})
		}
	}
}

func TestStdlibSavedConfigRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "encoding/openapi"
config:{version:"3.0.0"}
f:openapi.MarshalSchema(config,...)
out:f({#A:{a:string}})`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if _, err := v.LookupPath(cue.ParsePath("out")).String(); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"selfContained", "expandReferences"} {
			path := cue.ParsePath("config." + field)
			if err := v.FillPath(path, true).Validate(); err != nil {
				t.Fatal(err)
			}
			if err := v.FillPath(path, "invalid").Validate(); err == nil {
				t.Fatalf("the absent-field proof survived an incompatible %s", field)
			}
		}
		source, err := format.Node(v.Syntax(opts...))
		if err != nil {
			t.Fatal(err)
		}
		v = ctx.CompileBytes(source)
	}
}
