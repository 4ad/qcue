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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// A required capture has a usable description before it has a supplied value.
// Lexical references and selectors must agree, and proving the body must not
// make the closure concrete or discharge the call's runtime capture demand.
func TestQuantifiedRequiredCaptures(t *testing.T) {
	for _, tt := range []struct{ name, fields, body, refinement string }{
		{"lexical", "z!:int", "x+y+z", "z:10"},
		{"selector", "config:{z!:int}", "x+y+config.z", "config:z:10"},
		{"transitive", "z!:int\nlet n=z", "x+y+n", "z:10"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			source := "_f:{" + tt.fields + "\nf:func(x:int,y:int)->int:" + tt.body + "}\n"
			v := ctx.CompileString(source)
			if err := v.Validate(); err != nil {
				t.Fatalf("prototype: %v", err)
			}
			f := v.LookupPath(cue.MakePath(cue.Hid("_f", "_"), cue.Str("f")))
			if err := f.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("unsupplied capture made a concrete closure")
			}
			missing := ctx.CompileString(source + "bar:_f.f(100,1000)")
			if _, err := missing.LookupPath(cue.ParsePath("bar")).Int64(); err == nil {
				t.Fatal("call supplied its own missing capture")
			}
			v = ctx.CompileString(source + "bar:(_f & {" + tt.refinement + "}).f(100,1000)")
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "bar", "1110")
		})
	}
}

func TestQuantifiedCaptureEvidenceDoesNotInventPremises(t *testing.T) {
	for _, source := range []string{
		`z?:int; f:func()->int:z`,
		`z!:string; f:func()->int:z`,
		`z!:int; f:func()->string:z`,
		`z!:int; z!:string; f:func()->int:z`,
		`z!:int; z!:1+"s"; f:func()->int:z`,
		`z!:_; f:func()->int:z`,
	} {
		v := cuecontext.New().CompileString(strings.ReplaceAll(source, ";", "\n"))
		if !v.LookupPath(cue.ParsePath("f")).Exists() {
			t.Fatalf("invalid test source: %v", v.Err())
		}
		if err := v.Validate(); err == nil {
			t.Fatalf("unjustified capture evidence: %s", source)
		}
	}
}
