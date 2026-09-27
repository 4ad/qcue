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

import "slices"

// ProofTypes checks a seal's explicit representation witnesses and returns
// its private interface obligation. No executable package is created;
// the caller must check the unchanged body against that obligation before
// introducing the existential result. The boundary must also transport every
// admitted value, rather than succeeding only on a particular implementation.
func (s *PackageSeal) ProofTypes(c *OpContext, e *Existential, witnesses []Value) Value {
	if e == nil || len(witnesses) != len(s.Names) || len(s.Names) != len(e.Template.Params) {
		return nil
	}
	p := &sealedPackage{interfaceType: e, carriers: make(map[*TypeParameter]*opaqueCarrier)}
	private, public := make(map[*TypeParameter]Value), make(map[*TypeParameter]Value)
	for _, param := range e.Template.Params {
		if param.Bound != nil || param.ValueRange != nil {
			return nil
		}
		i := slices.Index(s.Names, param.Src.Name.Name)
		if i < 0 || witnesses[i] == nil {
			return nil
		}
		value := witnesses[i]
		if b := param.checkWitness(c, quantifiedEnvironment(c, e, private), value); b != nil {
			return nil
		}
		carrier := &opaqueCarrier{owner: p, parameter: param, representation: value}
		p.carriers[param] = carrier
		private[param], public[param] = value, &OpaqueType{carrier: carrier}
	}
	p.privateEnv = quantifiedEnvironment(c, e, private)
	p.publicEnv = quantifiedEnvironment(c, e, public)
	if !p.planTransport(c, scopedPredicate{p.publicEnv, e.Template.Body}, true).total {
		return nil
	}
	value := c.newInlineVertex(nil, nil, MakeRootConjunct(p.privateEnv, e.Template.Body))
	value.Finalize(c)
	return value
}

// ProofView eliminates an arbitrary admitted package under a fresh abstract
// carrier. It exposes its interface hypotheses, never a representation or
// an executable package witness. Extra interface constraints may be omitted:
// proving the body for this wider view is sufficient for every refinement.
// Runtime members carry either an explicit seal or the constructive data
// witness used by Existential.validate and openDataWitness.
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
	p := &sealedPackage{interfaceType: e, carriers: make(map[*TypeParameter]*opaqueCarrier)}
	carrier := &opaqueCarrier{owner: p, parameter: param}
	p.carriers[param] = carrier
	typ := &OpaqueType{carrier: carrier}
	env := quantifiedEnvironment(c, e, map[*TypeParameter]Value{param: typ})
	view := c.newInlineVertex(nil, nil, MakeRootConjunct(env, e.Template.Body))
	view.Finalize(c)
	if view.Bottom() != nil {
		return nil, nil
	}
	if covariantData(e.Template.Body) {
		// This domain also admits transparent records. Their membership
		// rule must construct a witness whose view is executable for all
		// admitted inputs, not just for a successful trial value. Ambiguous
		// union transports cannot justify an elimination theorem.
		witness, b := e.dataWitness(c)
		if b != nil {
			return nil, nil
		}
		carrier.representation = witness.arguments[param]
		if !p.planTransport(c, scopedPredicate{expr: view}, true).total {
			return nil, nil
		}
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
	plan := s.planOperation(c, public)
	if !plan.total() {
		return advertised, nil, required, false
	}
	advertised = FuncType{Fn: s.signature, Env: public}
	required = FuncType{Fn: s.signature, Env: private}
	if !s.outward {
		advertised, required = required, advertised
	}
	return advertised, s.private, required, true
}
