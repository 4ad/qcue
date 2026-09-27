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

// scopedPredicate is a proposition in its declaration scope. It is not a
// runtime witness, even when its evaluation has a concrete approximation.
type scopedPredicate struct {
	env  *Environment
	expr Expr
}

func mergeCallWitnesses(a, b []*Existential) []*Existential {
	for _, witness := range b {
		if !slices.ContainsFunc(a, func(e *Existential) bool {
			return e.Template == witness.Template && e.Env == witness.Env
		}) {
			a = append(slices.Clone(a), witness)
		}
	}
	return a
}

func (v *Vertex) addCallWitness(c *OpContext, predicate scopedPredicate) {
	if predicate.expr == nil {
		return
	}
	if e := existentialExprOf(c, predicate.env, predicate.expr, make(map[Expr]bool)); e != nil {
		v.callWitnesses = mergeCallWitnesses(v.callWitnesses, []*Existential{e})
	}
}

// HasCallWitnesses reports boundary proof provenance, including interfaces
// invalidated by later refinement. Export must use the independently computed
// value even when none of the old witnesses can be reconstructed anymore.
func (v *Vertex) HasCallWitnesses() bool { return len(v.callWitnesses) != 0 }

// CallWitnesses returns existential interfaces established at an execution
// boundary that still hold after refinement. They are proof metadata, not
// constraints on future refinements.
// Source export can re-establish them with an identity call, preserving both
// elimination and the original runtime value without conjoining a schema.
func (v *Vertex) CallWitnesses(c *OpContext) []*Existential {
	var witnesses []*Existential
	for _, e := range v.callWitnesses {
		saved := c.PushState(c.Env(0), e.Source())
		b := e.validate(c, v)
		if err := c.PopState(saved); b == nil && err == nil {
			witnesses = append(witnesses, e)
		}
	}
	return witnesses
}

// membershipCheck keeps the subject and the compatibility calculation apart.
// The meet may add fields or callable contracts. Such additions are not
// evidence about the supplied subject. In particular, callers must not publish
// meet as a replacement for subject, or mistake its concreteness for a proof.
type membershipCheck struct {
	subject   Value
	predicate scopedPredicate
	meet      *Vertex
}

func checkMembership(c *OpContext, subject Value, predicate scopedPredicate) membershipCheck {
	v := c.newInlineVertex(nil, nil, MakeRootConjunct(predicate.env, predicate.expr),
		MakeRootConjunct(nil, membershipOperand(subject)))
	v.Finalize(c)
	return membershipCheck{subject: subject, predicate: predicate, meet: v}
}

// checkCallValue discharges an execution boundary without changing its
// subject. Concrete membership and schema inclusion are different proofs;
// compatibility alone is neither. In particular, an incomplete body cannot
// become a value by meeting its result annotation or selecting its default.
func checkCallValue(c *OpContext, subject Value, predicate scopedPredicate, role string) *Bottom {
	if b, ok := Unwrap(subject).(*Bottom); ok {
		return b
	}
	if predicate.expr == nil {
		return nil
	}
	if concreteCapture(c, subject) {
		m := checkMembership(c, subject, predicate)
		if capabilityHasCallable(subject, make(map[Value]bool)) {
			bound, complete := c.Evaluate(predicate.env, predicate.expr)
			if complete && c.provesInclusion(bound, subject) {
				return nil
			}
			// Keep a failed body judgment distinct from unresolved value
			// membership. The admission search uses a three-way result, but
			// an execution boundary must preserve its blocking diagnostic.
			if b := Validate(c, m.meet, &ValidateConfig{Concrete: true, Final: true, Runtime: true,
				CheckFunction: c.CheckFunction, CheckBuiltin: c.CheckBuiltin}); b != nil && b.Code == BlockedError {
				return b
			}
		}
		switch m.packetMembership(c) {
		case proofEstablished:
			return nil
		case proofRefuted:
			if b := m.meet.Bottom(); b != nil && !b.IsIncomplete() {
				return b
			}
			return c.NewErrf("function %s does not satisfy its contract", role)
		}
	} else {
		// Inclusion can certify a schema-valued computation, but cannot
		// narrow it. Keep the independently evaluated subject as the result.
		bound, complete := c.Evaluate(predicate.env, predicate.expr)
		if complete && c.provesInclusion(bound, subject) {
			return nil
		}
	}
	return &Bottom{Code: IncompleteError,
		Err: c.Newf("function %s conformance remains unproved", role)}
}

