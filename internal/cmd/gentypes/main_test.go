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

package main

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/astinternal"
	"cuelang.org/go/pkg"
)

const reference = "../../../doc/types.md"

func TestReferenceCurrent(t *testing.T) {
	if err := run(reference, true); err != nil {
		t.Fatal(err)
	}
}

// Regrouping declarations into signatures and supporting schemas must retain
// every contract, constant, attribute on a parameter, and default exactly.
func TestReferencePackageSyntax(t *testing.T) {
	source, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	_, catalogue, _ := strings.Cut(string(source), startMarker)
	catalogue, _, _ = strings.Cut(catalogue, endMarker)
	sections := strings.Split(catalogue, "\n### `")[1:]
	paths := pkg.ImportPaths()
	if len(sections) != len(paths) {
		t.Fatalf("got %d package sections; want %d", len(sections), len(paths))
	}
	config := astinternal.DebugConfig{
		Filter:    func(v reflect.Value) bool { return v.Type() != reflect.TypeFor[token.Pos]() },
		OmitEmpty: true,
	}
	shape := func(file *ast.File) []string {
		var result []string
		for _, decl := range file.Decls {
			switch decl.(type) {
			case *ast.Package, *ast.Attribute:
				continue
			}
			result = append(result, string(astinternal.AppendDebug(nil, decl, config)))
		}
		slices.Sort(result)
		return result
	}
	for i, ip := range paths {
		t.Run(ip, func(t *testing.T) {
			if !strings.HasPrefix(sections[i], ip+"`\n") {
				t.Fatalf("missing package heading for %q", ip)
			}
			var code strings.Builder
			for _, block := range strings.Split(sections[i], "```cue\n")[1:] {
				text, _, _ := strings.Cut(block, "```")
				code.WriteString(text)
				code.WriteByte('\n')
			}
			file, err := parser.ParseFile(ip+"/pkg.cue", code.String())
			if err != nil {
				t.Fatal(err)
			}
			original, _ := pkg.Source(ip)
			want, err := parser.ParseFile(ip+"/pkg.cue", original)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(shape(file), shape(want)) {
				t.Fatal("reference formatting changed the package's declarations")
			}
			if err := cuecontext.New().BuildFile(file).Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Check the written builtin contracts against the actual builtin values, and
// each operator arrow by checking an implementation containing that operator.
// These are signature checks, not unrelated examples of how to write CUE.
func TestReferenceContracts(t *testing.T) {
	source, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	operatorBodies := map[string]map[string]string{
		"binary": {
			"&": "x & y", "|": "x | y", "+": "x + y", "-": "x - y",
			"*": "x * y", "/": "x / y", "==": "x == y", "!=": "x != y",
			"<": "x < y", "<=": "x <= y", ">": "x > y", ">=": "x >= y",
			"=~": "x =~ y", "!~": "x !~ y", "&&": "x && y", "||": "x || y",
		},
		"unary": {
			"+": "+x", "-": "-x", "!": "!x", "<": "<x", "<=": "<=x",
			">": ">x", ">=": ">=x", "!=": "!=x", "=~": "=~x", "!~": "!~x",
		},
		"selection": {
			"xs[i]": "x[y]", "tuple[i]": "x[y]", "x.field": "x.field",
			"x[\"field\"]": "x[y]", "xs[lo:hi]": "x[y:z]", "b[lo:hi]": "x[y:z]",
			"string interpolation": `"\(x)"`, "bytes interpolation": `'\(x)'`,
		},
	}
	for _, section := range []string{"builtin", "binary", "unary", "selection"} {
		t.Run(section, func(t *testing.T) {
			marker := "<!-- contracts: " + section + " -->\n```cue\n"
			_, rest, ok := strings.Cut(string(source), marker)
			if !ok {
				t.Fatalf("missing %s contracts", section)
			}
			code, _, _ := strings.Cut(rest, "```")
			file, err := parser.ParseFile("types.cue", code)
			if err != nil {
				t.Fatal(err)
			}
			fields := make(map[string]ast.Expr)
			for _, decl := range file.Decls {
				field, ok := decl.(*ast.Field)
				if !ok {
					t.Fatalf("unexpected declaration %T", decl)
				}
				name, _, err := ast.LabelName(field.Label)
				if err != nil {
					t.Fatal(err)
				}
				fields[name] = field.Value
			}
			count := 0
			for name, typ := range fields {
				if strings.HasPrefix(name, "#") {
					continue
				}
				count++
				t.Run(name, func(t *testing.T) {
					var implementations []string
					if section == "builtin" && name != "error" {
						implementations = []string{"__" + name + " & " + name}
					} else {
						body, ok := operatorBodies[section][name]
						if section == "builtin" && name == "error" {
							// error is a special call form; its value does not
							// support direct attachment of an arrow interface.
							body, ok = "__error(x)", true
						}
						if !ok {
							t.Fatalf("no implementation for %q", name)
						}
						for _, impl := range implementContract(t, typ, fields, body) {
							text, err := format.Node(impl)
							if err != nil {
								t.Fatal(err)
							}
							implementations = append(implementations, string(text))
						}
					}
					for _, impl := range implementations {
						v := cuecontext.New().CompileString(code + "\nchecked: " + impl)
						checked := v.LookupPath(cue.ParsePath("checked"))
						if err := checked.Validate(); err != nil {
							t.Fatalf("%s\n%v", impl, err)
						}
					}
				})
			}
			if count == 0 || section != "builtin" && count != len(operatorBodies[section]) {
				t.Fatalf("incomplete contract listing: %d entries", count)
			}
		})
	}
}

func implementContract(t *testing.T, typ ast.Expr, fields map[string]ast.Expr, body string) []ast.Expr {
	t.Helper()
	switch x := typ.(type) {
	case *ast.ParenExpr:
		return implementContract(t, x.X, fields, body)
	case *ast.Ident:
		definition, ok := fields[x.Name]
		if !ok {
			t.Fatalf("unknown contract alias %s", x.Name)
		}
		return implementContract(t, definition, fields, body)
	case *ast.BinaryExpr:
		if x.Op != token.AND {
			t.Fatalf("unexpected contract operator %v", x.Op)
		}
		return append(implementContract(t, x.X, fields, body), implementContract(t, x.Y, fields, body)...)
	case *ast.Quantifier:
		var result []ast.Expr
		for _, impl := range implementContract(t, x.Body, fields, body) {
			q := *x
			q.Body = impl
			result = append(result, &q)
		}
		return result
	case *ast.Func:
		f := *x
		f.Params = nil
		for i, param := range x.Parameters() {
			p := *param
			p.Label = ast.NewIdent(string(rune('x' + i)))
			f.Params = append(f.Params, &p)
		}
		expr, err := parser.ParseExpr("body.cue", body)
		if err != nil {
			t.Fatal(err)
		}
		f.Body = expr
		return []ast.Expr{&f}
	default:
		t.Fatalf("unexpected contract type %T", typ)
		return nil
	}
}
