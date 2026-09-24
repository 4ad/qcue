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

type transportKind uint8

const (
	transportResidual transportKind = iota
	transportIdentityValue
	transportCarrier
	transportCallable
	transportComposite
	transportChoice
	transportExistential
)

// transportPlan is the common interpretation of a scoped public schema for
// execution and certification. All child schemas are resolved by the evaluator
// in their own scopes, including list tails and pattern constraints. The
// executor never reconstructs lexical frames from syntax.
//
// total proves that every admitted source has a transport. A non-total plan
// may still transport a particular input (for example an unambiguous union
// branch), but that success cannot certify the whole operation.
type transportPlan struct {
	owner    *sealedPackage
	schema   Value
	outward  bool
	kind     transportKind
	total    bool
	identity bool
	carrier  *opaqueCarrier
	function *FuncValue
	fields   map[Feature]*transportPlan
	branches []*transportPlan
	sources  []Value
}

type operationTransport struct {
	inputs []*transportPlan
	result *transportPlan
}

func (s *OpaqueCall) planOperation(c *OpContext, public *Environment) operationTransport {
	var plan operationTransport
	for _, param := range s.signature.Params {
		plan.inputs = append(plan.inputs, s.owner.planTransport(c, scopedPredicate{public, param.Value}, !s.outward))
	}
	plan.result = s.owner.planTransport(c, scopedPredicate{public, s.signature.Ret}, s.outward)
	return plan
}

func (plan operationTransport) total() bool {
	if !plan.result.total {
		return false
	}
	for _, input := range plan.inputs {
		if !input.total {
			return false
		}
	}
	return true
}

func (p *sealedPackage) planTransport(c *OpContext, schema scopedPredicate, outward bool) *transportPlan {
	var value Value = &Top{}
	complete := true
	if schema.expr != nil {
		value, complete = c.Evaluate(schema.env, schema.expr)
	}
	if !complete || value == nil {
		return &transportPlan{owner: p, schema: value, outward: outward}
	}
	return p.compileTransport(c, value, outward, make(map[Value]bool))
}

