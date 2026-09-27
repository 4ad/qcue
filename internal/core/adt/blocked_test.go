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

package adt_test

import (
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
)

func TestBlockedJudgments(t *testing.T) {
	for _, name := range []string{
		"direct", "left_union", "right_union", "default", "default_blocked",
		"contradiction", "incomplete", "required", "optional", "equal_bottom", "not_bottom",
		"record_equal_bottom", "record_not_bottom",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			blocked := &adt.Bottom{Code: adt.BlockedError, Err: ctx.Newf("missing certificate")}
			one := ctx.NewInt64(1)
			var expr adt.Expr = blocked
			switch name {
			case "left_union", "right_union", "default", "default_blocked":
				branches := []adt.Disjunct{{Val: blocked}, {Val: one}}
				if name == "right_union" {
					branches[0], branches[1] = branches[1], branches[0]
				}
				branches[0].Default = name == "default_blocked"
				branches[1].Default = name == "default"
				expr = &adt.DisjunctionExpr{Values: branches, HasDefaults: branches[0].Default || branches[1].Default}
			case "contradiction":
				expr = &adt.BinaryExpr{Op: adt.AndOp, X: blocked,
					Y: &adt.BinaryExpr{Op: adt.AndOp, X: one, Y: ctx.NewInt64(2)}}
			case "incomplete":
				expr = &adt.BinaryExpr{Op: adt.AndOp, X: blocked,
					Y: &adt.Bottom{Code: adt.IncompleteError, Err: ctx.Newf("pending data")}}
			case "required", "optional":
				field := &adt.Field{Label: ctx.StringLabel("field"), Value: blocked}
				if name == "optional" {
					field.ArcType = adt.ArcOptional
				}
				expr = &adt.StructLit{Decls: []adt.Decl{field}}
			case "equal_bottom", "not_bottom", "record_equal_bottom", "record_not_bottom":
				op := adt.EqualOp
				if name == "not_bottom" || name == "record_not_bottom" {
					op = adt.NotEqualOp
				}
				var subject adt.Expr = blocked
				if name == "record_equal_bottom" || name == "record_not_bottom" {
					subject = &adt.StructLit{Decls: []adt.Decl{&adt.Field{Label: ctx.StringLabel("field"), Value: blocked}}}
				}
				expr = &adt.BinaryExpr{Op: op, X: subject,
					Y: &adt.Bottom{Code: adt.LegacyUserError, Err: ctx.Newf("explicit failure")}}
			}
			v := &adt.Vertex{}
			v.AddConjunct(adt.MakeRootConjunct(nil, expr))
			v.Finalize(ctx)
			cfg := &adt.ValidateConfig{CheckFunction: func(*adt.OpContext, *adt.FuncValue) *adt.Bottom { return nil }}
			b := adt.Validate(ctx, v, cfg)
			if b == nil || b.Code != adt.BlockedError {
				t.Fatalf("ordinary validation lost the static diagnostic: %v", b)
			}
			if !b.IsIncomplete() {
				t.Fatal("a blocked judgment became a semantic refutation")
			}
		})
	}
}

func TestBlockedErrorCombination(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	blocked := &adt.Bottom{Code: adt.BlockedError, Err: ctx.Newf("missing certificate")}
	for code := adt.EvalError; code <= adt.BlockedError; code++ {
		other := &adt.Bottom{Code: code, Err: ctx.Newf("other diagnostic")}
		for _, pair := range [][2]*adt.Bottom{{blocked, other}, {other, blocked}} {
			if got := adt.CombineErrors(nil, pair[0], pair[1]); got.Code != adt.BlockedError {
				t.Fatalf("blocked combined with %v became %v", code, got.Code)
			}
		}
	}
}
