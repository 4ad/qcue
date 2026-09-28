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

func TestQuantifiedInterfaceRelevance(t *testing.T) {
	for _, tt := range []struct {
		name    string
		source  string
		blocked bool
	}{
		{"same_domain", `f: (func(int)->int) & (func(int)->bool)`, true},
		{"separate_declarations", "f: func(int)->int\nf: func(int)->bool", true},
		{"empty_result", `f: func(int)->_|_`, true},
		{"empty_domain", `f: func(int & string)->_|_`, false},
		{"overlap", `f: (func(number)->int) & (func(int)->bool)`, true},
		{"compatible", `f: (func(number)->number) & (func(int)->int)`, false},
		{"disjoint", `f: (func(int)->string) & (func(string)->int)`, false},
		{"triple", `f: (func(int)->(0|1)) & (func(int)->(1|2)) & (func(int)->(0|2))`, true},
		{"union", `f: (func(int)->int) | (func(int)->bool)`, false},
		{"union_invalid", `f: (func(int)->int) | (func(int)->_|_)`, true},
		{"union_default", `f: *(func(int)->int) | (func(int)->_|_)`, true},
		{"callback", `f: func((func(int)->int) & (func(int)->bool))->string`, true},
		{"result", `f: func(int)->((func(int)->int) & (func(int)->bool))`, true},
		{"record", `f: func({cb: func(int)->_|_})->string`, true},
		{"record_meet", `f: func({cb: func(int)->int} & {cb: func(int)->bool})->string`, true},
		{"optional", `f: func({cb?: func(int)->_|_})->string`, true},
		{"absent_optional", `f: func({cb?: (func(int)->_|_) & int})->string`, false},
		{"list", `f: func([func(int)->_|_])->string`, true},
		{"list_tail", `f: func([...func(int)->_|_])->string`, true},
		{"empty_list", `f: func([] & [...func(int)->_|_])->string`, false},
		{"list_union", `f: func([] | [func(int)->_|_])->string`, true},
		{"refuted_record_guard", `f: ({tag: 1, cb: func(int)->_|_} & {tag: 2}) | {tag: 3}`, false},
		{"refuted_list_guard", `f: ([func(int)->_|_] & []) | []`, false},
		{"unresolved_guard", `f: {tag: int, cb: func(int)->_|_}`, true},
		{"unused_bound", `f: forall (A: func(int)->_|_) func(string)->string`, true},
		{"generic", `meet(A, B): func(x:A, y:B)->(A&B): x&y`, false},
		{"selected", "meet(A, B): func(x:A, y:B)->(A&B): x&y\nf: meet[int,bool]", false},
		{"alias", "Arrow(A, B) = func(A)->B\nf: Arrow(int, int&bool)", true},
		{"anchor", "quiet(A): func(A)->A\nquiet: func(int)->bool", true},
		{"composite_universal", `f: forall A {cb: func(int)->_|_, data?: A}`, true},
		{"nested_universal", `f: func(forall A {cb: func(int)->_|_, data?: A})->string`, true},
		{"universal_generic", `f: forall (A, B) {cb: func(A, B)->(A&B)}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			if err := v.Err(); err != nil {
				t.Fatalf("interface diagnostics changed the value denotation: %v", err)
			}
			err := v.Validate()
			if (err != nil) != tt.blocked {
				t.Fatalf("blocked=%v: %v", tt.blocked, err)
			}
			if err != nil && !strings.Contains(err.Error(), "interface relevance blocked") {
				t.Fatal(err)
			}
			// Validation must leave the graph intact for later refinement and
			// separate semantic operations. No error-valued arrow is installed.
			if err := v.LookupPath(cue.ParsePath("f")).Err(); err != nil && strings.Contains(tt.source, "f:") {
				t.Fatalf("checking changed a function denotation: %v", err)
			}
		})
	}
}