func (p *sealedPackage) compileTransport(c *OpContext, value Value, outward bool, active map[Value]bool) *transportPlan {
	plan := &transportPlan{owner: p, schema: value, outward: outward}
	if value == nil || active[value] {
		return plan
	}
	active[value] = true
	defer delete(active, value)
	if v, ok := value.(*Vertex); ok {
		v.Finalize(c)
		if v.Bottom() != nil {
			return plan
		}
		value = v.DerefValue()
		plan.schema = value
	}
	if carrier := opaqueTypeOf(value); carrier != nil && carrier.owner == p {
		plan.kind, plan.carrier, plan.total = transportCarrier, carrier, true
		return plan
	}
	switch v := Unwrap(value).(type) {
	case *Top, *BasicType, *Null, *Bool, *Num, *String, *Bytes, *BoundValue:
		plan.kind, plan.total, plan.identity = transportIdentityValue, true, true
	case *RigidType:
		if !abstractEscapes(c, v.Bound, p, make(map[Value]bool)) {
			plan.kind, plan.total, plan.identity = transportIdentityValue, true, true
		}
	case *Existential:
		plan.kind = transportExistential
		plan.total = len(v.Template.References) == 0
		if vertex, ok := value.(*Vertex); ok && (len(vertex.Arcs) != 0 || vertex.PatternConstraints != nil) {
			plan.total = false
		}
	case *Conjunction:
		if !abstractEscapes(c, v, p, make(map[Value]bool)) {
			plan.kind, plan.total, plan.identity = transportIdentityValue, true, true
			for _, term := range v.Values {
				x := p.compileTransport(c, term, outward, active)
				plan.total = plan.total && x.total
				plan.identity = plan.identity && x.identity
			}
			if !plan.identity {
				plan.kind, plan.total = transportResidual, false
			}
		}
	case *Disjunction:
		plan.kind, plan.total, plan.identity = transportChoice, true, true
		var kinds Kind
		for _, branch := range v.Values {
			x := p.compileTransport(c, branch, outward, active)
			plan.branches = append(plan.branches, x)
			source := branch
			if outward {
				var b *Bottom
				source, b = p.privatePredicate(c, branch, make(map[Value]bool))
				if b != nil {
					source = nil
				}
			}
			plan.sources = append(plan.sources, source)
			plan.identity = plan.identity && x.identity
			if source == nil {
				plan.total = false
			} else {
				plan.total = plan.total && x.total && kinds&source.Kind() == 0
				kinds |= source.Kind()
			}
		}
		if plan.identity {
			plan.total = true // All arms perform exactly the same operation.
		}
	case *FuncValue:
		plan.kind, plan.function, plan.total = transportCallable, v, true
		clauses := v.selectionAndOriginalClauses()
		boundary := &OpaqueCall{owner: p, clauses: clauses, outward: outward}
		plan.total = boundary.coherent(c)
		for _, t := range clauses {
			if len(typeParameters(t.Env)) != 0 || t.Fn.Open {
				plan.total = false
				continue
			}
			for _, param := range t.Fn.Params {
				var x Value = &Top{}
				complete := true
				if param.Value != nil {
					x, complete = c.Evaluate(t.Env, param.Value)
				}
				plan.total = plan.total && complete && p.compileTransport(c, x, !outward, active).total
			}
			var result Value = &Top{}
			complete := true
			if t.Fn.Ret != nil {
				result, complete = c.Evaluate(t.Env, t.Fn.Ret)
			}
			plan.total = plan.total && complete && p.compileTransport(c, result, outward, active).total
		}
	case *Vertex:
		if v.Kind()&(StructKind|ListKind) == 0 {
			return plan
		}
		plan.kind, plan.total, plan.identity = transportComposite, true, true
		plan.fields = make(map[Feature]*transportPlan)
		for _, arc := range v.Arcs {
			if arc.Label.IsLet() {
				continue
			}
			x := p.compileTransport(c, arc, outward, active)
			plan.fields[arc.Label] = x
			plan.total = plan.total && x.total
			plan.identity = plan.identity && x.identity
		}
		if !v.IsClosedList() && v.IsList() {
			label := MakeIntLabel(IntLabel, int64(len(v.Arcs)))
			x := p.compileTransport(c, plan.fieldSchema(c, label), outward, active)
			plan.total = plan.total && x.total
			plan.identity = plan.identity && x.identity
		}
		if pc := v.PatternConstraints; pc != nil {
			for _, pair := range pc.Pairs {
				x := p.compileTransport(c, pair.Constraint, outward, active)
				plan.total = plan.total && x.total
				plan.identity = plan.identity && x.identity
			}
		}
	}
	return plan
}

func (plan *transportPlan) field(c *OpContext, label Feature) *transportPlan {
	if child := plan.fields[label]; child != nil {
		return child
	}
	return plan.owner.compileTransport(c, plan.fieldSchema(c, label), plan.outward, make(map[Value]bool))
}

func (plan *transportPlan) fieldSchema(c *OpContext, label Feature) *Vertex {
	v := plan.schema.(*Vertex)
	field := c.newInlineVertex(nil, nil)
	field.Label = label
	v.MatchAndInsert(c, field)
	field.Finalize(c)
	return field
}

func (plan *transportPlan) apply(c *OpContext, value Value, project bool, export *publicExport) Value {
	if b, ok := Unwrap(value).(*Bottom); ok {
		return b
	}
	if packageValue := plan.owner.transportPackage(c, nil, plan.schema, value); packageValue != nil {
		return packageValue
	}
	if plan.identity && !project {
		// This is graph identity, including predicates and future
		// refinements. Rebuilding the present fields would weaken it.
		return value
	}
	switch plan.kind {
	case transportIdentityValue:
		return value
	case transportCarrier:
		if !plan.outward {
			if v, ok := Unwrap(value).(*OpaqueValue); ok && v.carrier == plan.carrier {
				return v.private
			}
			return c.NewErrf("argument belongs to a different abstract type")
		}
		if !concreteCapture(c, value) {
			return &Bottom{Code: IncompleteError, Err: c.Newf("incomplete private representation")}
		}
		return &OpaqueValue{carrier: plan.carrier, private: value}
	case transportCallable:
		return plan.owner.transportFunction(c, plan.function, value, plan.outward, export.canonical())
	case transportChoice:
		return plan.applyChoice(c, value, project, export)
	case transportComposite:
		return plan.applyComposite(c, value, project, export)
	case transportExistential:
		if plan.total {
			return value
		}
	}
	return &Bottom{Code: IncompleteError, Err: c.Newf("opaque transport schema remains unresolved")}
}

