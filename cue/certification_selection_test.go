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

func TestQuantifiedSelectedImplementationScope(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"independent_universal", `
f(A): func(x:A)->A:x
f: forall B func(B)->B
selected: f[int]
`, true},
		{"bounded_universal", `
f(A:number): func(x:A)->A:x
f: forall (B:int) func(B)->B
selected: f[int]
`, true},
		{"bad_original_body", `
f(A): func(x:A)->A:({value:0}).value
f: forall B func(B)->B
selected: f[int]
`, false},
		{"bad_additional_contract", `
f(A): func(x:A)->A:x
f: forall B func(B)->int
selected: f[int]
`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := cuecontext.New().CompileString(tt.source)
			selected := root.LookupPath(cue.ParsePath("selected"))
			if !selected.Exists() {
				t.Fatalf("invalid test source: %v", root.Err())
			}
			if err := selected.Validate(); (err == nil) != tt.valid {
				t.Fatalf("selected implementation: valid=%v: %v", tt.valid, err)
			}
		})
	}
}
