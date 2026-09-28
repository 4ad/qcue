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

// Each alternative must admit the operation; a valid arm cannot conceal an
// invalid one. Checked failures propagate only after both operands have been
// independently synthesized by expr.
func (p *inference) binary(x *adt.BinaryExpr, a, b adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	if x.Op != adt.AndOp {
		if refuted(a) {
			return a
		}
		if refuted(b) {
			return b
		}
		for side, operand := range []adt.Value{a, b} {
			if union, ok := adt.Unwrap(operand).(*adt.Disjunction); ok {
				var results []adt.Value
				for _, branch := range union.Values {
					left, right := a, b
					if side == 0 {
						left = branch
					} else {
						right = branch
					}
					result := p.binary(x, left, right)
					if result == nil {
						return nil
					}
					results = append(results, result)
				}
				return proofUnion(results)
			}
		}
	}
	ka, kb := a.Kind(), b.Kind()
	ground := func() adt.Value {
		if !adt.IsConcrete(a) || !adt.IsConcrete(b) ||
			ka&(adt.StructKind|adt.ListKind) != 0 || kb&(adt.StructKind|adt.ListKind) != 0 {
			return nil
		}
		return adt.BinOp(p.ctx, x, x.Op, adt.Unwrap(a), adt.Unwrap(b))
	}
	switch x.Op {
	case adt.AndOp:
		// Both operands have their own derivation. Their meet constrains
		// successful results even when they conflict; it is an operation
		// in the source body, never a filter supplied by an annotation.
		return p.sourceMeet(a, b)
	case adt.AddOp, adt.SubtractOp, adt.MultiplyOp, adt.FloatQuotientOp:
		if ka&adt.NumberKind == ka && kb&adt.NumberKind == kb {
			if v := ground(); v != nil {
				return v
			}
			if x.Op == adt.AddOp || x.Op == adt.SubtractOp {
				if n, ok := adt.Unwrap(b).(*adt.Num); ok {
					return p.schema(nil, p.translateNumber(a, n, x.Op))
				}
				if n, ok := adt.Unwrap(a).(*adt.Num); ok && x.Op == adt.AddOp {
					return p.schema(nil, p.translateNumber(b, n, x.Op))
				}
			}
			kind := ka | kb
			if x.Op == adt.FloatQuotientOp {
				kind = adt.NumberKind
			}
			return &adt.BasicType{K: kind}
		}
		if x.Op == adt.AddOp && ka == kb && (ka == adt.StringKind || ka == adt.BytesKind) {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: ka}
		}
		if x.Op == adt.MultiplyOp &&
			(ka == adt.IntKind && (kb == adt.StringKind || kb == adt.BytesKind) ||
				kb == adt.IntKind && (ka == adt.StringKind || ka == adt.BytesKind)) {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: (ka | kb) &^ adt.IntKind}
		}
	case adt.EqualOp, adt.NotEqualOp:
		data := adt.NumberKind | adt.StringKind | adt.BytesKind | adt.BoolKind | adt.NullKind | adt.StructKind | adt.ListKind
		modern := adt.Pos(x).Experiment().StructCmp
		if modern && ka&^data == 0 && kb&^data == 0 ||
			ka == kb && ka&(data&^adt.StructKind) == ka ||
			ka&adt.NumberKind == ka && kb&adt.NumberKind == kb ||
			ka == adt.NullKind || kb == adt.NullKind {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: adt.BoolKind}
		}
	case adt.LessThanOp, adt.LessEqualOp, adt.GreaterThanOp, adt.GreaterEqualOp:
		if ka&adt.NumberKind == ka && kb&adt.NumberKind == kb ||
			ka == kb && (ka == adt.StringKind || ka == adt.BytesKind) {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: adt.BoolKind}
		}
	case adt.MatchOp, adt.NotMatchOp:
		if ka == adt.StringKind && kb&^(adt.StringKind|adt.BytesKind) == 0 {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: adt.BoolKind}
		}
	case adt.BoolAndOp, adt.BoolOrOp:
		if ka == adt.BoolKind && kb == adt.BoolKind {
			if v := ground(); v != nil {
				return v
			}
			return &adt.BasicType{K: adt.BoolKind}
		}
	}
	return nil
}

// List selection is partial and preserves the element description even when
// the index is only known to be an integer. Record selection still requires
// an independently known field inventory for every possible label.
func (p *inference) index(value, index adt.Value) adt.Value {
	if value == nil || index == nil || !p.step() {
		return nil
	}
	if refuted(value) {
		return value
	}
	if refuted(index) {
		return index
	}
	for side, operand := range []adt.Value{value, index} {
		if union, ok := adt.Unwrap(operand).(*adt.Disjunction); ok {
			var results []adt.Value
			for _, branch := range union.Values {
				x, i := value, index
				if side == 0 {
					x = branch
				} else {
					i = branch
				}
				result := p.index(x, i)
				if result == nil {
					return nil
				}
				results = append(results, result)
			}
			return proofUnion(results)
		}
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType:
		return p.index(x.Bound, index)
	case *adt.LiveType:
		return p.index(x.Upper, index)
	}
	if label, ok := adt.Unwrap(index).(*adt.String); ok && value.Kind() == adt.StructKind {
		return p.project(value, p.ctx.StringLabel(label.Str))
	}
	if value.Kind() != adt.ListKind || index.Kind() != adt.IntKind {
		return nil
	}
	if n, ok := adt.Unwrap(index).(*adt.Num); ok {
		i, err := n.X.Int64()
		if err != nil || i < 0 || i >= adt.MaxIndex {
			return &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("list index out of range")}
		}
		return p.project(value, adt.MakeIntLabel(adt.IntLabel, i))
	}
	return p.listFold(value, false)
}
