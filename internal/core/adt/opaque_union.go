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

import "maps"

// transportUnion selects a branch in the source representation. Matching
// the public opaque predicate against private data would incorrectly discard
// that branch and could expose the representation through another union arm.
// If overlapping branches transport the value differently, the untagged
// interface does not determine a unique observation; keep it incomplete.
func (p *sealedPackage) transportUnion(c *OpContext, union *Disjunction, value Value, outward, project bool, export *publicExport) Value {
	if union.Kind()&^(NullKind|BoolKind|NumberKind|StringKind|BytesKind) == 0 {
		// Every branch uses identity transport. Preserve the disjunction
		// and its preferences instead of demanding a unique branch and
		// mistaking an ordinary default for an ambiguous representation.
		v := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, union), MakeRootConjunct(nil, value))
		v.Finalize(c)
		return v
	}
	var result Value
	unknown := false
	for _, branch := range union.Values {
		source := branch
		if outward {
			var b *Bottom
			source, b = p.privatePredicate(c, branch, make(map[Value]bool))
			if b != nil {
				unknown = true
				continue
			}
		}
		candidate := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, source), MakeRootConjunct(nil, value))
		candidate.Finalize(c)
		if b := candidate.Bottom(); b != nil {
			unknown = unknown || b.IsIncomplete()
			continue
		}
		// Keep the branch predicate on the candidate. In particular a
		// compatible closure descriptor does not prove its arrow contract.
		x := p.transportResolvedExport(c, branch, candidate, outward, project, export)
		if _, ok := Unwrap(x).(*Bottom); ok {
			unknown = true
			continue
		}
		if !concreteCapture(c, x) {
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

// privatePredicate substitutes representations in a resolved public schema.
// Re-evaluating original conjuncts preserves record correlations, patterns,
// and closedness instead of rebuilding a product of field approximations.
func (p *sealedPackage) privatePredicate(c *OpContext, value Value, active map[Value]bool) (Value, *Bottom) {
	if active[value] {
		return nil, &Bottom{Code: IncompleteError, Err: c.Newf("recursive opaque union predicate remains unresolved")}
	}
	active[value] = true
	defer delete(active, value)
	switch v := Unwrap(value).(type) {
	case *OpaqueType:
		if v.carrier.owner == p {
			return v.carrier.representation, nil
		}
	case *OpaqueValue:
		if v.carrier.owner == p {
			return v.private, nil
		}
	case *Disjunction:
		copy := *v
		copy.Values = nil
		for _, term := range v.Values {
			x, b := p.privatePredicate(c, term, active)
			if b != nil {
				return nil, b
			}
			copy.Values = append(copy.Values, x)
		}
		return &copy, nil
	case *Conjunction:
		copy := &Conjunction{}
		for _, term := range v.Values {
			x, b := p.privatePredicate(c, term, active)
			if b != nil {
				return nil, b
			}
			copy.Values = append(copy.Values, x)
		}
		return copy, nil
	case *FuncValue:
		copy := *v
		var b *Bottom
		copy.Env, b = p.privateTypeEnvironment(c, v.Env, active)
		if b != nil {
			return nil, b
		}
		copy.Types = nil
		for _, t := range v.Types {
			t.Env, b = p.privateTypeEnvironment(c, t.Env, active)
			if b != nil {
				return nil, b
			}
			copy.Types = append(copy.Types, t)
		}
		return &copy, nil
	case *Vertex:
		v.Finalize(c)
		if v.sealed != nil {
			return value, nil
		}
		var conjuncts []Conjunct
		for conj := range v.LeafConjuncts() {
			env, b := p.privateTypeEnvironment(c, conj.Env, active)
			if b != nil {
				return nil, b
			}
			x := conj.Expr()
			if val, ok := x.(Value); ok {
				x, b = p.privatePredicate(c, val, active)
				if b != nil {
					return nil, b
				}
			}
			conjuncts = append(conjuncts, MakeConjunct(env, x, conj.CloseInfo))
		}
		if len(conjuncts) == 0 {
			// Normalized disjuncts have no source conjuncts: their arcs
			// and pattern constraints define the complete predicate.
			// Copy those constraints, including presence and closedness,
			// rather than using a data snapshot that drops latent fields.
			copy := v.Clone()
			copy.Arcs = nil
			for _, arc := range v.Arcs {
				x, b := p.privatePredicate(c, arc, active)
				if b != nil {
					return nil, b
				}
				a := ToVertex(x).Clone()
				a.Parent, a.Label, a.ArcType = copy, arc.Label, arc.ArcType
				copy.Arcs = append(copy.Arcs, a)
			}
			if pc := v.PatternConstraints; pc != nil {
				constraints := *pc
				constraints.Pairs = nil
				for _, pair := range pc.Pairs {
					x, b := p.privatePredicate(c, pair.Constraint, active)
					if b != nil {
						return nil, b
					}
					pair.Constraint = ToVertex(x)
					constraints.Pairs = append(constraints.Pairs, pair)
				}
				copy.PatternConstraints = &constraints
			}
			return copy, nil
		}
		result := c.newInlineVertex(nil, nil, conjuncts...)
		result.Finalize(c)
		return result, nil
	}
	return value, nil
}

func (p *sealedPackage) privateTypeEnvironment(c *OpContext, env *Environment, active map[Value]bool) (*Environment, *Bottom) {
	if env == nil {
		return nil, nil
	}
	up, b := p.privateTypeEnvironment(c, env.Up, active)
	if b != nil {
		return nil, b
	}
	copy := *env
	copy.Up = up
	if scope := env.types; scope != nil {
		types := *scope
		types.arguments = maps.Clone(scope.arguments)
		for param, value := range scope.arguments {
			x, b := p.privatePredicate(c, value, active)
			if b != nil {
				return nil, b
			}
			types.arguments[param] = x
		}
		copy.types = &types
	}
	copy.cache = nil
	return &copy, nil
}
