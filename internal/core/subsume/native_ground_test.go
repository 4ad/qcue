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
	"cuelang.org/go/internal/core/subsume"
)

func TestNativeGroundPurity(t *testing.T) {
	for _, tc := range []struct {
		name              string
		pure, nonConcrete bool
	}{
		{"pure", true, false},
		{"effectful", false, false},
		{"schema", true, true},
	} {
		for _, capability := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/call", true: "/capability"}[capability], func(t *testing.T) {
				ctx := eval.NewContext(runtime.New(), nil)
				calls := 0
				arg := &adt.String{Str: "argument"}
				want := &adt.String{Str: "result"}
				b := &adt.Builtin{
					Name: "Test", Pure: tc.pure, NonConcrete: tc.nonConcrete,
					Params: []adt.Param{{Value: &adt.BasicType{K: adt.StringKind}}},
					Result: adt.StringKind,
					Func: func(adt.BuiltinCallContext) adt.Expr {
						calls++
						return want
					},
				}
				var err *adt.Bottom
				if capability {
					b.Types = []adt.FuncType{{Fn: &adt.Function{
						Quantified: true,
						Params:     []adt.FuncParam{{Positional: true, Value: arg}},
						Ret:        want,
					}}}
					err = subsume.ValidateBuiltin(ctx, b)
				} else {
					f := &adt.FuncValue{Fn: &adt.Function{
						Quantified: true,
						Ret:        want,
						Body:       &adt.CallExpr{Fun: b, Args: []adt.Expr{arg}},
					}}
					err = ctx.CheckFunction(ctx, f)
				}
				valid := tc.pure && !tc.nonConcrete
				if (err == nil) != valid {
					t.Fatalf("valid=%v: %v", valid, err)
				}
				if (calls != 0) != valid {
					t.Fatalf("valid=%v, native executed %d times", valid, calls)
				}
			})
		}
	}
}
