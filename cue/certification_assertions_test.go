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
	"fmt"
	"testing"

	"cuelang.org/go/cue"
)

// An explicit assertion contributes the same body evidence regardless of
// how its result annotation is split or attached to the implementation.
func TestQuantifiedAssertionDeclarations(t *testing.T) {
	for i, source := range []string{
		`f: func(x: int) -> (int & >0): x & >0`,
		`f: func(int) -> (int & >0)
f: func(x: int) -> _: x & >0`,
		`f: func(x: int) -> int: x & >0
f: func(int) -> >0`,
		`f: func(int) -> int
f: func(x: int) -> >0: x & >0`,
		`f: func(int) -> int
f: func(int) -> >0
f: func(x: int) -> _: x & >0`,
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			v := semanticValue(t, source+"\ngood: f(3)\nbad: f(0)")
			f := v.LookupPath(cue.ParsePath("f"))
			if err := f.Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("explicit assertion did not certify: %v", err)
			}
			semanticJSON(t, v, "good", "3")
			if err := v.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
				t.Fatal("explicit assertion failed to reject zero")
			}
		})
	}
}

func TestQuantifiedPartialComputationCertification(t *testing.T) {
	for _, tt := range []struct {
		name, source, good, want, bad string
	}{
		{"meet", `f(A, B): func(x: A, y: B) -> (A & B): x & y`, `f({a: 1}, {b: true})`, `{"a":1,"b":true}`, `f[int, bool](1, true)`},
		{"tuple", `f(A): func(xs: [A, A]) -> A: xs[0] & xs[1]`, `f([{a: 1}, {b: true}])`, `{"a":1,"b":true}`, `f([1, 2])`},
		{"kind", `f: func(x: int | string) -> int: x & int`, `f(3)`, "3", `f("wrong")`},
		{"stop", `f(A): func(x: A) -> A: _|_`, "", "", `f(3)`},
		{"conflict", `f: func(x: int) -> string: x & string`, "", "", `f(3)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := semanticValue(t, tt.source+"\nbad: "+tt.bad)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("partial computation did not certify: %v", err)
			}
			if err := v.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
				t.Fatal("failing call did not reduce to bottom")
			}
			if tt.good != "" {
				v := semanticValue(t, tt.source+"\ngood: "+tt.good)
				semanticJSON(t, v, "good", tt.want)
			}
		})
	}
}

func TestQuantifiedFailureDoesNotProveUncheckedTerms(t *testing.T) {
	for _, source := range []string{
		`f: func(x: int) -> (int & >0): x`,
		`f: func(x: int | string) -> int: x`,
		`f(A): func(x: A) -> A: 0`,
		`f: func(x: {}) -> string: (1 & "wrong") & x.missing`,
		`f: func(x: {}) -> string: x.missing & (1 & "wrong")`,
		`f: func(x: {}) -> string: {failed: _|_, invalid: x.missing}.failed`,
		`helper: func(x: int) -> int: x
f: func() -> string: _|_ & helper("wrong")`,
		`hypothesis: int & string
f(A): func(x: A) -> A: hypothesis`,
	} {
		t.Run(source, func(t *testing.T) {
			v := semanticValue(t, source)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err == nil {
				t.Fatal("unchecked term acquired a conformance certificate")
			}
		})
	}
}