func (plan *transportPlan) applyChoice(c *OpContext, value Value, project bool, export *publicExport) Value {
	var result Value
	unknown := false
	for i, branch := range plan.branches {
		source := plan.sources[i]
		if source == nil {
			unknown = true
			continue
		}
		switch capabilityMember(c, nil, source, value) {
		case proofRefuted:
			continue
		case proofUnknown:
			unknown = true
			continue
		}
		x := branch.apply(c, value, project, export)
		if _, failed := Unwrap(x).(*Bottom); failed || !concreteCapture(c, x) {
			unknown = true
			continue
		}
		if result != nil && !Equal(c, result, x, CheckStructural) {
			unknown = true
		}
		result = x
	}
	if unknown {
		return &Bottom{Code: IncompleteError, Err: c.Newf("opaque union transport remains ambiguous or unresolved")}
	}
	if result == nil {
		return c.NewErrf("value does not match any opaque interface union branch")
	}
	return result
}

func (plan *transportPlan) applyComposite(c *OpContext, value Value, project bool, export *publicExport) Value {
	v, ok := value.(*Vertex)
	if !ok {
		return c.NewErrf("interface requires a composite value")
	}
	v.Finalize(c)
	v = v.DerefValue()
	typ := plan.schema.(*Vertex)
	if typ.IsList() {
		out := &ListLit{}
		for a := range v.Elems() {
			out.Elems = append(out.Elems, plan.field(c, a.Label).apply(c, a, false, export.field(a.Label)))
		}
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, out))
		result.Finalize(c)
		return result
	}
	out := &StructLit{}
	for _, field := range typ.Arcs {
		if field.Label.IsLet() {
			continue
		}
		if field.Label.IsDef() {
			var predicate Value = field
			if !plan.outward {
				var b *Bottom
				predicate, b = plan.owner.privatePredicate(c, field, make(map[Value]bool))
				if b != nil {
					return b
				}
			}
			out.Decls = append(out.Decls, &Field{Label: field.Label, ArcType: field.ArcType, Value: predicate})
			continue
		}
		a := v.LookupRaw(field.Label)
		if a == nil || a.ArcType != ArcMember {
			if field.ArcType == ArcOptional {
				continue
			}
			return c.NewErrf("missing interface field %s", field.Label.SelectorString(c))
		}
		out.Decls = append(out.Decls, &Field{Label: field.Label,
			Value: plan.field(c, field.Label).apply(c, a, false, export.field(field.Label))})
	}
	for _, a := range v.Arcs {
		if a.Label.IsLet() || typ.LookupRaw(a.Label) != nil {
			continue
		}
		if a.ArcType != ArcMember || !a.Label.IsRegular() {
			if !project {
				out.Decls = append(out.Decls, &Field{Label: a.Label, ArcType: a.ArcType, Value: a})
			}
			continue
		}
		field := c.newInlineVertex(nil, nil)
		field.Label = a.Label
		typ.MatchAndInsert(c, field)
		if len(field.Conjuncts) == 0 {
			if !project {
				out.Decls = append(out.Decls, &Field{Label: a.Label, ArcType: a.ArcType, Value: a})
			}
			continue
		}
		out.Decls = append(out.Decls, &Field{Label: a.Label,
			Value: plan.field(c, a.Label).apply(c, a, false, export.field(a.Label))})
	}
	result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, out))
	result.Finalize(c)
	return result
}
