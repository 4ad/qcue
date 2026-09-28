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

func TestStdlibSavedSchemas(t *testing.T) {
	for _, tc := range []struct {
		name, source, bad string
	}{
		{"json_private", `import "encoding/json"
f:{schema:{a:int},out:json.Validate(v:schema,...)}.out
out:f("{\"a\":1}")`, `f("{\"a\":\"wrong\"}")`},
		{"json_identity", `import "encoding/json"
schema:{a:int}
f:json.Validate(v:schema,...) & json.Validate(v:schema,...)
out:f("{\"a\":1}")`, `f("{\"a\":\"wrong\"}")`},
		{"json_factory", `import "encoding/json"
make:func(n:int)->func(string|bytes)->bool:json.Validate(v:{a:>=n},...)
f:make(3) & make(3)
out:f("{\"a\":4}")`, `f("{\"a\":2}")`},
		{"json_callback", `import "encoding/json"
wrap:func(f:func(string|bytes)->bool)->func(string|bytes)->bool:func(x:string|bytes)->bool:f(x)
f:wrap({schema:{a:int},out:json.Validate(v:schema,...)}.out)
out:f("{\"a\":1}")`, `f("{\"a\":\"wrong\"}")`},
		{"yaml_private", `import "encoding/yaml"
f:{schema:{a:int},out:yaml.Validate(v:schema,...)}.out
out:f("a: 1")`, `f("a: wrong")`},
		{"yaml_partial", `import "encoding/yaml"
f:{schema:{a:int,b:string},out:yaml.ValidatePartial(v:schema,...)}.out
out:f("a: 1")`, `f("a: wrong")`},
		{"match_n", `import "list"
f:{n:>=1,schema:int,out:list.MatchN(n:n,matchValue:schema,...)}.out
out:f([1,"x"])`, `f([true,"x"])`},
		{"match_n_identity", `import "list"
n:>=1
schema:int
f:list.MatchN(n:n,matchValue:schema,...) & list.MatchN(n:n,matchValue:schema,...)
out:f([1,"x"])`, `f([true,"x"])`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tc.source)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", `true`)
				bad := v.Unify(ctx.CompileString("f:_\nbad:" + tc.bad))
				if got, err := bad.LookupPath(cue.ParsePath("bad")).Bool(); err == nil && got {
					t.Fatal("saved validator accepted data outside its schema")
				} else if err != nil && bad.Validate() == nil {
					t.Fatalf("invalid concrete data remained unresolved: %v", err)
				}
				source, err := format.Node(v.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileBytes(source)
				if err := v.Validate(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
			}
		})
	}
}

func TestStdlibSavedOpenAPISchema(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "encoding/openapi"
f:{schema:{#A:{a:string}},out:openapi.MarshalSchema(schema:schema,...)}.out
out:f({version:"3.0.0"})`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := v.LookupPath(cue.ParsePath("out")).String(); err != nil {
			t.Fatal(err)
		}
		if err := v.Unify(ctx.CompileString(`f:_
bad:f({version:42})`)).Validate(); err == nil {
			t.Fatal("saving the schema bypassed configuration type checking")
		}
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

func TestStdlibSavedSchemaIdentities(t *testing.T) {
	for _, source := range []string{
		`make:func(n:int)->func(string|bytes)->bool:json.Validate(v:{a:>=n},...)
left:make(1)
right:make(2)`,
		`template:{n:int,f:json.Validate(v:{a:>=n},...)}
left:(template & {n:1}).f
right:(template & {n:2}).f`,
	} {
		t.Run(source, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("import \"encoding/json\"\n" + source + `
out:[left("{\"a\":3}"),right("{\"a\":3}")]`)
			for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
				semanticJSON(t, v, "out", `[true,true]`)
				bad := v.Unify(ctx.CompileString(`left:_
right:_
bad:(left & right)("{\"a\":3}")`))
				if _, err := bad.LookupPath(cue.ParsePath("bad")).Bool(); err == nil {
					t.Fatal("different saved schemas acquired the same identity")
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

func TestStdlibSavedSchemaRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`import "encoding/json"
schema:{a:number}
f:json.Validate(v:schema,...)
out:f("{\"a\":1}")`)
	for _, opts := range [][]cue.Option{nil, {cue.Raw()}, {cue.Final()}} {
		semanticJSON(t, v, "out", `true`)
		semanticJSON(t, v.Unify(ctx.CompileString(`schema:{a:int}`)), "out", `true`)
		bad := v.Unify(ctx.CompileString(`schema:{a:>2}`))
		if got, err := bad.LookupPath(cue.ParsePath("out")).Bool(); err == nil && got {
			t.Fatal("saved validator froze its original schema")
		} else if err != nil && bad.Validate() == nil {
			t.Fatalf("refined concrete failure remained unresolved: %v", err)
		}
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
