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

package subsume_test

import (
	"slices"
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/core/subsume"
)

func TestRelevanceAnchoredUniversals(t *testing.T) {
	for _, tt := range []struct {
		name    string
		clauses []string
		valid   bool
	}{
		{"meet", []string{`forall (A, B) func(A, B) -> (A & B)`}, true},
		{"quiet", []string{`forall A func(A) -> A`, `func(int) -> bool`}, false},
		{"quiet_renamed", []string{`forall Z func(Z) -> Z`, `func(int) -> bool`}, false},
		{"quiet_floated", []string{`forall A func(A) -> A`, `forall A func(int) -> bool`}, false},
		{"bounded_disjoint", []string{`forall (A: string) func(A) -> A`, `func(int) -> bool`}, true},
		{"bounded_match", []string{`forall (A: number) func(A) -> A`, `func(int) -> bool`}, false},
		{"bounded_literal", []string{`forall (A: number) func(A) -> A`, `func(1) -> bool`}, false},
		{"repeated_same", []string{`forall A func(A, A) -> A`, `func(int, int) -> bool`}, false},
		{"repeated_different", []string{`forall A func(A, A) -> A`, `func(int, string) -> bool`}, true},
		{"result_only", []string{`forall A func(int) -> A`, `func(int) -> bool`}, true},
		{"record", []string{`forall A func({value: A}) -> A`, `func({value: int}) -> bool`}, false},
		{"record_width", []string{`forall A func({value: A}) -> A`, `func({value: int, extra: true}) -> bool`}, false},
		{"fixed_list", []string{`forall A func([A, A]) -> A`, `func([int, int]) -> bool`}, false},
		{"homogeneous_list", []string{`forall A func([...A]) -> A`, `func([...int]) -> bool`}, false},
		{"callback", []string{`forall A func(func(A) -> A) -> A`, `func(func(int) -> int) -> bool`}, false},
		{"polymorphic_callback", []string{`forall A func(forall B func(B) -> A) -> A`, `func(forall C func(C) -> int) -> bool`}, false},
		{"kind_guard", []string{`forall A func(A & int) -> A`, `func(1) -> bool`}, false},
		{"joint_instances", []string{`forall A func(A) -> (0 | 1)`, `func(int) -> (1 | 2)`, `func(int) -> (0 | 2)`}, false},
		{"irrelevant_label", []string{`forall A func(x!: A) -> A`, `func(y!: int) -> bool`}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			clauses := relevanceClauses(t, ctx, tt.clauses)
			for range 2 {
				err := subsume.CheckDeclarationRelevance(ctx, clauses)
				if (err == nil) != tt.valid || err != nil && err.Limit {
					t.Fatalf("valid=%v: %v", tt.valid, err)
				}
				slices.Reverse(clauses)
			}
		})
	}
}

func TestRelevanceAliasDependencies(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `
Independent(A) = int
Dependent(A) = {value: A}
generic: forall A func(A) -> A
anchor: forall A func(Independent(A)) -> bool
record: forall A func(Dependent(A)) -> A
recordAnchor: func({value: int}) -> bool
`)
	clauses := make(map[string]adt.FuncType)
	for _, arc := range root.Arcs {
		if f, ok := adt.Unwrap(arc).(*adt.FuncValue); ok {
			clauses[arc.Label.SelectorString(ctx)] = adt.FuncType{Fn: f.Fn, Env: f.Env}
		}
	}
	for _, pair := range [][2]string{{"generic", "anchor"}, {"record", "recordAnchor"}} {
		err := subsume.CheckDeclarationRelevance(ctx, []adt.FuncType{clauses[pair[0]], clauses[pair[1]]})
		if err == nil || err.Limit {
			t.Fatalf("%v: lost alias-expanded dependency: %v", pair, err)
		}
	}
}
