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

package adt

import "cuelang.org/go/cue/ast"

// TransportConstraint retains the inverse image of an original constraint
// graph. Rebuilding a transported record's current fields is not enough:
// optional predicates, patterns, closedness, and correlations constrain later
// refinements too. The source graph stays behind this authorized boundary.
//
// This is a predicate, not an inhabitant or a proof of total transport.
type TransportConstraint struct {
	plan   *transportPlan
	source Value
	target Value
}

func (t *TransportConstraint) Source() ast.Node         { return t.source.Source() }
func (*TransportConstraint) node()                      {}
func (*TransportConstraint) expr()                      {}
func (*TransportConstraint) declNode()                  {}
func (*TransportConstraint) elemNode()                  {}
func (t *TransportConstraint) Kind() Kind               { return t.target.Kind() }
func (*TransportConstraint) Concreteness() Concreteness { return Constraint }

func (t *TransportConstraint) validate(c *OpContext, value Value) *Bottom {
	if !concreteCapture(c, value) {
		return &Bottom{Code: IncompleteError, Err: c.Newf("transported predicate requires a complete subject")}
	}
	if c.transportChecks[t] {
		return &Bottom{Code: IncompleteError, Err: c.Newf("recursive transport constraint remains unresolved")}
	}
	if c.transportChecks == nil {
		c.transportChecks = make(map[*TransportConstraint]bool)
	}
	c.transportChecks[t] = true
	defer delete(c.transportChecks, t)
	inverse := t.plan.owner.planTransport(c, scopedPredicate{expr: t.plan.schema}, !t.plan.outward)
	// This inverse is a checking operand. It does not install another
	// transport predicate referring back to this very obligation.
	original := inverse.transform(c, value, false, nil, false)
	if b, ok := Unwrap(original).(*Bottom); ok {
		return b
	}
	check := c.newInlineVertex(nil, nil,
		MakeRootConjunct(nil, t.source), MakeRootConjunct(nil, original))
	check.Finalize(c)
	if b := check.Bottom(); b != nil {
		copy := *b
		copy.ChildError, copy.HasRecursive = false, false
		return &copy
	}
	if capabilityHasCallable(check, make(map[Value]bool)) && (c.CheckFunction == nil || c.CheckBuiltin == nil) {
		return &Bottom{Code: IncompleteError, Err: c.Newf("transported callable predicate remains unproved")}
	}
	return Validate(c, check, &ValidateConfig{Concrete: true, Final: true, Runtime: true,
		CheckFunction: c.CheckFunction, CheckBuiltin: c.CheckBuiltin})
}

func (plan *transportPlan) destination(c *OpContext) (Value, *Bottom) {
	target := plan.schema
	if target == nil {
		return nil, &Bottom{Code: IncompleteError, Err: c.Newf("transport predicate schema remains unresolved")}
	}
	if !plan.outward {
		var b *Bottom
		target, b = plan.owner.privatePredicate(c, target, make(map[Value]bool))
		if b != nil {
			return nil, b
		}
	}
	return target, nil
}

func (plan *transportPlan) retain(c *OpContext, source, result Value) Value {
	target, b := plan.destination(c)
	if b != nil {
		return b
	}
	constraint := &TransportConstraint{plan: plan, source: source, target: target}
	out := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, result), MakeRootConjunct(nil, constraint))
	out.Finalize(c)
	return out
}

// predicate transports a schema occurrence, such as a definition or an absent
// optional field. It never demands a concrete witness for that occurrence.
// Projectable fields retain their own predicates as well as the whole graph's
// inverse image; projecting a definition must not lose its narrower domain.
func (plan *transportPlan) predicate(c *OpContext, source Value) Value {
	target, b := plan.destination(c)
	if b != nil {
		return b
	}
	if source == nil {
		return target
	}
	if plan.identity {
		return source
	}
	if plan.kind == transportCarrier && !plan.outward {
		private, b := plan.owner.privatePredicate(c, source, make(map[Value]bool))
		if b != nil {
			return b
		}
		return private
	}
	if plan.kind != transportComposite {
		return plan.retain(c, source, target)
	}
	v, ok := Unwrap(source).(*Vertex)
	if !ok {
		return plan.retain(c, source, target)
	}
	v.Finalize(c)
	typ := plan.schema.(*Vertex)
	var shape Expr
	if v.IsList() && typ.IsList() {
		out := &ListLit{}
		count := int64(0)
		for a := range v.Elems() {
			out.Elems = append(out.Elems, plan.field(c, a.Label).predicate(c, a))
			count++
		}
		if !v.IsClosedList() {
			label := MakeIntLabel(IntLabel, count)
			tail := c.newInlineVertex(nil, nil)
			tail.Label = label
			v.MatchAndInsert(c, tail)
			tail.Finalize(c)
			out.Elems = append(out.Elems, &Ellipsis{Value: plan.field(c, label).predicate(c, tail)})
		}
		shape = out
	} else if v.Kind() == StructKind && typ.Kind() == StructKind {
		out := &StructLit{}
		for _, field := range v.Arcs {
			if !field.Label.IsLet() {
				out.Decls = append(out.Decls, &Field{Label: field.Label, ArcType: field.ArcType,
					Value: plan.field(c, field.Label).predicate(c, field)})
			}
		}
		shape = out
	} else {
		return plan.retain(c, source, target)
	}
	result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, shape), MakeRootConjunct(nil, target))
	result.Finalize(c)
	return plan.retain(c, source, result)
}
