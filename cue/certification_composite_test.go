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

func TestQuantifiedCompositeIntroductionCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source, call, want string
		valid                    bool
	}{
		{"record", `f:func(n:int)->_:forall A {get:func(x:A)->int:n}`, `f(7)[int].get(1)`, "7", true},
		{"identity", `f:func()->_:forall A {id:func(x:A)->A:x}`, `f()[string].id("s")`, `"s"`, true},
		{"list", `f:func(n:int)->_:forall A [func(x:A)->int:n]`, `f(7)[int][0](1)`, "7", true},
		{"bad_identity", `f:func()->_:forall A {id:func(x:A)->A:1}`, "", "", false},
		{"bad_capture", `f:func(n:string)->_:forall A {get:func(x:A)->int:n}`, "", "", false},
		{"bad_list", `f:func()->_:forall A [func(x:A)->A:1]`, "", "", false},
		{"bad_nested", `f:func()->_:forall A {r:{get:func(x:A)->int:x}}`, "", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := tt.source
			if tt.valid {
				source += "\nout:" + tt.call
			}
			root := semanticValue(t, source)
			f := root.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid source: %v", root.Err())
			}
			if err := f.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("certificate: valid=%v: %v", tt.valid, err)
			}
			if tt.valid {
				semanticJSON(t, root, "out", tt.want)
			}
		})
	}
}
