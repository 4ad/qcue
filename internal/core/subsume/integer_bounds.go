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

package subsume

import (
	"github.com/cockroachdb/apd/v3"

	"cuelang.org/go/internal"
	"cuelang.org/go/internal/core/adt"
)

// integerBound gives an inclusive endpoint for an integer-valued subject.
// It is usable only with independent evidence that the subject is an integer:
// >-1 alone does not imply >=0, but int & >-1 does. This also preserves the
// integer rounding required after translating an interval by a constant.
func integerBound(bound *adt.BoundValue) *adt.BoundValue {
	n, ok := bound.Value.(*adt.Num)
	if !ok || n.X.Form != apd.Finite {
		return nil
	}
	value := &adt.Num{K: adt.IntKind}
	ctx := internal.ExactContext
	var err error
	op := adt.GreaterEqualOp
	switch bound.Op {
	case adt.GreaterEqualOp, adt.LessThanOp:
		_, err = ctx.Ceil(&value.X, &n.X)
	case adt.LessEqualOp, adt.GreaterThanOp:
		_, err = ctx.Floor(&value.X, &n.X)
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	var delta int64
	switch bound.Op {
	case adt.GreaterThanOp:
		delta = 1
	case adt.LessThanOp:
		delta = -1
	}
	if delta != 0 {
		var condition apd.Condition
		condition, err = ctx.Add(&value.X, &value.X, apd.New(delta, 0))
		if err != nil || condition.Inexact() {
			return nil
		}
	}
	if bound.Op == adt.LessThanOp || bound.Op == adt.LessEqualOp {
		op = adt.LessEqualOp
	}
	return &adt.BoundValue{Op: op, Value: value}
}
