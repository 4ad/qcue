// Copyright 2026 CUE Authors
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

package cuetxtar

import (
	"bytes"
	"strings"
	"testing"

	"cuelang.org/go/cue"
)

// runValueAssertion exercises the public validation and observation APIs.
// These requests differ from equality of displayed constraints: a symbolic
// call may have a known result constraint without a materialized result.
func (r *inlineRunner) runValueAssertion(t testing.TB, path cue.Path, val cue.Value, pa parsedTestAttr) {
	t.Helper()
	var args []string
	for _, kv := range pa.raw.Fields[1:] {
		switch kv.Key() {
		case "at":
			p, err := parseAtPath(kv.Value())
			if err != nil {
				t.Fatal(err)
			}
			val = val.LookupPath(p)
			path = cue.MakePath(append(path.Selectors(), p.Selectors()...)...)
		case "":
			args = append(args, strings.TrimSpace(kv.Text()))
		case "hint", "p":
		default:
			t.Fatalf("unknown @test(%s) option %s", pa.directive, kv.Key())
		}
	}
	if !val.Exists() {
		t.Fatalf("path %s: value does not exist", path)
	}
	switch pa.directive {
	case "json":
		if len(args) != 1 {
			t.Fatal("@test(json) requires one expected JSON value")
		}
		want, err := val.Context().CompileString(args[0]).MarshalJSON()
		if err != nil {
			t.Fatalf("invalid expected JSON: %v", err)
		}
		got, err := val.MarshalJSON()
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("path %s: JSON = %s, %v; want %s", path, got, err, want)
		}
	case "validate":
		var opts []cue.Option
		outcome := "success"
		for _, arg := range args {
			switch arg {
			case "concrete":
				opts = append(opts, cue.Concrete(true))
			case "functions":
				opts = append(opts, cue.VerifyFunctions(true))
			case "incomplete", "conflict":
				if outcome != "success" {
					t.Fatal("duplicate validation outcome")
				}
				outcome = arg
			default:
				t.Fatalf("unknown @test(validate) option %q", arg)
			}
		}
		base := val.Validate()
		err := val.Validate(opts...)
		switch outcome {
		case "success":
			if err != nil {
				t.Errorf("path %s: validation failed: %v", path, err)
			}
		case "conflict":
			if base == nil {
				t.Errorf("path %s: expected a definite conflict", path)
			}
		case "incomplete":
			if base != nil || err == nil {
				t.Errorf("path %s: expected incompleteness; ordinary validation: %v; demanded validation: %v", path, base, err)
			}
		}
	case "subsume":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "fail") {
			t.Fatal("@test(subsume) requires two paths and an optional fail flag")
		}
		a, b := val.LookupPath(cue.ParsePath(args[0])), val.LookupPath(cue.ParsePath(args[1]))
		if !a.Exists() || !b.Exists() {
			t.Fatal("subsumption path does not exist")
		}
		err := a.Subsume(b)
		if (err == nil) != (len(args) == 2) {
			t.Errorf("subsumption %s >= %s: %v", args[0], args[1], err)
		}
	}
}
