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

package parser

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/internal/astinternal"
)

func TestQuantifiedSyntax(t *testing.T) {
	cases := []struct{ src, want string }{
		{`bridge: extern func(int) -> int !bridge`, `bridge: extern func(int) -> int !bridge`},
		{`codec: exists A {decode: func(bytes) -> A !decode}`, `codec: exists (A) {decode: func(bytes) -> A !decode}`},
		{`x: f[int, string](1)`, `x: f[int][string](1)`},
		{`Box(A) = {value: A}`, `Box(A) = {value: A}`},
		{`map: func<A, B>(func(A) -> B, [...A]) -> [...B]`, `map: forall (A, B) func(func(A) -> B, [...A]) -> [...B]`},
		{`f: func<A: number>(A) -> A`, `f: forall (A: number) func(A) -> A`},
		{"module(A): {\nexists State\nempty: State\npush: func(A, State) -> State\n}", `module: forall (A) exists (State) {empty: State, push: func(A, State) -> State}`},
		{"x: {\nexists T: U\nforall V: W\nvalue: T\ntransform: func(V) -> V\n}", `x: exists (T: U) forall (V: W) {value: T, transform: func(V) -> V}`},
		{`p: seal #Showable with (A = int) {value: 7, show: func(x: int) -> string: "x"}`, `p: seal #Showable with (A=int) {value: 7, show: func(x: int) -> string: "x"}`},
		{`v: (open p as (A, P) {out: P.show(P.value)}).out`, `v: (open p as (A, P) {out: P.show(P.value)}).out`},
		{`id(A): func(x: A) -> A: x`, `id: forall (A) func(x: A) -> A: x`},
		{`pair(A: number): func(A, A) -> [A, A]`, `pair: forall (A: number) func(A, A) -> [A, A]`},
		{`r: forall (A, B: A) {a: A, b: B}`, `r: forall (A, B: A) {a: A, b: B}`},
		{`p: exists A {value: A, show: func(A) -> string}`, `p: exists (A) {value: A, show: func(A) -> string}`},
		{`f: func(forall A func(A) -> A) -> int`, `f: func(forall (A) func(A) -> A) -> int`},
		{`r: forall (A in Type(1): number) [...A]`, `r: forall (A in Type(1): number) [...A]`},
		{`r: exists (n in 1 | 2) {value: n}`, `r: exists (n in 1|2) {value: n}`},
		{`r: (forall A A) | int`, `r: (forall (A) A)|int`},
	}
	for _, tt := range cases {
		t.Run(tt.src, func(t *testing.T) {
			f, err := ParseFile("test.cue", "@experiment(quantified)\n"+tt.src)
			if err != nil {
				t.Fatal(err)
			}
			got := astinternal.DebugStr(f.Decls[1])
			if got != tt.want {
				t.Fatalf("got %s; want %s", got, tt.want)
			}
			ast.Walk(f, func(n ast.Node) bool { return true }, nil)
		})
	}
}

func TestQuantifiedSyntaxErrors(t *testing.T) {
	for _, src := range []string{
		`x: forall () int`,
		`x: exists () int`,
		`x: forall (A: ) A`,
		`x: forall (A, B int`,
		`x: forall A`,
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := ParseFile("test.cue", "@experiment(quantified)\n"+src); err == nil {
				t.Fatal("accepted malformed quantifier")
			}
		})
	}
	_, err := ParseFile("test.cue", `x: forall A A`)
	if err == nil || !strings.Contains(err.Error(), "requires @experiment(quantified)") {
		t.Fatalf("unexpected disabled syntax error: %v", err)
	}
	// No new reserved words in the data language.
	if _, err := ParseFile("test.cue", `forall: 1, exists: 2, x: forall, y: exists, z: forall(1)`); err != nil {
		t.Fatal(err)
	}
}
