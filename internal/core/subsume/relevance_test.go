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
	"fmt"
	"slices"
	"strings"
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/core/subsume"
)

func relevanceClauses(t *testing.T, ctx *adt.OpContext, sources []string) []adt.FuncType {
	t.Helper()
	var clauses []adt.FuncType
	for _, source := range sources {
		root := parse(t, ctx, "f: "+source)
		f, ok := adt.Unwrap(root.Arcs[0]).(*adt.FuncValue)
		if !ok {
			t.Fatalf("missing clause for %s: %v", source, root.Bottom())
		}
		clauses = append(clauses, adt.FuncType{Fn: f.Fn, Env: f.Env})
	}
	return clauses
}

func TestArrowRelevance(t *testing.T) {
	for _, tt := range []struct {
		name    string
		clauses []string
		valid   bool
	}{
		{"equal", []string{`func(int) -> int`, `func(int) -> bool`}, false},
		{"empty_result", []string{`func(int) -> _|_`}, false},
		{"partial_overlap", []string{`func(number) -> int`, `func(int) -> bool`}, false},
		{"compatible", []string{`func(number) -> number`, `func(int) -> int`}, true},
		{"disjoint", []string{`func(int) -> string`, `func(string) -> int`}, true},
		{"triple", []string{`func(int) -> (0 | 1)`, `func(int) -> (1 | 2)`, `func(int) -> (0 | 2)`}, false},
		{"empty_input", []string{`func(int & string) -> _|_`}, true},
		{"empty_interval", []string{`func(int & >0 & <1) -> _|_`}, true},
		{"interval_overlap", []string{`func(int & >=0) -> int`, `func(int & <=0) -> bool`}, false},
		{"interval_disjoint", []string{`func(int & >0) -> int`, `func(int & <=0) -> bool`}, true},
		{"required_conflict", []string{`func({a: 1}) -> int`, `func({a: 2}) -> bool`}, true},
		{"width_overlap", []string{`func({a: 1}) -> int`, `func({}) -> bool`}, false},
		{"result_fields", []string{`func(int) -> {a: 1}`, `func(int) -> {a: 2}`}, false},
		{"result_lists", []string{`func(int) -> [1]`, `func(int) -> [1, 2]`}, false},
		{"arity", []string{`func(int) -> int`, `func(int, int) -> bool`}, true},
		{"positional_label_overlap", []string{`func(x: int) -> int`, `func(x!: int) -> bool`}, false},
		{"labels_disjoint", []string{`func(x!: int) -> int`, `func(y!: int) -> bool`}, true},
		{"optional_overlap", []string{`func(x?: int) -> int`, `func(y?: int) -> bool`}, false},
		{"default_overlap", []string{`func(x: int = 1) -> int`, `func() -> bool`}, false},
		{"optional_absence", []string{`func(x?: _|_) -> int`, `func() -> bool`}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			clauses := relevanceClauses(t, ctx, tt.clauses)
			for range 2 {
				err := subsume.CheckArrowRelevance(ctx, clauses)
				if (err == nil) != tt.valid {
					t.Fatalf("valid=%v: %v", tt.valid, err)
				}
				if err != nil && err.Limit {
					t.Fatal(err)
				}
				slices.Reverse(clauses)
			}
		})
	}
}

// Compare all pairs and triples over the finite Boolean domain with an
// independent pointwise set oracle. Three clauses can have pairwise nonempty
// results and still refute their accumulated successful observation.
func TestArrowRelevanceFiniteOracle(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	sets := []string{"_|_", "false", "true", "bool"}
	var sources []string
	for _, domain := range sets {
		for _, result := range sets {
			sources = append(sources, "func("+domain+") -> "+result)
		}
	}
	clauses := relevanceClauses(t, ctx, sources)
	for a := range clauses {
		for b := range clauses {
			for c := range clauses {
				indexes := []int{a, b, c}
				valid := true
				for input := range 2 {
					result, relevant := 3, false
					for _, i := range indexes {
						if (i/4)&(1<<input) != 0 {
							relevant = true
							result &= i % 4
						}
					}
					valid = valid && (!relevant || result != 0)
				}
				err := subsume.CheckArrowRelevance(ctx, []adt.FuncType{clauses[a], clauses[b], clauses[c]})
				if (err == nil) != valid || err != nil && err.Limit {
					t.Fatalf("%v: want valid=%v, got %v", indexes, valid, err)
				}
			}
		}
	}
}

