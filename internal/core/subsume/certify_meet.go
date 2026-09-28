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

// A declared local field remains refinable by other record conjuncts. Its
// initializer's exact constructor inventory therefore cannot become a
// capture invariant. Retain its declared fields and callable evidence, but
// not the synthetic closedness used for a direct constructor result.
func (p *inference) bindingDescription(value adt.Value) adt.Value {
	if value == nil || !p.step() {
		return nil
	}
	switch x := value.(type) {
	case *adt.Disjunction:
		out := *x
		out.Values = nil
		for _, branch := range x.Values {
			v := p.bindingDescription(branch)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return &out
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			v := p.bindingDescription(term)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return out
	}
	v, ok := value.(*adt.Vertex)
	if !ok {
		return value
	}
	result := value
	if p.constructors[v] != nil {
		result = p.sourceDescription(v)
	}
	if out, ok := result.(*adt.Vertex); ok && p.projections[v] != nil {
		fields := make(map[adt.Feature]adt.Value)
		for label, field := range p.projections[v] {
			fields[label] = p.bindingDescription(field)
			if fields[label] == nil {
				return nil
			}
		}
		p.projections[out] = fields
	}
	return result
}

// Source unification combines constructor descriptions before assigning the
// result a static field inventory. Exact inventories used for width and
// presence proofs must not make {a: 1} & {b: true} an empty computation.
func (p *inference) sourceMeet(a, b adt.Value) adt.Value {
	left, right := p.sourceDescription(a), p.sourceDescription(b)
	value := p.eagerMeet(left, right)
	if value == nil || refuted(value) {
		return value
	}
	// A successful executable assertion establishes membership in the
	// named live coordinate. Normalizing its current data must not erase
	// that relation: its upper approximation alone cannot prove the same
	// result obligation after another refinement.
	for _, operand := range []adt.Value{a, b} {
		if live, ok := adt.Unwrap(operand).(*adt.LiveType); ok {
			p.memberships[value] = append(p.memberships[value], live)
		}
		p.memberships[value] = append(p.memberships[value], p.memberships[operand]...)
	}
	if builtin, ok := adt.Unwrap(value).(*adt.Builtin); ok {
		if ValidateBuiltin(p.ctx, builtin) != nil {
			return nil
		}
		return builtin
	}
	if f, ok := adt.Unwrap(value).(*adt.FuncValue); ok {
		// A function meet collects obligations on one implementation. Its
		// components cannot be used as independent alternatives that drop an
		// inconvenient contract when selecting a call domain.
		if f.Fn.Body != nil && !p.implementation(f) {
			return nil
		}
		p.assume(f, make(map[adt.Value]bool))
		return f
	}
	if v, ok := value.(*adt.Vertex); ok && v.Bottom() == nil && v.Kind()&(adt.StructKind|adt.ListKind) != 0 {
		fields := make(map[adt.Feature]adt.Value)
		for _, field := range v.Arcs {
			if field.ArcType != adt.ArcMember && field.ArcType != adt.ArcRequired {
				continue
			}
			x, y := p.project(a, field.Label), p.project(b, field.Label)
			switch {
			case x != nil && y != nil:
				fields[field.Label] = p.sourceMeet(x, y)
			case x != nil:
				fields[field.Label] = x
			case y != nil:
				fields[field.Label] = y
			default:
				fields[field.Label] = field
			}
			if fields[field.Label] == nil {
				return nil
			}
		}
		summary := v
		av, aok := a.(*adt.Vertex)
		bv, bok := b.(*adt.Vertex)
		if v.Kind() == adt.StructKind && aok && bok && p.constructors[av] != nil && p.constructors[bv] != nil {
			// Two complete constructor inventories give an exact merged
			// inventory. Retain its open description for subsequent meets.
			summary = v.ToDataSingle()
			summary.ClosedNonRecursive = true
			p.constructors[summary] = v
		}
		p.projections[summary] = fields
		p.memberships[summary] = p.memberships[value]
		return summary
	}
	if v, ok := value.(*adt.Vertex); ok && v.HasSubjectSchemes() {
		// A normalized scalar can still carry a quantified introduction.
		// Keep that subject evidence for subsequent explicit selection.
		return v
	}
	// Preserve scoped callback hypotheses and rigid predicates when the
	// meet is symbolic rather than a normalized composite description.
	return p.schema(nil, &adt.Conjunction{Values: []adt.Value{a, b}})
}

func (p *inference) sourceDescription(value adt.Value) adt.Value {
	if value == nil || !p.step() {
		return nil
	}
	switch x := value.(type) {
	case *adt.Vertex:
		switch source := p.constructors[x].(type) {
		case *adt.StructLit:
			out := &adt.StructLit{}
			for _, decl := range source.Decls {
				f, ok := decl.(*adt.Field)
				if !ok {
					// Pattern constraints have already been checked and do not
					// contribute a present field's construction inventory.
					out.Decls = append(out.Decls, decl)
					continue
				}
				field := *f
				field.Value = p.sourceDescription(field.Value.(adt.Value))
				if field.Value == nil {
					return nil
				}
				out.Decls = append(out.Decls, &field)
			}
			return p.schema(nil, out)
		case *adt.ListLit:
			out := &adt.ListLit{}
			for _, elem := range source.Elems {
				if tail, ok := elem.(*adt.Ellipsis); ok {
					value := p.sourceDescription(tail.Value.(adt.Value))
					if value == nil {
						return nil
					}
					out.Elems = append(out.Elems, &adt.Ellipsis{Value: value})
				} else {
					value := p.sourceDescription(elem.(adt.Value))
					if value == nil {
						return nil
					}
					out.Elems = append(out.Elems, value)
				}
			}
			return p.schema(nil, out)
		case adt.Value:
			return source
		}
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			value := p.sourceDescription(term)
			if value == nil {
				return nil
			}
			out.Values = append(out.Values, value)
		}
		return out
	case *adt.Disjunction:
		out := *x
		out.Values = nil
		for _, branch := range x.Values {
			value := p.sourceDescription(branch)
			if value == nil {
				return nil
			}
			out.Values = append(out.Values, value)
		}
		return &out
	}
	return value
}

// A literal arrow in a source meet is an annotation of the other operand.
// It cannot introduce an executable import hypothesis by itself.
func (p *inference) functionAnnotation(env *adt.Environment, expr adt.Expr) (adt.Value, bool) {
	body := expr
	if q, ok := body.(*adt.Quantified); ok {
		body = q.Body
	}
	f, ok := body.(*adt.Function)
	if !ok || f.Body != nil {
		return nil, false
	}
	if !p.typeOperations(env, expr) {
		return nil, true
	}
	return p.schema(env, expr), true
}

func (p *inference) assertFunction(value, annotation adt.Value) adt.Value {
	if value == nil || annotation == nil || value.Kind() != adt.FuncKind {
		return nil
	}
	// sourceMeet collects the obligations on the supplied implementation
	// and checks its unchanged body before exposing any result clause.
	return p.sourceMeet(value, annotation)
}
