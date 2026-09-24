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
	conjuncts := []Conjunct{MakeRootConjunct(nil, result), MakeRootConjunct(nil, constraint)}
	if patterns := plan.identityPatterns(c, source); patterns != nil {
		// Keep observable predicates on fields where transport is the
		// identity. The inverse-image check retains their denotation but
		// cannot by itself expose their lexical dependencies to scope and
		// universe checking, or instantiate their label aliases later.
		conjuncts = append(conjuncts, MakeRootConjunct(nil, membershipOperand(patterns)))
	}
	out := c.newInlineVertex(nil, nil, conjuncts...)
	out.Finalize(c)
	return out
}

// identityPatterns retains each source pattern outside the fields and
// patterns whose plans change representation. Constraints within the changed
// region are checked by the inverse-image predicate. This partition uses the
// same compiled child plans as execution, not a second schema interpretation.
func (plan *transportPlan) identityPatterns(c *OpContext, source Value) *Vertex {
	v, ok := Unwrap(source).(*Vertex)
	if !ok || plan.kind != transportComposite || v.IsList() || v.PatternConstraints == nil {
		return nil
	}
	var changed []Value
	for _, field := range plan.schema.(*Vertex).Arcs {
		if field.Label.IsRegular() && !plan.fields[field.Label].identity {
			changed = append(changed, field.Label.ToValue(c))
		}
	}
	if pc := plan.schema.(*Vertex).PatternConstraints; pc != nil {
		for _, pair := range pc.Pairs {
			if !plan.owner.compileTransport(c, pair.Constraint, plan.outward, make(map[Value]bool)).identity {
				changed = append(changed, pair.Pattern)
			}
		}
	}
	patterns := &Constraints{}
sourcePattern:
	for _, pair := range v.PatternConstraints.Pairs {
		terms := []Value{pair.Pattern}
		for _, excluded := range changed {
			if c.provesInclusion(excluded, pair.Pattern) {
				continue sourcePattern
			}
			if label, ok := Unwrap(excluded).(*String); ok {
				terms = append(terms, &BoundValue{Op: NotEqualOp, Value: label})
			} else {
				terms = append(terms, &TransportPatternExclusion{pattern: excluded})
			}
		}
		pair.Pattern = &Conjunction{Values: terms}
		patterns.Pairs = append(patterns.Pairs, pair)
	}
	if len(patterns.Pairs) == 0 {
		return nil
	}
	return &Vertex{BaseValue: &StructMarker{}, PatternConstraints: patterns}
}

// TransportPatternExclusion is the complement of a schema's field-label
// predicate, used only to partition retained patterns. It is queried on a
// concrete field label; an unresolved match never establishes its complement.
type TransportPatternExclusion struct{ pattern Value }

func (x *TransportPatternExclusion) Source() ast.Node         { return x.pattern.Source() }
func (*TransportPatternExclusion) node()                      {}
func (*TransportPatternExclusion) expr()                      {}
func (*TransportPatternExclusion) declNode()                  {}
func (*TransportPatternExclusion) elemNode()                  {}
func (*TransportPatternExclusion) Kind() Kind                 { return StringKind }
func (*TransportPatternExclusion) Concreteness() Concreteness { return Constraint }

func (x *TransportPatternExclusion) validate(c *OpContext, value Value) *Bottom {
	// The supplied label already carries this exclusion. Replaying its
	// root validators would ask the very same question recursively.
	check := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, x.pattern), MakeRootConjunct(nil, membershipOperand(value)))
	check.Finalize(c)
	if b := check.Bottom(); b != nil {
		if b.IsIncomplete() {
			return b
		}
		return nil
	}
	if !IsConcrete(check) {
		return &Bottom{Code: IncompleteError, Err: c.Newf("transport pattern match remains unresolved")}
	}
	return c.NewErrf("field is covered by a changing transport")
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
