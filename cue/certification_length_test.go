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

func TestQuantifiedLengthCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"record_minimum", `f:func(r:{a:int})->(int&>=1):len(r)`, true},
		{"record_required", `f:func(r:{a!:int})->(int&>=1):len(r)`, true},
		{"record_closed", `f:func(r:close({a:int}))->1:len(r)`, true},
		{"record_optional", `f:func(r:close({a:int,b?:int}))->(int&>=1&<=2):len(r)`, true},
		{"fixed_list", `f:func(xs:[int,string])->2:len(xs)`, true},
		{"list_minimum", `f:func(xs:[int,...int])->(int&>=1):len(xs)`, true},
		{"string", `f:func()->2:len("é")`, true},
		{"bytes", `f:func()->2:len('ab')`, true},
		{"open_not_exact", `f:func(r:{a:int})->1:len(r)`, false},
		{"optional_not_present", `f:func(r:{a?:int})->(int&>=1):len(r)`, false},
		{"hidden_not_counted", `f:func(r:{_a:int})->(int&>=1):len(r)`, false},
		{"definition_not_counted", `f:func(r:{#A:int})->(int&>=1):len(r)`, false},
		{"pattern_not_finite", `f:func(r:close({[string]:int}))->0:len(r)`, false},
		{"list_not_exact", `f:func(xs:[int,...int])->1:len(xs)`, false},
		{"wrong_packet", `f:func()->1:len(wrong:"x")`, false},
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