// membershipOperand exposes the evaluated root without replaying its root
// validators. The original subject remains responsible for those obligations.
// Children are retained in their original scopes, including their predicates;
// this is deliberately not a recursive data/JSON projection. Optional and
// required fields, qualified labels, patterns and closedness all survive.
//
// This operand is only an input to a compatibility calculation. It cannot
// establish membership without checking the original subject and shape.
func membershipOperand(value Value) Value {
	v, ok := value.(*Vertex)
	if !ok {
		return value
	}
	return &evaluatedSubject{v.DerefValue()}
}

// evaluatedSubject is an internal evaluator input, never a public value or
// proof. insertValueConjunct consumes it by inserting evaluated components and
// constraints, without replaying root conjuncts. Keeping this distinct from
// Vertex's data mode prevents JSON projection rules entering membership.
type evaluatedSubject struct{ *Vertex }

// preservesShape checks membership's no-enrichment side condition. Every
// observable label matters, including definitions and hidden fields. A let is
// a lexical binding rather than a component of the supplied subject.
func (m membershipCheck) preservesShape(c *OpContext) bool {
	return sameSubjectShape(c, m.subject, m.meet, make(map[[2]*Vertex]bool))
}

func sameSubjectShape(c *OpContext, before, after Value, seen map[[2]*Vertex]bool) bool {
	a, aok := Unwrap(before).(*Vertex)
	b, bok := Unwrap(after).(*Vertex)
	if !aok || !bok {
		return true // Scalar conflicts are handled by the meet itself.
	}
	key := [2]*Vertex{a, b}
	if seen[key] {
		return true
	}
	seen[key] = true
	a.Finalize(c)
	b.Finalize(c)
	for _, field := range b.Arcs {
		if field.Label.IsLet() || field.ArcType == ArcOptional {
			continue
		}
		original := a.LookupRaw(field.Label)
		if original == nil || original.ArcType != ArcMember ||
			!sameSubjectShape(c, original, field, seen) {
			return false
		}
	}
	return true
}

// packetMembership is three-valued. Only a complete supplied packet has an
// exact absence observation. A contradiction in its meet refutes membership;
// a complete meet with no enrichment and independent callable inclusion proves
// it. Incomplete evaluation or proof is neither result.
func (m membershipCheck) packetMembership(c *OpContext) proofResult {
	if !concreteCapture(c, m.subject) {
		return proofUnknown
	}
	// Prove membership from the supplied subject before requiring its meet
	// with the predicate to be concrete. For an open-record union, that meet
	// can retain alternatives which add fields even though one original arm
	// already includes the whole subject. Those alternatives do not make the
	// independently established inclusion ambiguous.
	bound, complete := c.Evaluate(m.predicate.env, m.predicate.expr)
	if complete && c.provesInclusion(bound, m.subject) {
		return proofEstablished
	}
	if b := m.meet.Bottom(); b != nil {
		if !b.IsIncomplete() {
			return proofRefuted
		}
		return proofUnknown
	}
	if !m.preservesShape(c) {
		return proofRefuted
	}
	if b := Validate(c, m.meet, &ValidateConfig{Concrete: true, Final: true, Runtime: true}); b != nil {
		if !b.IsIncomplete() {
			return proofRefuted
		}
		return proofUnknown
	}
	if capabilityHasCallable(m.subject, make(map[Value]bool)) {
		bound, complete := c.Evaluate(m.predicate.env, m.predicate.expr)
		if complete && c.provesInclusion(bound, m.subject) {
			return proofEstablished
		}
		// The meet records callable contracts as obligations on the same
		// implementation. Prove each from its body; merely comparing the
		// combined signatures would assume the promised conformance. This
		// also covers singleton witnesses and packages, whose membership
		// evidence is identity rather than structural arrow inclusion.
		if c.CheckFunction == nil || c.CheckBuiltin == nil {
			return proofUnknown
		}
		if b := Validate(c, m.meet, &ValidateConfig{Concrete: true, Final: true, Runtime: true,
			CheckFunction: c.CheckFunction, CheckBuiltin: c.CheckBuiltin}); b != nil {
			return proofUnknown
		}
	}
	return proofEstablished
}
