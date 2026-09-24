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

// scopedPredicate is a proposition in its declaration scope. It is not a
// runtime witness, even when its evaluation has a concrete approximation.
type scopedPredicate struct {
	env  *Environment
	expr Expr
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
		if !complete || !c.provesInclusion(bound, m.subject) {
			return proofUnknown
		}
	}
	return proofEstablished
}
