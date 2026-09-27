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

func TestSynthesizedBoundDiagnostic(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	env := &adt.Environment{Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	bound, ok := ctx.Evaluate(env, &adt.BoundExpr{Op: adt.LessThanOp, Expr: ctx.NewInt64(0)})
	if !ok || bound == nil {
		t.Fatal("missing synthesized bound")
	}
	if bound.Source() != nil {
		t.Fatal("synthesized bound acquired a typed nil source")
	}
	v := &adt.Vertex{}
	v.AddConjunct(adt.MakeRootConjunct(nil, bound))
	v.AddConjunct(adt.MakeRootConjunct(nil, ctx.NewInt64(1)))
	v.Finalize(ctx)
	if b := v.Bottom(); b == nil || b.IsIncomplete() || b.Err.Error() == "" {
		t.Fatalf("missing bound-conflict diagnostic: %v", b)
	}
}