func TestArrowRelevanceRigidDeclaration(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `meet(A, B): func(A, B) -> (A & B)`)
	f := adt.Unwrap(root.Arcs[0]).(*adt.FuncValue)
	clause := adt.FuncType{Fn: f.Fn, Env: f.Env}
	var args []adt.Value
	for _, param := range adt.FunctionTypeParameters(clause) {
		args = append(args, &adt.RigidType{Param: param, Bound: &adt.Top{}})
	}
	clause = adt.BindFunctionTypes(clause, args)
	if err := subsume.CheckArrowRelevance(ctx, []adt.FuncType{clause}); err != nil {
		t.Fatal(err)
	}
}

func TestArrowRelevanceWorkLimit(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	clauses := relevanceClauses(t, ctx, []string{`func(int) -> int`, `func(string) -> bool`})
	if err, used := subsume.CheckArrowRelevanceBudget(ctx, clauses, 1); err == nil || !err.Limit || used != 1 {
		t.Fatalf("exhaustion must block: %v, used %d", err, used)
	}
	if err := subsume.CheckArrowRelevance(ctx, clauses); err != nil {
		t.Fatalf("fresh check after exhaustion: %v", err)
	}
	// Non-refuted joint results need no exponential presence expansion.
	var params []string
	for i := range 16 {
		params = append(params, fmt.Sprintf("a%d?: int", i))
	}
	clauses = relevanceClauses(t, ctx, []string{"func(" + strings.Join(params, ",") + ") -> int"})
	err := subsume.CheckArrowRelevance(ctx, clauses)
	if err != nil {
		t.Fatalf("trivial result required packet expansion: %v", err)
	}
}

func TestRelevanceExactRegions(t *testing.T) {
	for _, tt := range []struct {
		positive []string
		negative [][]string
		covered  bool
	}{
		{[]string{"bool"}, [][]string{{"false"}, {"true"}}, true},
		{[]string{"number"}, [][]string{{"int"}, {"float"}}, true},
		{[]string{"number"}, [][]string{{"<0"}, {">=0"}}, true},
		{[]string{"number"}, [][]string{{"<0"}, {">0"}}, false},
		{[]string{"int"}, [][]string{{"<=0"}, {">=1"}}, true},
		{[]string{"number"}, [][]string{{"<=0"}, {">=1"}}, false},
		{[]string{"1 | 2"}, [][]string{{"1"}, {"2"}}, true},
		{[]string{"float & >=0 & <=0"}, [][]string{{"!=0"}}, false},
		{[]string{"float & >=0 & <=0"}, [][]string{{"0"}}, false},
		{[]string{"float & >=0 & <=0"}, [][]string{{"!=0"}, {"0.0"}}, true},
		{[]string{"int", "int"}, [][]string{{"<=0", "_"}, {">0", "<0"}, {">0", ">=0"}}, true},
		{[]string{"int", "int"}, [][]string{{"<=0", "_"}, {">0", "<0"}, {">0", ">0"}}, false},
		{[]string{"{a: 1, b: true}"}, [][]string{{"{a: 1}"}}, true},
		{[]string{"{a: int}"}, [][]string{{"{b: int}"}}, false},
	} {
		t.Run(fmt.Sprint(tt.positive, tt.negative), func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			values := func(sources []string) []adt.Value {
				var values []adt.Value
				for _, source := range sources {
					root := parse(t, ctx, "v: "+source)
					values = append(values, root.Arcs[0])
				}
				return values
			}
			var negative [][]adt.Value
			for _, excluded := range tt.negative {
				negative = append(negative, values(excluded))
			}
			if got := subsume.RelevanceRegionCovered(ctx, values(tt.positive), negative); got != tt.covered {
				t.Fatalf("covered=%v; want %v", got, tt.covered)
			}
		})
	}
}
