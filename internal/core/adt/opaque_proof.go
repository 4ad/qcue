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

// ProofView eliminates an arbitrary admitted package under a fresh abstract
// carrier. It exposes its interface hypotheses, never a representation or
// an executable package witness. Extra interface constraints may be omitted:
// proving the body for this wider view is sufficient for every refinement.
func (o *PackageOpen) ProofView(c *OpContext, value Value) (*OpaqueType, Value) {
	if value == nil {
		return nil, nil
	}
	e := proofInterface(c, value)
	if e == nil || len(e.Template.Params) != 1 {
		return nil, nil
	}
	param := e.Template.Params[0]
	if param.Bound != nil || param.ValueRange != nil {
		return nil, nil
	}
	p := &sealedPackage{interfaceType: e}
	typ := &OpaqueType{carrier: &opaqueCarrier{owner: p, parameter: param, level: param.Level}}
	env := quantifiedEnvironment(c, e, map[*TypeParameter]Value{param: typ})
	view := c.newInlineVertex(nil, nil, MakeRootConjunct(env, e.Template.Body))
	view.Finalize(c)
	if view.Bottom() != nil {
		return nil, nil
	}
	return typ, view
}

func proofInterface(c *OpContext, value Value) *Existential {
	if union, ok := Unwrap(value).(*Disjunction); ok {
		var common *Existential
		for _, branch := range union.Values {
			e := proofInterface(c, branch)
			if e == nil || (common != nil && !common.SubsumesPackage(c, e)) {
				return nil
			}
			common = e
		}
		return common
	}
	if v, ok := value.(*Vertex); ok && v.DerefValue().sealed != nil {
		return v.DerefValue().sealed.interfaceType
	}
	return existentialOf(c, value, make(map[Expr]bool))
}

// Escapes checks that the result of a proof-level package elimination is
// independent of the fresh carrier, just as for runtime opening.
func (x *OpaqueType) Escapes(c *OpContext, value Value) bool {
	return abstractEscapes(c, value, x.carrier.owner, make(map[Value]bool))
}

// ProofTypes exposes the adapter's proof obligations to the conformance
// checker, without making its private implementation visible to clients or
// ordinary traversals. The transport theorem applies only to schemas whose
// transport is known to be total in the appropriate direction.
func (s *OpaqueCall) ProofTypes(c *OpContext, env *Environment) (advertised FuncType, implementation *FuncValue, required FuncType, ok bool) {
	if !s.coherent(c) {
		return advertised, nil, required, false
	}
	bindings := make(map[*TypeParameter]Value)
	for e := env; e != nil; e = e.Up {
		if e.types != nil {
			for param, value := range e.types.arguments {
				if s.owner.carriers[param] == nil {
					bindings[param] = value
				}
			}
		}
	}
	public := instantiateEnvironment(s.env, bindings)
	private, b := s.owner.privateTypeEnvironment(c, public, make(map[Value]bool))
	if b != nil {
		return advertised, nil, required, false
	}
	for _, param := range s.signature.Params {
		if !s.owner.totalTransport(c, public, param.Value, !s.outward, make(map[Value]bool)) {
			return advertised, nil, required, false
		}
	}
	if !s.owner.totalTransport(c, public, s.signature.Ret, s.outward, make(map[Value]bool)) {
		return advertised, nil, required, false
	}
	advertised = FuncType{Fn: s.signature, Env: public}
	required = FuncType{Fn: s.signature, Env: private}
	if !s.outward {
		advertised, required = required, advertised
	}
	return advertised, s.private, required, true
}

func (p *sealedPackage) totalTransport(c *OpContext, env *Environment, schema Expr, outward bool, active map[Value]bool) bool {
	if schema == nil {
		return true
	}
	value, complete := c.Evaluate(env, schema)
	if !complete || value == nil {
		return false
	}
	if v, ok := value.(*Vertex); ok {
		v.Finalize(c)
		value = v.DerefValue()
		if _, ok := Unwrap(value).(*Existential); ok &&
			(len(v.Arcs) != 0 || v.PatternConstraints != nil) {
			// Refinements of an existential may borrow the outer carrier.
			return false
		}
	}
	value = Unwrap(value)
	if active[value] {
		return false
	}
	active[value] = true
	defer delete(active, value)
	switch v := value.(type) {
	case *Top, *BasicType, *Null, *Bool, *Num, *String, *Bytes, *BoundValue:
		return true
	case *OpaqueType:
		return v.carrier.owner == p
	case *RigidType:
		return !abstractEscapes(c, v.Bound, p, make(map[Value]bool))
	case *Existential:
		// A closed interface with no free predicate dependencies cannot
		// mention this boundary's carrier. Its packages pass unchanged.
		return len(v.Template.References) == 0
	case *Conjunction:
		if abstractEscapes(c, v, p, make(map[Value]bool)) {
			return false
		}
		for _, term := range v.Values {
			if !p.totalTransport(c, nil, term, outward, active) {
				return false
			}
		}
		return true
	case *Disjunction:
		var kinds Kind
		for _, branch := range v.Values {
			source := branch
			if outward {
				var b *Bottom
				source, b = p.privatePredicate(c, branch, make(map[Value]bool))
				if b != nil {
					return false
				}
			}
			// Disjoint source kinds suffice to select exactly one arm.
			// Overlapping arms require a stronger transport equivalence
			// proof and therefore remain residual here.
			if kinds&source.Kind() != 0 || !p.totalTransport(c, nil, branch, outward, active) {
				return false
			}
			kinds |= source.Kind()
		}
		return true
	case *FuncValue:
		clauses := v.selectionAndOriginalClauses()
		boundary := &OpaqueCall{owner: p, clauses: clauses, outward: outward}
		if !boundary.coherent(c) {
			return false
		}
		for _, t := range clauses {
			if len(typeParameters(t.Env)) != 0 || t.Fn.Open {
				return false
			}
			for _, param := range t.Fn.Params {
				if !p.totalTransport(c, t.Env, param.Value, !outward, active) {
					return false
				}
			}
			if !p.totalTransport(c, t.Env, t.Fn.Ret, outward, active) {
				return false
			}
		}
		return true
	case *Vertex:
		if v.Bottom() != nil || v.sealed != nil {
			return false
		}
		for _, arc := range v.Arcs {
			if !arc.Label.IsLet() && !p.totalTransport(c, nil, arc, outward, active) {
				return false
			}
		}
		if pc := v.PatternConstraints; pc != nil {
			for _, pair := range pc.Pairs {
				if !p.totalTransport(c, nil, pair.Constraint, outward, active) {
					return false
				}
			}
		}
		return true
	}
	return false
}
