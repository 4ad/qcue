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
	"slices"

	"github.com/cockroachdb/apd/v3"

	"cuelang.org/go/internal/core/adt"
)

// refutation retains a robust packet witness, not a point in an upper
// approximation of a live input. Each coordinate has a checked membership in
// Positive, and every excluded product has a checked disjoint coordinate.
// Result, when present, is independently proved empty. The certificate lives
// in the same assumption store as its goal and never denotes semantic bottom.
type refutation struct {
	Store    *assumptionStore
	Positive []adt.Value
	Negative [][]adt.Value
	Witness  []adt.Value
	Result   adt.Value
	Returned adt.Value
	Required adt.Value
}

// reach proves existential reachability by finding and then checking a finite
// witness. Candidate generation is heuristic; only membership and disjointness
// proofs below have logical force. In particular a failed search is pending.
func (p *inference) reach(positive []adt.Value, negative [][]adt.Value) *refutation {
	packet := make([]adt.Value, len(positive))
	var search func(int) bool
	search = func(i int) bool {
		if !p.step() {
			return false
		}
		if i == len(packet) {
			for _, excluded := range negative {
				outside := false
				for j, value := range packet {
					outside = outside || refuted(p.eagerMeet(value, excluded[j]))
				}
				if !outside {
					return false
				}
			}
			return true
		}
		for _, candidate := range p.candidates(positive[i], make(map[adt.Value]bool)) {
			if p.includes(positive[i], candidate) {
				packet[i] = candidate
				if search(i + 1) {
					return true
				}
			}
		}
		return false
	}
	if !search(0) {
		return nil
	}
	return &refutation{Store: p.support, Positive: slices.Clone(positive),
		Negative: slices.Clone(negative), Witness: packet}
}

func (p *inference) candidates(value adt.Value, seen map[adt.Value]bool) []adt.Value {
	if value == nil || seen[value] || !p.step() {
		return nil
	}
	seen[value] = true
	defer delete(seen, value)
	number := func(n int64, exponent int32) *adt.Num {
		kind := adt.IntKind
		if exponent < 0 {
			kind = adt.FloatKind
		}
		return &adt.Num{K: kind, X: *apd.New(n, exponent)}
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.Null, *adt.Bool, *adt.Num, *adt.String, *adt.Bytes:
		return []adt.Value{x.(adt.Value)}
	case *adt.LiveType:
		// These are only candidates. reach must prove membership in x,
		// which Upper alone can never establish in the opposite direction.
		return p.candidates(x.Upper, seen)
	case *adt.RigidType:
		return nil // Its bound establishes no lower membership theorem.
	case *adt.BasicType, *adt.Top:
		return []adt.Value{number(0, 0), number(1, 0), number(-1, 0), number(15, -1),
			&adt.Bool{}, &adt.Bool{B: true}, &adt.String{}, &adt.Bytes{}, &adt.Null{}}
	case *adt.BoundValue:
		n, ok := x.Value.(*adt.Num)
		if !ok {
			return p.candidates(x.Value, seen)
		}
		out := []adt.Value{n}
		if endpoint := integerBound(x); endpoint != nil {
			out = append(out, endpoint.Value)
		}
		for _, op := range []adt.Op{adt.AddOp, adt.SubtractOp} {
			out = append(out, adt.BinOp(p.ctx, nil, op, n, number(1, 0)))
		}
		return out
	case *adt.Conjunction:
		var out []adt.Value
		for _, term := range x.Values {
			out = append(out, p.candidates(term, seen)...)
		}
		// A bounded fractional interval need not contain an integer or an
		// endpoint. Midpoints are candidates, not an assumed interval rule.
		endpoints := slices.Clone(out)
		for i, a := range endpoints {
			for _, b := range endpoints[:i] {
				if !p.step() {
					return out
				}
				if _, ok := a.(*adt.Num); !ok {
					continue
				}
				if _, ok := b.(*adt.Num); !ok {
					continue
				}
				sum := adt.BinOp(p.ctx, nil, adt.AddOp, a, b)
				out = append(out, adt.BinOp(p.ctx, nil, adt.FloatQuotientOp, sum, number(2, 0)))
			}
		}
		return out
	case *adt.Disjunction:
		var out []adt.Value
		for _, branch := range x.Values {
			out = append(out, p.candidates(branch, seen)...)
		}
		return out
	case *adt.Vertex:
		if x.Kind()&(adt.StructKind|adt.ListKind) == 0 {
			return nil
		}
		var fields []adt.Decl
		var elems []adt.Elem
		for _, field := range x.Arcs {
			if field.Label.IsLet() || field.Label.IsDef() || field.ArcType == adt.ArcOptional {
				continue
			}
			var member adt.Value
			for _, candidate := range p.candidates(field, seen) {
				if p.includes(field, candidate) {
					member = candidate
					break
				}
			}
			if member == nil {
				return nil
			}
			if x.IsList() {
				elems = append(elems, member)
			} else {
				fields = append(fields, &adt.Field{Label: field.Label, Value: member})
			}
		}
		var expr adt.Expr = &adt.StructLit{Decls: fields}
		if x.IsList() {
			expr = &adt.ListLit{Elems: elems}
		}
		if v, ok := p.schema(nil, expr).(*adt.Vertex); ok {
			return []adt.Value{v.ToDataAll(p.ctx)}
		}
	}
	return nil
}

