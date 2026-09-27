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

// A capability is a universal implication from admitted packets to successful
// results. Its domain constrains the implication, not the implementation's
// protocol. In particular, meeting capabilities never installs a default or
// narrows a closure's accepted argument values.
func capabilityMode(fn *Function, types []FuncType) bool {
	if fn != nil && fn.Quantified {
		return true
	}
	return slices.ContainsFunc(types, func(t FuncType) bool { return t.Fn.Quantified })
}

func mergeCapabilities(c *OpContext, a, b *FuncValue) (*FuncValue, *Bottom) {
	if !IsFuncType(a) && !IsFuncType(b) {
		return mergeClosureIdentities(c, a, b)
	}
	if IsFuncType(a) && !IsFuncType(b) {
		a, b = b, a
	}
	m := *a
	m.explicit = mergeFuncTypes(a.explicit, b.explicit)
	incoming := append([]FuncType{{Fn: b.Fn, Env: b.Env}}, b.Types...)
	if a.IsPartial() {
		for i := range incoming {
			incoming[i].partial = a
		}
	}
	m.Types = mergeFuncTypes(a.Types, incoming)
	return &m, nil
}

// fixedCapabilityExpr identifies descriptions independent of refinable
// witnesses. Ground membership and lexical-identity checks can inspect these
// without selecting one possible completion of an unresolved environment.
func fixedCapabilityExpr(x Expr) bool {
	switch x := x.(type) {
	case nil, *Top, *BasicType, *Num, *String, *Bool, *Bytes, *Null:
		return true
	case *Bottom:
		return !x.IsIncomplete()
	case *BoundExpr:
		return fixedCapabilityExpr(x.Expr)
	case *BoundValue:
		return fixedCapabilityExpr(x.Value)
	case *BinaryExpr:
		return x.Op == AndOp && fixedCapabilityExpr(x.X) && fixedCapabilityExpr(x.Y)
	case *DisjunctionExpr:
		for _, d := range x.Values {
			if !fixedCapabilityExpr(d.Val) {
				return false
			}
		}
		return true
	}
	return false
}

// capabilityMember is three-valued. A failed concrete meet refutes membership;
// a successful complete validation establishes it. Incomplete evaluation is
// neither a refutation nor a proof.
func capabilityMember(c *OpContext, env *Environment, constraint Expr, value Value) proofResult {
	if constraint == nil {
		return proofEstablished
	}
	// Do not evaluate a speculative meet on an incomplete supplied packet.
	if !concreteCapture(c, value) {
		return proofUnknown
	}
	m := checkMembership(c, value, scopedPredicate{env, constraint})
	return m.packetMembership(c)
}

func capabilityHasCallable(v Value, seen map[Value]bool) bool {
	if v == nil || seen[v] {
		return false
	}
	seen[v] = true
	switch v := Unwrap(v).(type) {
	case *FuncValue, *Builtin:
		return true
	case *Vertex:
		for _, a := range v.Arcs {
			if a.ArcType == ArcMember && !a.Label.IsLet() && !a.Label.IsDef() && capabilityHasCallable(a, seen) {
				return true
			}
		}
	}
	return false
}

// recordCallResult retains certified interfaces for later existential and
// quantified elimination. Annotations neither construct nor filter the body's
// independently computed result; their implications were proved statically.
func (f *FuncValue) recordCallResult(c *OpContext, packet callPacket, value Value) {
	evidence := []scopedPredicate{{f.Env, f.Fn.Ret}}
	for _, t := range f.Types {
		if t.Fn.Body != nil {
			continue
		}
		projected, result := packet.project(t, f.Fn)
		var admitted *packetAdmission
		if result == proofEstablished {
			admitted, result = projected.admit(c, t, false)
		}
		switch result {
		case proofEstablished:
			t := admitted.clause
			evidence = append(evidence, scopedPredicate{t.Env, t.Fn.Ret})
		}
	}
	if v, ok := value.(*Vertex); ok {
		for _, predicate := range evidence {
			v.addCallWitness(c, predicate)
		}
	}
}

// capabilityMatches translates a residual packet back to the implementation
// activation. The mask is fixed when the clause is attached, so later partial
// applications preserve obligations over arguments they have since bound.
func capabilityMatches(t FuncType, fn *Function) []int {
	if t.partial == nil {
		return matchFuncParams(t.Fn, fn, false)
	}
	residual, slots := t.partial.residualSignature()
	matches := matchFuncParams(t.Fn, residual, false)
	for i, j := range matches {
		if j >= 0 {
			matches[i] = slots[j]
		}
	}
	return matches
}

func (f *FuncValue) residualSignature() (*Function, []int) {
	fn := *f.Fn
	fn.Params = nil
	var slots []int
	for i, p := range f.Fn.Params {
		if i < len(f.args) && f.args[i].expr != nil {
			continue
		}
		fn.Params = append(fn.Params, p)
		slots = append(slots, i)
	}
	return &fn, slots
}

// ResidualSignature returns the implementation's remaining call protocol.
// It does not alter the origin, captures, or attached capability clauses.
func (f *FuncValue) ResidualSignature() *Function {
	fn, _ := f.residualSignature()
	return fn
}
