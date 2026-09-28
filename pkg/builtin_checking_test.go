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
	"cuelang.org/go/internal/core/eval"
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
		if vertex == nil && ip == "tool" {
			// The tool package is injected into command files and has no
			// importable builtin registration.
			continue
		}
		if vertex == nil {
			t.Fatalf("missing registered builtin package %q", ip)
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
				ctx := eval.NewContext(r, nil)
				protocol := b.Protocol(ctx)
				var params, args []string
				var paramKinds []adt.Kind
				for i, param := range protocol.Params {
					name := fmt.Sprintf("a%d", i)
					typ, kind := builtinConstraintSyntax(t, ctx, r, param.Value)
					paramKinds = append(paramKinds, kind)
					params = append(params, name+": "+typ)
					args = append(args, name)
				}
				resultType, _ := builtinConstraintSyntax(t, ctx, r, protocol.Ret)
				if b.Result == adt.BottomKind {
					resultType = "_"
				}
				source := fmt.Sprintf("import native %q\nf: func(%s) -> (%s): native.%s(%s)",
					ip, strings.Join(params, ", "), resultType, b.Name, strings.Join(args, ", "))
				v := cuecontext.New().CompileString(source)
				if err := v.Validate(); err != nil {
					t.Fatalf("%s\n%v", source, err)
				}
				partialArgs := strings.Join(append(append([]string(nil), args...), "..."), ", ")
				partial := fmt.Sprintf("import native %q\nf: func(%s) -> (%s): native.%s(%s)()",
					ip, strings.Join(params, ", "), resultType, b.Name, partialArgs)
				if err := cuecontext.New().CompileString(partial).Validate(); err != nil {
					t.Fatalf("native partial application: %s\n%v", partial, err)
				}
				for i, kind := range paramKinds {
					// Change each constrained slot independently. Top slots
					// have no incompatible kind to test.
					for _, wrong := range []struct {
						kind  adt.Kind
						value string
					}{
						{adt.NullKind, "null"}, {adt.BoolKind, "true"},
						{adt.IntKind, "0"}, {adt.StringKind, `"wrong"`},
					} {
						if kind&wrong.kind != 0 {
							continue
						}
						badArgs := append([]string(nil), args...)
						badArgs[i] = wrong.value
						badSource := fmt.Sprintf("import native %q\nf: func(%s) -> _: native.%s(%s)",
							ip, strings.Join(params, ", "), b.Name, strings.Join(badArgs, ", "))
						bad := cuecontext.New().CompileString(badSource)
						if err := bad.Validate(); err == nil {
							t.Fatalf("invalid argument accepted: %s", badSource)
						}
						partial := fmt.Sprintf("import native %q\nf: func(%s) -> _: native.%s(%s, ...)()",
							ip, strings.Join(params, ", "), b.Name, strings.Join(badArgs, ", "))
						if err := cuecontext.New().CompileString(partial).Validate(); err == nil {
							t.Fatalf("invalid saved argument accepted: %s", partial)
						}
						break
					}
				}
				if len(b.Params) > 0 && b.IsValidator(len(b.Params)-1) {
					constructor := fmt.Sprintf("import native %q\nf: func(%s) -> %s: native.%s(%s)",
						ip, strings.Join(params[1:], ", "), builtinKindSyntax(b.Params[0].Kind()),
						b.Name, strings.Join(args[1:], ", "))
					if err := cuecontext.New().CompileString(constructor).Validate(); err != nil {
						t.Fatalf("%s\n%v", constructor, err)
					}
				}
				// A correct broad return must not let the same call prove a singleton.
				// Bottom-returning natives have no successful result to contradict it.
				if b.Result != adt.BottomKind {
					source = strings.Replace(source, " -> ("+resultType+"):", " -> {impossible: 42}:", 1)
					bad := cuecontext.New().CompileString(source)
					if err := bad.LookupPath(cue.ParsePath("f")).Validate(); err == nil {
						t.Fatalf("unproved result accepted: %s", source)
					}
				}
			})
		}
	}
}

func builtinConstraintSyntax(t *testing.T, ctx *adt.OpContext, r *runtime.Runtime, value adt.Expr) (string, adt.Kind) {
	t.Helper()
	root := &adt.Environment{Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	constraint, complete := ctx.Evaluate(root, value)
	if !complete || constraint == nil {
		t.Fatal("incomplete native checking type")
	}
	constraint = adt.Unwrap(constraint)
	if basic, ok := constraint.(*adt.BasicType); ok {
		return builtinKindSyntax(basic.K), basic.K
	}
	if union, ok := constraint.(*adt.Disjunction); ok {
		copy := *union
		copy.NumDefaults, copy.HasDefaults = 0, false
		constraint = &copy
	}
	expr, err := export.All.Value(r, "", constraint)
	if err != nil {
		t.Fatal(err)
	}
	text, formatErr := format.Node(expr)
	if formatErr != nil {
		t.Fatal(formatErr)
	}
	return string(text), constraint.Kind()
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
