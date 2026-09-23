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

func TestQuantifiedAliasWitness(t *testing.T) {
	for _, alias := range []string{
		`let Y = x & int`,
		`let Z = x & int; let Y = Z`,
		`let Y = {a: x & int}`,
	} {
		for _, runtimeFirst := range []bool{false, true} {
			t.Run(alias, func(t *testing.T) {
				ctx := cuecontext.New()
				argument := "2"
				if alias == `let Y = {a: x & int}` {
					argument = "{a: 2}"
				}
				use := "f: func(y: Y) -> string: \"ok\"\nout: f(" + argument + ")\n"
				if runtimeFirst {
					use = "runtime: Y\n" + use
				} else {
					use += "runtime: Y\n"
				}
				v := ctx.CompileString("x: int\n" + strings.ReplaceAll(alias, ";", "\n") + "\n" + use)
				out := v.LookupPath(cue.ParsePath("out"))
				if err := out.Validate(); err != nil {
					t.Fatalf("pending witness: %v", err)
				}
				if _, err := out.MarshalJSON(); err == nil {
					t.Fatal("unresolved alias witness was erased")
				}
				for _, n := range []int{1, 2} {
					r := v.FillPath(cue.ParsePath("x"), n).LookupPath(cue.ParsePath("out"))
					if _, err := r.MarshalJSON(); (err == nil) != (n == 2) {
						t.Fatalf("witness %d: %v", n, err)
					}
				}
			})
		}
	}
	// Boolean predicate abbreviations keep their ordinary predicate meaning.
	v := cuecontext.New().CompileString(`
x: int
let Nat = int & >=0
let Either = x | string
f: func(y: Nat) -> int: y
g: func(y: Either) -> string: "ok"
out: [f(2), g("s")]
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[2,"ok"]` {
		t.Fatalf("predicate abbreviation: %s, %v", got, err)
	}
}
