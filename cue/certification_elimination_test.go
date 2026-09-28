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

func TestQuantifiedExplicitEliminationCertification(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"callback", `id(A): func(x: A) -> A: x
f: func(id: (forall A func(A) -> A)) -> [int, string]: [id[int](3), id[string]("three")]`, `f(id)`, `[3,"three"]`},
		{"rigid", `id(A): func(x: A) -> A: x
f(B): func(id: (forall A func(A) -> A), x: B) -> B: id[B](x)`, `f(id, 3)`, `3`},
		{"impredicative", `id(A): func(x: A) -> A: x
f: func() -> int: id[forall A func(A) -> A](id)[int](3)`, `f()`, `3`},
		{"bounded", `id(A: number): func(x: A) -> A: x
f: func(x: int) -> int: id[int](x)`, `f(3)`, `3`},
		{"record_union", `f: func(r: {a: 1} | {a: 2, b: true}) -> (1 | 2): r.a`, `f({a: 2, b: true})`, `2`},
		{"callback_union", `f: func(r: {a: func(int) -> 1} | {a: func(int) -> 2}) -> (1 | 2): r.a(3)`, `f({a: func(int) -> 1: 1})`, `1`},
		{"list_union", `f: func(xs: [1] | [2, 3]) -> (1 | 2): xs[0]`, `f([2, 3])`, `2`},
		{"list_tail", `f: func(xs: [...int]) -> int: xs[0]`, `f([3])`, `3`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source + "\nout: " + tt.call)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("certification: %v", err)
			}
			semanticJSON(t, v, "out", tt.want)
		})
	}
}

func TestQuantifiedEliminationRequiresEveryPremise(t *testing.T) {
	for _, source := range []string{
		`f: func(id: func(int) -> int) -> int: id[int](3)`,
		`f: func(id: (forall A func(A) -> A)) -> string: id[int](3)`,
		`f: func(id: (forall (A: number) func(A) -> A)) -> string: id[string]("x")`,
		`f: func(r: {a: 1} | {b: 2}) -> int: r.a`,
		`f: func(r: {a: 1} | {a?: 2}) -> int: r.a`,
		`f: func(r: {a: 1} | {a: "x"}) -> int: r.a`,
		`f: func() -> int: (>true) & 1`,
		`f: func() -> 3: 6 / 2`,
		`f: func() -> true: 2 > 3`,
		`bad: func() -> (forall A func(A) -> A): func(x: int) -> int: x
f: func() -> int: bad()[int](3)`,
		`bad(A): func(x: A) -> A: {value: 0}.value
f: func() -> int: bad[int](3)`,
	} {
		v := cuecontext.New().CompileString(source)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err == nil {
			t.Errorf("unchecked elimination certified: %s", source)
		}
	}
}

func TestQuantifiedGroundComputationCertification(t *testing.T) {
	for _, tt := range []struct{ body, result string }{
		{"2 * 3", "6"}, {"6 / 2", "3.0"}, {"5 / 2", "2.5"},
		{`"a" + "b"`, `"ab"`}, {`'a' + 'b'`, `'ab'`},
		{`"a" * 3`, `"aaa"`}, {`3 * 'a'`, `'aaa'`},
		{"2 < 3", "true"}, {"2.0 == 2", "true"}, {`"a" < "b"`, "true"},
		{"true && false", "false"}, {"false || true", "true"},
		{"1 / 0", "int"}, {"[1][2]", "int"},
	} {
		v := cuecontext.New().CompileString("f: func() -> (" + tt.result + "): " + tt.body)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
			t.Errorf("%s: %v", tt.body, err)
		}
	}
	for _, body := range []string{`"a" * true`, `true < false`, `false && 1`, `1 + "x"`} {
		v := cuecontext.New().CompileString("f: func() -> int: (" + body + ") & _|_")
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err == nil {
			t.Errorf("failure concealed invalid primitive: %s", body)
		}
	}
}
