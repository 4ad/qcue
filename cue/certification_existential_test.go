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
)

func TestQuantifiedExistentialDataCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"literal", `f:func()->(exists A {x:A}):{x:1}`, true},
		{"parameter", `f:func(n:int)->(exists A {x:A}):{x:n}`, true},
		{"bound", `f:func(n:int)->(exists (A:number) {x:A}):{x:n}`, true},
		{"repeated", `f:func(n:int,s:string)->(exists A {x:A,y:A}):{x:n,y:s}`, true},
		{"nested", `f:func(n:int)->(exists A {x:[A]}):{x:[n]}`, true},
		{"optional", `f:func()->(exists A {x:A,extra?:1}):{x:1}`, true},
		{"required_missing", `f:func()->(exists A {x:A,y!:int}):{x:1}`, false},
		{"wrong_bound", `f:func(n:string)->(exists (A:number) {x:A}):{x:n}`, false},
		{"wrong_optional", `f:func()->(exists A {x:A,extra?:1}):{x:1,extra:2}`, false},
		{"wrong_constant", `f:func()->(exists A {x:A,tag:true}):{x:1,tag:false}`, false},
		{"wrong_nested", `f:func(n:int)->(exists A {x:[A,A]}):{x:[n]}`, false},
		{"bad_operation", `f:func(r:{})->(exists A {x:A}):{x:r.missing}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := semanticValue(t, tt.source)
			f := root.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid source: %v", root.Err())
			}
			if err := f.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("certificate: valid=%v: %v", tt.valid, err)
			}
		})
	}
}
