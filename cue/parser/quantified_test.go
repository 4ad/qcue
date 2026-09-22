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

package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/internal/astinternal"
)

func TestQuantifiedSyntax(t *testing.T) {
	files, err := filepath.Glob("testdata/quantified/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no quantified syntax fixtures")
	}
	for _, filename := range files {
		t.Run(strings.TrimSuffix(filepath.Base(filename), ".txtar"), func(t *testing.T) {
			a, err := txtar.ParseFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			contents := make(map[string]string)
			for _, f := range a.Files {
				contents[f.Name] = string(f.Data)
			}
			src, ok := contents["in.cue"]
			if !ok {
				t.Fatal("missing in.cue")
			}
			f, err := ParseFile("in.cue", src, ParseComments)
			if strings.Contains(string(a.Comment), "#error") {
				if err == nil {
					t.Fatal("accepted malformed or disabled quantified syntax")
				}
				if want := strings.TrimSpace(contents["error.txt"]); want != "" && !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v; want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want, ok := contents["debug.txt"]; ok {
				if got := astinternal.DebugStr(f.Decls[0]); got != strings.TrimSpace(want) {
					t.Fatalf("AST = %s; want %s", got, want)
				}
			}
			ast.Walk(f, func(ast.Node) bool { return true }, nil)
		})
	}
}
