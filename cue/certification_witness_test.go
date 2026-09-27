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

func TestQuantifiedSingletonCertification(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"record", `w:{x:1}
f:func()->w:{x:1}`, true},
		{"nested", `w:{x:{y:1}}
f:func()->w:{x:{y:1}}`, true},
		{"list", `w:[1,2]
f:func()->w:[1,2]`, true},
		{"nested_list", `w:[{x:1}]
f:func()->w:[{x:1}]`, true},
		{"hidden", `w:{x:1,_secret:2}
f:func()->w:{x:1,_secret:2}`, true},
		{"definition", `w:{x:1,#D:int}
f:func()->w:{x:1}`, true},
		{"extra", `w:{x:1}
f:func()->w:{x:1,y:2}`, false},
		{"missing", `w:{x:1,y:2}
f:func()->w:{x:1}`, false},
		{"wrong_value", `w:{x:1}
f:func()->w:{x:2}`, false},
		{"wrong_hidden", `w:{x:1,_secret:2}
f:func()->w:{x:1,_secret:3}`, false},
		{"extra_hidden", `w:{x:1}
f:func()->w:{x:1,_secret:2}`, false},
		{"unknown_witness", `w:{x:int}
f:func()->w:{x:1}`, false},
		{"open_witness_list", `w:[1,...int]
f:func()->w:[1]`, false},
		{"nested_open_witness_list", `w:{xs:[1,...int]}
f:func()->w:{xs:[1]}`, false},
		{"unknown_body", `w:{x:1}
f:func(x:int)->w:{x:x}`, false},
		{"open_parameter", `w:{x:1}
f:func(r:{x:1})->w:r`, false},
		{"open_nested_parameter", `w:{x:{y:1}}
f:func(r:{y:1})->w:{x:r}`, false},
		{"open_list_parameter", `w:[1]
f:func(xs:[...1])->w:xs`, false},
		{"open_element_parameter", `w:[{x:1}]
f:func(r:{x:1})->w:[r]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid test source: %v", v.Err())
			}
			if err := f.Validate(); (err == nil) != tt.valid {
				t.Fatalf("singleton result: valid=%v: %v", tt.valid, err)
			}
		})
	}
}
