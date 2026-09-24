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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
)

// Semantic assertions live beside their CUE programs in the ordinary txtar
// corpus. This small index test ensures no paper listing silently disappears,
// including D exclusions, syntax templates, and deliberate formation errors.
func TestQuantifiedPaperIndex(t *testing.T) {
	paper, err := os.ReadFile("../doc/paper.tex")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)\\begin\{lstlisting\}(?:\[[^\n]*\])?\n(.*?)\\end\{lstlisting\}`)
	listings := re.FindAllSubmatch(paper, -1)
	if len(listings) == 0 {
		t.Fatal("no paper listings found")
	}
	files, err := filepath.Glob("testdata/quantified/paper/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(listings) {
		t.Fatalf("%d archives for %d paper listings", len(files), len(listings))
	}
	for i, filename := range files {
		t.Run(filepath.Base(filename), func(t *testing.T) {
			a, err := txtar.ParseFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			contents := make(map[string]string)
			for _, f := range a.Files {
				if _, exists := contents[f.Name]; exists {
					t.Fatalf("duplicate archive section %s", f.Name)
				}
				contents[f.Name] = string(f.Data)
			}
			tags := make(map[string]string)
			for _, line := range strings.Split(string(a.Comment), "\n") {
				if k, v, ok := strings.Cut(line, ": "); ok {
					tags[k] = v
				}
			}
			if tags["#paper"] != fmt.Sprint(i+1) || contents["paper.cue.txt"] != string(listings[i][1]) {
				t.Fatal("paper listing changed; update and review its txtar case")
			}
			if !strings.Contains(string(a.Comment), "\n#noformat\n") {
				t.Fatal("paper archives must preserve verbatim source with #noformat")
			}
			switch tags["#mode"] {
			case "evaluation", "pseudocode":
			case "syntax":
				if _, err := parser.ParseFile("paper.cue", contents["paper.cue.txt"]); err != nil {
					t.Fatal(err)
				}
			case "compile-error":
				v := cuecontext.New().CompileString(contents["invalid.cue.txt"])
				if err := v.Err(); err == nil || !strings.Contains(err.Error(), tags["#error"]) {
					t.Fatalf("got %v; want %s", err, tags["#error"])
				}
			default:
				t.Fatalf("unknown paper example mode %q", tags["#mode"])
			}
			if !strings.Contains(string(a.Comment), "#skip\n") && !strings.Contains(contents["in.cue"], "@test(") {
				t.Fatal("executable paper listing has no inline assertions")
			}
		})
	}
}