func (p *inference) coverageRefutation(target, source adt.FuncType) *refutation {
	if target.Fn.Open || source.Fn.Open || len(target.Fn.Params) != len(source.Fn.Params) {
		return nil
	}
	matches := adt.MatchFuncValueParams(target.Fn, &adt.FuncValue{Fn: source.Fn})
	var positive, negative []adt.Value
	for i, j := range matches {
		if j < 0 {
			return nil
		}
		a, b := target.Fn.Params[i], source.Fn.Params[j]
		if a.ArcType == adt.ArcOptional || b.ArcType == adt.ArcOptional || a.Default != nil || b.Default != nil {
			return nil
		}
		positive = append(positive, p.schema(target.Env, a.Value))
		negative = append(negative, p.schema(source.Env, b.Value))
	}
	return p.reach(positive, [][]adt.Value{negative})
}

// A result counterexample needs execution evidence, not merely a point in a
// synthesized upper bound. The direct literal and parameter-return rules
// below supply that evidence. Other bodies remain pending when unproved.
func (p *inference) resultRefutation(source, target adt.FuncType, want adt.Value) *refutation {
	if want == nil || source.Fn.Open || target.Fn.Open {
		return nil
	}
	var returned adt.Value
	index := -1
	switch body := source.Fn.Body.(type) {
	case *adt.Num, *adt.String, *adt.Bytes, *adt.Bool, *adt.Null:
		returned = body.(adt.Value)
	case *adt.FieldReference:
		if body.UpCount != 0 {
			return nil
		}
		matches := adt.MatchFuncValueParams(target.Fn, &adt.FuncValue{Fn: source.Fn})
		for i, j := range matches {
			if j >= 0 && source.Fn.Params[j].Local == body.Label {
				index = i
			}
		}
		if index < 0 {
			return nil
		}
	default:
		return nil
	}
	var positive, excluded []adt.Value
	for _, param := range target.Fn.Params {
		positive = append(positive, p.schema(target.Env, param.Value))
		excluded = append(excluded, &adt.Top{})
	}
	var negative [][]adt.Value
	if index >= 0 {
		excluded[index] = want
		negative = [][]adt.Value{excluded}
	} else if !refuted(p.eagerMeet(returned, want)) {
		return nil
	}
	proof := p.reach(positive, negative)
	if proof == nil {
		return nil
	}
	if index >= 0 {
		returned = proof.Witness[index]
	}
	proof.Returned, proof.Required = returned, want
	return proof
}
