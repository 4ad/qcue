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

// A contract may constrain an abstract call only after original-packet
// admission. These cases independently enumerate exact record presence and
// scalar membership, then put the packet in several structural contexts.
func TestQuantifiedBoundaryAbstractAdmission(t *testing.T) {
	for _, label := range []string{"a", "_a"} {
		for _, context := range []string{`%s`, `{nested: %s}`, `[%s]`} {
			for _, tc := range []struct {
				packet string
				member bool
			}{
				{`{}`, false},
				{fmt.Sprintf(`{%s?: 1}`, label), false},
				{fmt.Sprintf(`{%s: 2}`, label), false},
				{fmt.Sprintf(`{%s: 1}`, label), true},
				{fmt.Sprintf(`{%s: 1, extra: 2}`, label), true},
			} {
				t.Run(label+"/"+context+"/"+tc.packet, func(t *testing.T) {
					guard := fmt.Sprintf(context, fmt.Sprintf(`{%s: 1}`, label))
					packet := fmt.Sprintf(context, tc.packet)
					v := semanticValue(t, fmt.Sprintf("f: func(%s) -> string\nout: f(%s) & 0", guard, packet))
					if refuted := v.Validate() != nil; refuted != tc.member {
						t.Fatalf("refuted=%v, original packet admitted=%v: %v", refuted, tc.member, v.Validate())
					}
					if err := v.Validate(cue.Concrete(true)); err == nil {
						t.Fatal("abstract call manufactured an execution witness")
					}
				})
			}
		}
	}
	// An intersection contributes every established consequence, regardless
	// of its head clause. A packet outside one domain remains possible.
	for _, contract := range []string{
		`(func({a: 1}) -> string) & (func({}) -> int)`,
		`(func({}) -> int) & (func({a: 1}) -> string)`,
	} {
		v := semanticValue(t, "f: "+contract+"\nout: f({}) & 0")
		if err := v.Validate(); err != nil {
			t.Fatalf("inapplicable clause constrained abstract result: %v", err)
		}
	}
}

func TestQuantifiedBoundaryAbstractRefinement(t *testing.T) {
	const original = `f: func({a: 1}) -> string
out: f({}) & 0`
	const implementation = `f: func(x: {}) -> (int|string): {
    if len(x) == 0 {r: 0}
    if len(x) > 0 {r: "ok"}
}.r`
	v := semanticValue(t, original)
	if err := v.Validate(); err != nil {
		t.Fatalf("satisfiable abstract program refuted: %v", err)
	}
	// Refinement of the same graph and rebuilding the combined source must
	// agree. An implementation can resolve uncertainty, never repair bottom.
	for _, refined := range []cue.Value{
		v.Unify(semanticValue(t, implementation)),
		semanticValue(t, original+"\n"+implementation),
	} {
		if err := refined.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, refined, "out", "0")
	}
}
