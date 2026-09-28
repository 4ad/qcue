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
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
)

func TestPropagationRefutations(t *testing.T) {
	for _, tt := range []struct {
		name, source          string
		observation, rejected bool
	}{
		{"fixed_input", `f:func(x:number)->int:x`, false, true},
		{"live_input", "A:number\nf:func(x:A)->int:x", false, false},
		{"live_result", "R:int\nf:func()->R:2", false, false},
		{"excluded_result", "R:int & >2\nf:func()->R:2", false, true},
		{"coverage", `f:(func(x:int)->int:x)&(func(number)->number)`, false, true},
		{"live_coverage", "Small:int\nf:(func(x:Small)->int:x)&(func(int)->int)", false, false},
		{"integer_region", `f:func(int & >0 & <2)->_|_`, true, true},
		{"fractional_region", `f:func(>0.1 & <0.2)->_|_`, true, true},
		{"record_region", `f:func({x:1})->_|_`, true, true},
		{"tuple_region", `f:func([1,true])->_|_`, true, true},
		{"optional_guard", `f:{cb?:func(int)->_|_}`, true, false},
		{"surviving_alternative", `f:(func(int)->_|_) | (func(int)->int)`, true, false},
		{"rejected_alternatives", `f:(func(int)->_|_) | (func(string)->_|_)`, true, true},
		{"live_region", "limit:int\nf:func(int & >limit & <10)->_|_", true, false},
		{"ground_region", "limit:0\nf:func(int & >limit & <10)->_|_", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			root := parse(t, ctx, tt.source)
			field := propagationField(t, ctx, root, "f")
			var b *adt.Bottom
			if tt.observation {
				b = ctx.CheckInterfaces(ctx, field)
			} else {
				b = ctx.CheckFunction(ctx, adt.Unwrap(field).(*adt.FuncValue))
			}
			if b == nil || b.Code != adt.BlockedError {
				t.Fatalf("missing checking diagnostic: %v", b)
			}
			var rejected []*adt.Goal
			for _, goal := range ctx.Propagation.Goals() {
				if goal.State == adt.Rejected {
					if goal.Support == nil {
						t.Fatal("refutation lacks checked support")
					}
					rejected = append(rejected, goal)
				}
			}
			if (len(rejected) != 0) != tt.rejected {
				t.Fatalf("rejected=%v; want %v: %v", len(rejected) != 0, tt.rejected, b)
			}
			ctx.Propagation.Notify(field)
			ctx.Propagation.Drain()
			for _, goal := range rejected {
				if goal.State != adt.Rejected {
					t.Fatal("a fixed refutation was forgotten")
				}
			}
			if field.Bottom() != nil {
				t.Fatal("negative checking evidence became semantic bottom")
			}
		})
	}
}

func TestPropagationProofBudgetResume(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `f:func(x:int)->int:x+1`)
	f := adt.Unwrap(propagationField(t, ctx, root, "f")).(*adt.FuncValue)
	graph := ctx.Propagation
	graph.Charge(graph.Remaining() - 2)
	if b := ctx.CheckFunction(ctx, f); b == nil || !b.IsIncomplete() {
		t.Fatalf("exhaustion did not retain a residual goal: %v", b)
	}
	limited := false
	for _, goal := range graph.Residual() {
		limited = limited || goal.Wait == adt.WorkLimit
	}
	if !limited {
		t.Fatal("kernel work was not charged to the shared allowance")
	}
	graph.AddBudget(10000)
	graph.Drain()
	if b := ctx.CheckFunction(ctx, f); b != nil {
		t.Fatalf("retained proof did not resume: %v", b)
	}
	if graph.Remaining() >= 10000 {
		t.Fatal("resumed proof did not account for its work")
	}
}

func propagationField(t *testing.T, ctx *adt.OpContext, root *adt.Vertex, name string) *adt.Vertex {
	t.Helper()
	for _, field := range root.Arcs {
		if field.Label.SelectorString(ctx) == name {
			return field
		}
	}
	t.Fatalf("missing field %s", name)
	return nil
}
