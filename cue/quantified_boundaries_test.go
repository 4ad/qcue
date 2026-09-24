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

// Erasure is a congruence for runtime observations: putting two inhabitants
// into the same data context must not make their retained contracts visible.
// Conversely, differing code origins, captures, and saved arguments remain
// observable through each context. Expected identities are specified here
// independently of the evaluator's graph and closure comparators.
func TestQuantifiedBoundaryRuntimeEquality(t *testing.T) {
	cases := []struct {
		name, declarations, a, b string
		equal                    bool
	}{
		{"contract", `f: func(x: int) -> int: x`, `f`, `f & (func(int) -> int)`, true},
		{"erasure", `f(A): func(x: A) -> A: x`, `f[int]`, `f[string]`, true},
		{"builtin", `import "strings"`, `strings.ToUpper`, `strings.ToUpper & (func(string) -> string)`, true},
		{"origin", `f: func(x: int) -> int: x
g: func(x: int) -> int: x`, `f`, `g`, false},
		{"saved", `f: func(x: int, y: int) -> int: x+y`, `f(1, ...)`, `f(2, ...)`, false},
		{"sameSaved", `f: func(x: int, y: int) -> int: x+y`, `f(1, ...)`, `f(x: 1, ...)`, true},
		{"capture", `fs: [for n in [1, 2] {func(x: int) -> int: x+n}]`, `fs[0]`, `fs[1]`, false},
		{"predicates", ``, `{x: 1, #T: int, y?: int}`, `{x: 1, #T: string, y?: string}`, true},
	}
	contexts := []string{`[%s]`, `[{x: %s}]`, `[[%s]]`, `[{x: [%s], y: 0}]`}
	for _, tc := range cases {
		for _, context := range contexts {
			t.Run(tc.name+"/"+context, func(t *testing.T) {
				a, b := fmt.Sprintf(context, tc.a), fmt.Sprintf(context, tc.b)
				v := semanticValue(t, tc.declarations+fmt.Sprintf("\nout: [%s == %s, %s != %s]", a, b, a, b))
				if err := v.Validate(cue.Concrete(true)); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", fmt.Sprintf("[%v,%v]", tc.equal, !tc.equal))
			})
		}
	}
}

func TestQuantifiedBoundaryOpaqueEquality(t *testing.T) {
	v := semanticValue(t, `
#M: exists A {left: A, right: A}
base: func(x: int) -> int: x
p: seal #M with (A = {f: func(int) -> int}) {
    left: {f: base}
    right: {f: base & (func(int) -> int)}
}
out: (open p as (A, P) {result: [
    P.left == P.right,
    [P.left] == [P.right],
    [{value: P.left}] == [{value: P.right}],
    [P.left] != [P.right],
]}).result
`)
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "out", `[true,true,true,false]`)
}
