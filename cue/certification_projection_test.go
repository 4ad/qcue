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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestQuantifiedBoundedProjectionCertification(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"record", `f(A,B:{x:A}):func(v:B)->A:v.x`, true},
		{"list", `f(A,B:[A]):func(v:B)->A:v[0]`, true},
		{"witness", "r:{x:int}\nf:func(v:r)->int:v.x", true},
		{"unknown_field", `f(A,B:{x:A}):func(v:B)->A:v.y`, false},
		{"optional_field", `f(A,B:{x?:A}):func(v:B)->A:v.x`, false},
		{"wrong_result", `f(A,B:{x:A}):func(v:B)->string:v.x`, false},
		{"lost_subtype", `f(A,B:{x:A}):func(v:B)->B:{x:v.x}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid test source: %v", v.Err())
			}
			if err := f.Validate(); (err == nil) != tt.valid {
				t.Fatalf("projection: valid=%v: %v", tt.valid, err)
			}
		})
	}
}
