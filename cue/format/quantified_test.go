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

package format_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/astinternal"
	"cuelang.org/go/internal/cueexperiment"
)

func TestQuantifiedRoundTrip(t *testing.T) {
	if err := cueexperiment.Init(); err != nil {
		t.Fatal(err)
	}
	defer func(v bool) { cueexperiment.Flags.FormatV2 = v }(cueexperiment.Flags.FormatV2)
	for _, mode := range []struct {
		name           string
		v2             bool
		clearPositions bool
	}{
		{name: "v1"},
		{name: "v1_generated", clearPositions: true},
		{name: "v2", v2: true},
		{name: "v2_generated", v2: true, clearPositions: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			cueexperiment.Flags.FormatV2 = mode.v2
			a, err := txtar.ParseFile("testdata/quantified.txtar")
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range a.Files {
				if !strings.HasSuffix(file.Name, ".input") {
					continue
				}

				t.Run(file.Name, func(t *testing.T) {
					in := string(file.Data)
					f, err := parser.ParseFile("test.cue", in, parser.ParseComments)
					if err != nil {
						t.Fatal(err)
					}
					var out []byte
					var opts []format.Option
					if mode.clearPositions {
						// Cloning and clearing layout hints must retain the
						// declaration's authored shorthand spelling.
						f = ast.Clone(f)
						format.ASTStyle{ClearPositions: true}.Apply(f)
						opts = []format.Option{format.LineWidth(40)}
						out, err = format.Node(f, opts...)
					} else {
						out, err = format.Source([]byte(in))
					}
					if err != nil {
						t.Fatal(err)
					}
					g, err := parser.ParseFile("test.cue", out, parser.ParseComments)
					if err != nil {
						t.Fatalf("%v\n%s", err, out)
					}
					if a, b := quantifiedComments(f), quantifiedComments(g); !slices.Equal(a, b) {
						t.Fatalf("comments changed: %q => %q\n%s", a, b, out)
					}
					if a, b := quantifiedShorthandFields(f), quantifiedShorthandFields(g); !slices.Equal(a, b) {
						t.Fatalf("shorthand declarations changed: %q => %q\n%s", a, b, out)
					}
					// Comments may move to a different node when binders are
					// normalized, but the expression tree must stay the same.
					for _, node := range []ast.Node{f, g} {
						ast.Walk(node, func(n ast.Node) bool {
							ast.SetComments(n, nil)
							return true
						}, nil)
					}
					if a, b := astinternal.DebugStr(f), astinternal.DebugStr(g); a != b {
						t.Fatalf("syntax changed: %s => %s\n%s", a, b, out)
					}
					again, err := format.Source(out, opts...)
					if err != nil {
						t.Fatal(err)
					}
					if string(out) != string(again) {
						t.Fatalf("unstable format:\n%s\n%s", out, again)
					}
				})
			}
		})
	}
}

func quantifiedShorthandFields(n ast.Node) []string {
	var names []string
	ast.Walk(n, func(n ast.Node) bool {
		if f, ok := n.(*ast.Field); ok {
			if q, ok := f.Value.(*ast.Quantifier); ok && q.Shorthand {
				names = append(names, astinternal.DebugStr(f.Label))
			}
		}
		return true
	}, nil)
	return names
}

// A quantifier can be copied out of its original field by an AST consumer.
// Its shorthand hint must not produce invalid syntax in the new context.
func TestQuantifiedShorthandContext(t *testing.T) {
	if err := cueexperiment.Init(); err != nil {
		t.Fatal(err)
	}
	defer func(v bool) { cueexperiment.Flags.FormatV2 = v }(cueexperiment.Flags.FormatV2)
	for _, v2 := range []bool{false, true} {
		cueexperiment.Flags.FormatV2 = v2
		f, err := parser.ParseFile("test.cue", "id(A): [...A]")
		if err != nil {
			t.Fatal(err)
		}
		q := f.Decls[0].(*ast.Field).Value.(*ast.Quantifier)
		record := ast.Clone(q)
		record.Body = ast.NewStruct(ast.NewIdent("value"), ast.NewIdent("A"))
		for _, tc := range []struct {
			name string
			node ast.Node
			want string
		}{
			{"expression", q, "forall (A) [...A]"},
			{"field", &ast.Field{Label: ast.NewIdent("copy"), Value: q}, "copy(A): [...A]"},
			{"quoted", &ast.Field{Label: ast.NewString("copy"), Value: q}, `"copy": forall (A) [...A]`},
			{"optional", &ast.Field{Label: ast.NewIdent("copy"), Constraint: token.OPTION, Value: q}, "copy?: forall (A) [...A]"},
			{"record", &ast.Field{Label: ast.NewIdent("copy"), Value: record}, "copy(A): {value: A}"},
		} {
			t.Run(fmt.Sprintf("v2=%t/%s", v2, tc.name), func(t *testing.T) {
				out, err := format.Node(tc.node)
				if err != nil {
					t.Fatal(err)
				}
				// The formatters lay out generated records differently.
				if got := strings.Join(strings.Fields(string(out)), ""); got != strings.Join(strings.Fields(tc.want), "") {
					t.Fatalf("got %q; want %q (ignoring whitespace)", out, tc.want)
				}
				if tc.name == "expression" {
					_, err = parser.ParseExpr("", out)
				} else {
					_, err = parser.ParseFile("", out)
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func quantifiedComments(n ast.Node) []string {
	var comments []string
	ast.Walk(n, func(n ast.Node) bool {
		if c, ok := n.(*ast.Comment); ok {
			comments = append(comments, c.Text)
		}
		return true
	}, nil)
	slices.Sort(comments)
	return comments
}
