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

func TestQuantifiedSealCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"captured", `#I:exists A {f:func(int)->int}
f:func(n:int)->#I:seal #I with (A=int) {f:func(x:int)->int:x+n}`, true},
		{"abstract", `#I:exists A {value:A,read:func(A)->int}
f:func(n:int)->#I:seal #I with (A=int) {value:n,read:func(x:int)->int:x}`, true},
		{"unused", `#I:exists A {value:int}
f:func(n:int)->#I:seal #I with (A=string) {value:n}`, true},
		{"multiple", `#I:exists (A,B) {a:A,b:B}
f:func(n:int)->#I:seal #I with (B=string,A=int) {a:n,b:"s"}`, true},
		{"wrong_representation", `#I:exists A {value:A}
f:func(n:int)->#I:seal #I with (A=string) {value:n}`, false},
		{"missing_field", `#I:exists A {value:A}
f:func(n:int)->#I:seal #I with (A=int) {other:n}`, false},
		{"wrong_body", `#I:exists A {f:func(int)->int}
f:func(n:int)->#I:seal #I with (A=int) {f:func(x:int)->int:"wrong"}`, false},
		{"wrong_domain", `#I:exists A {f:func(number)->int}
f:func(n:int)->#I:seal #I with (A=int) {f:func(x:int)->int:x}`, false},
		{"wrong_witness_label", `#I:exists A {value:A}
f:func(n:int)->#I:seal #I with (B=int) {value:n}`, false},
		{"ambiguous_transport", `#I:exists A {f:func()->(A|string)}
f:func(n:int)->#I:seal #I with (A=string) {f:func()->string:"s"}`, false},
		{"interface_refinement", `#I:exists A {value:A}
f:func(n:int)->#I:seal (#I & {tag:1}) with (A=int) {value:n,tag:2}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source).LookupPath(cue.ParsePath("f"))
			if !v.Exists() {
				t.Fatalf("invalid test source: %v", v.Err())
			}
			if err := v.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
		})
	}
}
