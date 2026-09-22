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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/parser"
)

// Keep every listing accounted for, including explicit D exclusions and
// grammar metavariables. Source equality makes changes to the paper demand
// corresponding review of the executable regression corpus.
func TestQuantifiedPaper(t *testing.T) {
	paper, err := os.ReadFile("../doc/quantified-cue.tex")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/quantified-paper.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID                                                        int
		Title, Source, Setup, Extra, Mode, Reason, Exclude, Error string
		ValidWithout                                              string `json:"valid_without"`
		Checks                                                    map[string]string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)\\begin\{lstlisting\}(?:\[[^\n]*\])?\n(.*?)\\end\{lstlisting\}`)
	listings := re.FindAllSubmatch(paper, -1)
	if len(listings) != len(cases) {
		t.Fatalf("paper has %d listings; corpus accounts for %d", len(listings), len(cases))
	}
	for i, tt := range cases {
		t.Run(fmt.Sprintf("%03d", tt.ID), func(t *testing.T) {
			if tt.ID != i+1 || tt.Source != string(listings[i][1]) {
				t.Fatal("paper listing changed; update and review its regression case")
			}
			if tt.Exclude != "" {
				t.Skip(tt.Exclude)
			}
			src := "@experiment(quantified)\n" + tt.Setup + tt.Source + "\n" + tt.Extra
			if tt.Mode == "syntax" {
				if _, err := parser.ParseFile("paper.cue", src); err != nil {
					t.Fatal(err)
				}
				return
			}
			ctx := cuecontext.New()
			v := ctx.CompileString(src, cue.Filename("paper.cue"))
			if tt.Mode == "compile-error" {
				if err := v.Err(); err == nil || !strings.Contains(err.Error(), tt.Error) {
					t.Fatalf("got %v; want compile error containing %q", err, tt.Error)
				}
				var valid []string
				for _, line := range strings.Split(src, "\n") {
					if !strings.HasPrefix(strings.TrimSpace(line), tt.ValidWithout) {
						valid = append(valid, line)
					}
				}
				v = ctx.CompileString(strings.Join(valid, "\n"))
			}
			if len(tt.Checks) == 0 {
				t.Fatal("executable example has no assertions")
			}
			for path, want := range tt.Checks {
				x := v.LookupPath(cue.ParsePath(path))
				if !x.Exists() {
					t.Errorf("%s missing (root: %v)", path, v.Err())
					continue
				}
				switch {
				case strings.HasPrefix(want, "json:"):
					got, err := x.MarshalJSON()
					if err != nil || string(got) != strings.TrimPrefix(want, "json:") {
						t.Errorf("%s: got %s, %s; want %s", path, got, errors.Details(err, nil), want)
					}
				case want == "conflict":
					if err := x.Validate(); err == nil {
						t.Errorf("%s: missing definite conflict", path)
					}
				case want == "incomplete":
					if err := x.Validate(); err != nil {
						t.Errorf("%s: incomplete obligation was refuted: %v", path, err)
					}
					if err := x.Validate(cue.Concrete(true)); err == nil {
						t.Errorf("%s: unresolved obligation was reported complete", path)
					}
				case want == "valid":
					if err := x.Validate(); err != nil {
						t.Errorf("%s: %v", path, err)
					}
				default:
					t.Fatalf("unknown assertion %q", want)
				}
			}
		})
	}
}
