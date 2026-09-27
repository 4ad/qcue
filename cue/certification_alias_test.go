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

func TestQuantifiedAliasBodyCertification(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"closure", `F(A) = func(x: A) -> A: x
f: func(x: int) -> int: F(int)(x)`, `f(3)`, `3`},
		{"generic", `F(A) = func(x: A) -> A: x
f(B): func(x: B) -> B: F(B)(x)`, `f(3)`, `3`},
		{"local_let", `f: func(x: int) -> int: {let y = x + 1
out: y + y}.out`, `f(3)`, `8`},
		{"nested_lets", `f: func(x: int) -> int: {let y = x + 1
inner: {let z = y + 1
out: z}}.inner.out`, `f(3)`, `5`},
		{"captured_let", `let n = 3
f: func() -> 4: n + 1`, `f()`, `4`},
		{"let_callback", `let identity = func(x: int) -> int: x
f: func(x: int) -> int: identity(x)`, `f(3)`, `3`},
		{"unused_polymorphic_argument", `Ignore(A) = 1
f: func() -> 1: Ignore(forall A func(x: A) -> A: x)`, `f()`, `1`},
		{"nested_type_alias", `Ignore(A) = 1
At(A) = [2, 3][A]
f: func() -> 1: Ignore(At(0))`, `f()`, `1`},
		{"polymorphic_record_alias", `Ignore(A) = 1
R(A) = {f: func(x: A) -> A: x}
f: func() -> 1: Ignore(forall A R(A))`, `f()`, `1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source + "\nout: " + tt.call)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", tt.want)
		})
	}
	for _, source := range []string{
		`F(A: number) = func(x: A) -> A: x
f: func() -> string: F(string)("x")`,
		`f: func(x: {a: int}) -> int: {let y = x.b
out: y & _|_}.out`,
		`let identity = func(x: int) -> int: x
f: func(x: string) -> int: identity(x)`,
		`f: func(x: int) -> int: {let y = x + "bad"
out: y & _|_}.out`,
		`Ignore(A) = 1
f: func() -> 1: Ignore(1 + "bad")`,
		`Ignore(A) = 1
f: func() -> 1: Ignore({bad: 1 + "bad"})`,
		`Ignore(A) = 1
f: func() -> 1: Ignore((int & string) & (1 + "bad"))`,
		`Ignore(A) = 1
f: func() -> 1: Ignore(func(x: int) -> string: x)`,
		`Ignore(A) = 1
f: func() -> 1: Ignore(forall A func(x: A) -> A: 0)`,
		`Ignore(A) = 1
f: func() -> 1: Ignore({bad: func(x: int) -> string: x})`,
		`Ignore(A) = 1
Bad(A) = 1 + "bad"
f: func() -> 1: Ignore(Bad(int))`,
	} {
		v := cuecontext.New().CompileString(source)
		f := v.LookupPath(cue.ParsePath("f"))
		if !f.Exists() {
			t.Fatalf("invalid test source: %v", v.Err())
		}
		if err := f.Validate(cue.Concrete(true)); err == nil {
			t.Errorf("unchecked abbreviation certified: %s", source)
		}
	}
}
