// Copyright 2026 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pkg_test

import (
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/export"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/pkg"
)

// Exercise the public checker for every registered native, so new packages and
// functions cannot silently fall outside the primitive typing rules.
func TestBuiltinFunctionChecking(t *testing.T) {
	r := runtime.New()
	for _, ip := range pkg.ImportPaths() {
		vertex := r.LoadBuiltin(ip)
		if vertex == nil {
			continue
		}
		for _, arc := range vertex.Arcs {
			var b *adt.Builtin
			switch v := arc.BaseValue.(type) {
			case *adt.Builtin:
				b = v
			case *adt.BuiltinValidator:
				b = v.Builtin
			default:
				continue
			}
			t.Run(ip+"/"+b.Name, func(t *testing.T) {
				var params, args []string
				for i, param := range b.Params {
					name := fmt.Sprintf("a%d", i)
					typ := builtinKindSyntax(param.Kind())
					if _, basic := param.Value.(*adt.BasicType); !basic {
						constraint := param.Value
						if union, ok := constraint.(*adt.Disjunction); ok {
							copy := *union
							copy.NumDefaults, copy.HasDefaults = 0, false
							constraint = &copy
						}
						expr, err := export.Value(r, "", constraint)
						if err != nil {
							t.Fatal(err)
						}
						text, formatErr := format.Node(expr)
						if formatErr != nil {
							t.Fatal(formatErr)
						}
						typ = string(text)
					}
					params = append(params, name+": "+typ)
					args = append(args, name)
				}
				resultType := builtinKindSyntax(b.Result)
				if b.Result == adt.BottomKind {
					resultType = "_"
				}
				source := fmt.Sprintf("import native %q\nf: func(%s) -> %s: native.%s(%s)",
					ip, strings.Join(params, ", "), resultType, b.Name, strings.Join(args, ", "))
				v := cuecontext.New().CompileString(source)
				if err := v.Validate(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
				// A correct broad return must not let the same call prove a singleton.
				// Bottom-returning natives have no successful result to contradict it.
				if b.Result != adt.BottomKind {
					source = strings.Replace(source, " -> "+builtinKindSyntax(b.Result)+":", " -> {impossible: 42}:", 1)
					bad := cuecontext.New().CompileString(source)
					if err := bad.LookupPath(cue.ParsePath("f")).Validate(); err == nil {
						t.Fatalf("unproved result accepted: %s", source)
					}
				}
			})
		}
	}
}

func builtinKindSyntax(k adt.Kind) string {
	if k == adt.TopKind {
		return "_"
	}
	if k == adt.BottomKind {
		return "_|_"
	}
	var terms []string
	for _, entry := range []struct {
		kind adt.Kind
		text string
	}{
		{adt.NullKind, "null"}, {adt.BoolKind, "bool"}, {adt.IntKind, "int"},
		{adt.FloatKind, "float"}, {adt.StringKind, "string"}, {adt.BytesKind, "bytes"},
		{adt.StructKind, "{...}"}, {adt.ListKind, "[...]"},
	} {
		if k&entry.kind != 0 {
			terms = append(terms, entry.text)
		}
	}
	return "(" + strings.Join(terms, " | ") + ")"
}
