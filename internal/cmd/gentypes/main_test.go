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
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
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

// Check the rendered code as well as freshness: layout changes must preserve
// every declaration, parameter, schema attribute, and default in the API.
func TestReferencePackageSyntax(t *testing.T) {
	source, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	_, catalogue, _ := strings.Cut(string(source), startMarker)
	catalogue, _, _ = strings.Cut(catalogue, endMarker)
	blocks := strings.Split(catalogue, "```cue\n")[1:]
	paths := pkg.ImportPaths()
	if len(blocks) != len(paths) {
		t.Fatalf("got %d package blocks; want %d", len(blocks), len(paths))
	}
	config := astinternal.DebugConfig{
		Filter:    func(v reflect.Value) bool { return v.Type() != reflect.TypeFor[token.Pos]() },
		OmitEmpty: true,
	}
	for i, ip := range paths {
		t.Run(ip, func(t *testing.T) {
			code, _, _ := strings.Cut(blocks[i], "```")
			file, err := parser.ParseFile(ip+"/pkg.cue", code)
			if err != nil {
				t.Fatal(err)
			}
			original, _ := pkg.Source(ip)
			want, err := parser.ParseFile(ip+"/pkg.cue", original)
			if err != nil {
				t.Fatal(err)
			}
			gotAST := astinternal.AppendDebug(nil, file, config)
			wantAST := astinternal.AppendDebug(nil, want, config)
			if !bytes.Equal(gotAST, wantAST) {
				t.Fatal("reference formatting changed the package's syntax tree")
			}
			if err := cuecontext.New().BuildFile(file).Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Every CUE example before the generated catalogue is executable. Invalid
// examples use a "cue invalid" fence. Result comments assert JSON observations
// without requiring function values and schemas to be exportable as JSON.
func TestReferenceExamples(t *testing.T) {
	source, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	guide, _, ok := strings.Cut(string(source), startMarker)
	if !ok {
		t.Fatal("missing catalogue marker")
	}
	blocks := strings.Split(guide, "```")
	count := 0
	for i := 1; i < len(blocks); i += 2 {
		language, code, _ := strings.Cut(blocks[i], "\n")
		if language != "cue" && language != "cue invalid" {
			continue
		}
		count++
		t.Run(strings.SplitN(code, "\n", 2)[0], func(t *testing.T) {
			v := cuecontext.New().CompileString(code)
			err := v.Validate()
			if language == "cue invalid" {
				if err == nil {
					t.Fatal("invalid example passed validation")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(code, "\n") {
				observation, ok := strings.CutPrefix(line, "// Result: ")
				if !ok {
					continue
				}
				path, want, ok := strings.Cut(observation, " = ")
				if !ok {
					t.Fatalf("invalid result comment: %s", line)
				}
				got, err := v.LookupPath(cue.ParsePath(path)).MarshalJSON()
				if err != nil {
					t.Fatal(err)
				}
				var compact bytes.Buffer
				if err := json.Compact(&compact, []byte(want)); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, compact.Bytes()) {
					t.Errorf("%s: got %s; want %s", path, got, compact.Bytes())
				}
			}
		})
	}
	if count == 0 {
		t.Fatal("no executable examples in the reference")
	}
}
