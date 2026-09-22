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

package format_test

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/internal/astinternal"
	"cuelang.org/go/internal/cueexperiment"
)

func TestQuantifiedRoundTrip(t *testing.T) {
	if err := cueexperiment.Init(); err != nil {
		t.Fatal(err)
	}
	defer func(v bool) { cueexperiment.Flags.FormatV2 = v }(cueexperiment.Flags.FormatV2)
	for _, v2 := range []bool{false, true} {
		t.Run(fmt.Sprint(v2), func(t *testing.T) {
			cueexperiment.Flags.FormatV2 = v2
			for _, src := range []string{
				`bridge: extern func(int) -> int !bridge`,
				`codec: exists A {decode: func(bytes) -> A !decode}`,
				`x: f[int, string](1)`,
				`Box(A, B: A) = {left: A, right: B}`,
				`f: func<A: number, B>(A) -> B`,
				"module(A): {\nexists State\nempty: State\npush: func(A, State) -> State\n}",
				`p: seal #Counter with (State = int & >=0) {zero: 0}`,
				`r: (open p as (A, P) {out: P.read(P.zero)}).out`,
				`id(A): func(x: A) -> A: x`,
				`use: func(p: forall A func(A) -> A) -> [int, string]`,
				`p: exists (A: number, B: A) {value: B}`,
				`p: (forall A A) | int`,
				`p: (exists A {value: A}).value`,
				`p: forall (A in Type(1): number) [...A]`,
			} {
				t.Run(src, func(t *testing.T) {
					in := "@experiment(quantified)\n" + src + "\n"
					f, err := parser.ParseFile("test.cue", in)
					if err != nil {
						t.Fatal(err)
					}
					out, err := format.Source([]byte(in))
					if err != nil {
						t.Fatal(err)
					}
					g, err := parser.ParseFile("test.cue", out)
					if err != nil {
						t.Fatalf("%v\n%s", err, out)
					}
					if a, b := astinternal.DebugStr(f), astinternal.DebugStr(g); a != b {
						t.Fatalf("syntax changed: %s => %s", a, b)
					}
					again, err := format.Source(out)
					if err != nil {
						t.Fatal(err)
					}
					if string(out) != string(again) {
						t.Fatalf("unstable format:\n%s\n%s", out, again)
					}
				})
			}
		})
	}
}
