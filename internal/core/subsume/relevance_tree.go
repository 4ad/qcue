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

// ValidateInterfaces checks explicit descriptions without changing their
// denotations. The returned Bottom is only a diagnostic carrier for Validate;
// it must not be inserted into the evaluated graph. In particular, an invalid
// arrow alternative is not an empty alternative that normalization can omit.
func ValidateInterfaces(ctx *adt.OpContext, value *adt.Vertex) *adt.Bottom {
	if value == nil {
		return nil
	}
	p := newInference(ctx)
	defer p.enter()()
	return p.validateInterfaces(value)
}

func (p *inference) validateInterfaces(value *adt.Vertex) *adt.Bottom {
	ctx := p.ctx
	r := &relevanceChecker{p: p}
	if err := r.description(value, make(map[adt.Value]bool)); err != nil {
		source := err.source
		if source == nil {
			source = value
		}
		return &adt.Bottom{Src: source.Source(), Code: adt.BlockedError,
			Err: ctx.NewPosf(adt.Pos(source), "%s", err)}
	}
	return nil
}

func (r *relevanceChecker) description(value adt.Value, seen map[adt.Value]bool) (failure *RelevanceError) {
	defer func() {
		if failure != nil && failure.source == nil {
			failure.source = value
		}
	}()
	if value == nil {
		return r.blocked("nested interface remains unresolved")
	}
	if seen[value] {
		return nil
	}
	seen[value] = true
	if vertex, ok := value.(*adt.Vertex); ok && r.p.ctx.Propagation != nil {
		r.p.ctx.Propagation.Observe(vertex)
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.Bottom:
		// Refuted enclosing data removes the observation, including an
		// impossible optional field. Its data error is reported separately.
		return nil
	case *adt.FuncValue:
		clauses := x.ExplicitClauses()
		if len(clauses) == 0 {
			return nil // An inferred view is not a new source declaration.
		}
		prepared, err := r.declarations(clauses)
		if err != nil {
			return err
		}
		if err := r.arrows(prepared); err != nil {
			return err
		}
		for i, source := range clauses {
			for _, param := range adt.FunctionTypeParameters(source) {
				env := adt.TypeParameterScope(prepared[i].Env, param)
				if err := r.description(r.p.schema(env, param.Bound), seen); err != nil {
					return err
				}
			}
		}
		// Only the rigidly opened declarations introduce nested roots.
		// Sibling-anchored instances remain derived observation evidence.
		for _, clause := range prepared[:len(clauses)] {
			for _, param := range clause.Fn.Params {
				if err := r.description(r.p.schema(clause.Env, param.Value), seen); err != nil {
					return err
				}
			}
			if err := r.description(r.p.schema(clause.Env, clause.Fn.Ret), seen); err != nil {
				return err
			}
		}
	case *adt.Universal:
		return r.template(x.Template, x.Env, seen)

	case *adt.Disjunction:
		for _, branch := range x.Values {
			if err := r.description(branch, seen); err != nil {
				return err
			}
		}
	case *adt.Conjunction:
		for _, term := range x.Values {
			if err := r.description(term, seen); err != nil {
				return err
			}
		}
	case *adt.Vertex:
		for _, field := range x.Arcs {
			if field.Label.IsLet() {
				continue
			}
			field.Finalize(r.p.ctx)
			if err := r.description(field, seen); err != nil {
				return err
			}
		}
		if x.IsList() && !x.IsClosedList() {
			if !r.p.step() {
				return r.blocked("")
			}
			n := int64(0)
			for range x.Elems() {
				n++
			}
			tail := &adt.Vertex{Parent: x, Label: adt.MakeIntLabel(adt.IntLabel, n)}
			x.MatchAndInsert(r.p.ctx, tail)
			tail.Finalize(r.p.ctx)
			return r.description(tail, seen)
		}
	}
	return nil
}

func (r *relevanceChecker) template(q *adt.Quantified, outer *adt.Environment, seen map[adt.Value]bool) *RelevanceError {
	if !r.p.step() {
		return r.blocked("")
	}
	clause := adt.FuncType{Fn: &adt.Function{}, Env: q.CheckingScope(r.p.ctx, outer)}
	for _, param := range q.Params {
		bound := r.p.schema(clause.Env, param.Bound)
		if bound == nil {
			return r.blocked("quantifier bound remains unresolved")
		}
		if err := r.description(bound, seen); err != nil {
			return err
		}
		clause = adt.BindFunctionTypes(clause, []adt.Value{&adt.RigidType{Param: param, Bound: bound}})
	}
	body := r.p.schema(clause.Env, q.Body)
	if body == nil {
		return r.blocked("quantified interface remains unresolved")
	}
	return r.description(body, seen)
}
