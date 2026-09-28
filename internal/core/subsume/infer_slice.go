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

import "cuelang.org/go/internal/core/adt"

// A slice is partial: bounds and subject must be independently well typed,
// while invalid bounds may fail. Known list bounds preserve the fixed prefix;
// unknown bounds retain a homogeneous approximation of all possible elements.
func (p *inference) slice(env *adt.Environment, x *adt.SliceExpr) adt.Value {
	if x.Stride != nil {
		return nil
	}
	value := p.expr(env, x.X)
	lo, hi := adt.Value(&adt.Num{K: adt.IntKind}), adt.Value(nil)
	if x.Lo != nil {
		lo = p.expr(env, x.Lo)
	}
	if x.Hi != nil {
		hi = p.expr(env, x.Hi)
	}
	if value == nil || lo == nil || lo.Kind()&^adt.IntKind != 0 ||
		x.Hi != nil && (hi == nil || hi.Kind()&^adt.IntKind != 0) {
		return nil
	}
	return p.sliceValue(value, lo, hi)
}

func (p *inference) sliceValue(value, low, high adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType:
		return p.sliceValue(x.Bound, low, high)
	case *adt.LiveType:
		return p.sliceValue(x.Upper, low, high)
	}
	if union, ok := adt.Unwrap(value).(*adt.Disjunction); ok {
		var out []adt.Value
		for _, branch := range union.Values {
			result := p.sliceValue(branch, low, high)
			if result == nil {
				return nil
			}
			out = append(out, result)
		}
		return proofUnion(out)
	}
	if value.Kind() == adt.BytesKind {
		if adt.IsConcrete(value) && adt.IsConcrete(low) && (high == nil || adt.IsConcrete(high)) {
			return p.schema(nil, &adt.SliceExpr{X: value, Lo: low, Hi: high})
		}
		return &adt.BasicType{K: adt.BytesKind}
	}
	list, ok := value.(*adt.Vertex)
	if !ok || !list.IsList() {
		return nil
	}
	index := func(value adt.Value) (int64, bool) {
		n, ok := adt.Unwrap(value).(*adt.Num)
		if !ok {
			return 0, false
		}
		i, err := n.X.Int64()
		return i, err == nil
	}
	lo, loKnown := index(low)
	hi, hiKnown := index(high)
	if high == nil && list.IsClosedList() {
		hi, hiKnown = int64(len(list.Arcs)), true
	}
	failure := func() adt.Value {
		return &adt.Bottom{Src: list.Source(), Code: adt.EvalError, Err: p.ctx.Newf("invalid slice bounds")}
	}
	if loKnown && lo < 0 || hiKnown && hi < 0 || loKnown && hiKnown && lo > hi {
		return failure()
	}
	if list.IsClosedList() && (loKnown && lo > int64(len(list.Arcs)) || hiKnown && hi > int64(len(list.Arcs))) {
		return failure()
	}
	out := &adt.ListLit{}
	var elems []adt.Value
	if loKnown && hiKnown {
		for i := lo; i < hi; i++ {
			if !p.step() {
				return nil
			}
			elem := p.project(list, adt.MakeIntLabel(adt.IntLabel, i))
			if elem == nil {
				return nil
			}
			out.Elems = append(out.Elems, elem)
			elems = append(elems, elem)
		}
	} else {
		for a := range list.Elems() {
			elems = append(elems, a)
		}
		if !list.IsClosedList() {
			tail := p.project(list, adt.MakeIntLabel(adt.IntLabel, int64(len(list.Arcs))))
			if tail == nil {
				return nil
			}
			elems = append(elems, tail)
		}
		if len(elems) != 0 {
			out.Elems = append(out.Elems, &adt.Ellipsis{Value: proofUnion(elems)})
		}
	}
	result := p.schema(nil, out)
	if v, ok := result.(*adt.Vertex); ok {
		p.constructors[v] = out
	}
	if v, ok := result.(*adt.Vertex); ok && loKnown && hiKnown {
		fields := make(map[adt.Feature]adt.Value, len(elems))
		for i, elem := range elems {
			fields[adt.MakeIntLabel(adt.IntLabel, int64(i))] = elem
		}
		p.projections[v] = fields
	}
	return result
}
