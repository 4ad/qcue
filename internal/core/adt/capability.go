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

import (
	"slices"
	"strings"
)

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
	incoming := append([]FuncType{{Fn: b.Fn, Env: b.Env}}, b.Types...)
	if a.IsPartial() {
		for i := range incoming {
			incoming[i].partial = a
		}
	}
	m.Types = mergeFuncTypes(a.Types, incoming)
	clauses := append([]FuncType{{Fn: m.Fn, Env: m.Env}}, m.Types...)
	for i, t := range clauses {
		if t.Fn.Body != nil {
			continue
		}
		if !IsFuncType(&m) && !m.IsPartial() {
			if err := refuteCapability(c, &m, t); err != nil {
				return nil, err
			}
			if len(typeParameters(t.Env)) != 0 {
				if err := refuteGenericCapability(c, &m, t); err != nil {
					return nil, err
				}
			}
		}
		for _, u := range clauses[:i] {
			if u.Fn.Body == nil {
				if err := refuteArrowIntersection(c, t, u); err != nil {
					return nil, err
				}
				if len(typeParameters(t.Env)) != 0 || len(typeParameters(u.Env)) != 0 {
					if err := refuteGenericIntersection(c, t, u); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return &m, nil
}

// fixedCapabilityExpr admits a small closed vocabulary for counterexample
// search. A reference to a refinable witness is deliberately outside it: a
// counterexample to one assignment of that witness cannot refute every
// remaining environment. Such implications remain in the signature store.
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
			if a.ArcType == ArcMember && !a.Label.IsLet() && capabilityHasCallable(a, seen) {
				return true
			}
		}
	}
	return false
}

// capabilityPackets searches a bounded, deterministic vocabulary. Its results
// are witnesses only: successful trials never certify a universal obligation.
// The budget belongs to this clause, so unrelated declarations cannot consume
// another clause's opportunities to find a counterexample.
func capabilityPackets(c *OpContext, t FuncType, visit func(*CallExpr) bool) {
	if t.Fn.Open {
		return // The protocol row has not yet been supplied.
	}
	for _, p := range t.Fn.Params {
		if !fixedCapabilityExpr(p.Value) {
			return
		}
	}
	values := []Value{&Null{}, &Bool{B: false}, &Bool{B: true},
		&String{Str: ""}, &String{Str: "x"}}
	for _, s := range []string{"0", "1", "-1", "2", "1.5"} {
		x := &Num{K: IntKind}
		x.X.SetString(s)
		if s == "1.5" {
			x.K = FloatKind
		}
		values = append(values, x)
	}
	budget := 64
	var enumerate func(int, []Expr, []Feature, bool) bool
	enumerate = func(i int, args []Expr, labels []Feature, positional bool) bool {
		if budget == 0 {
			return false
		}
		if i == len(t.Fn.Params) {
			budget--
			return visit(&CallExpr{Args: slices.Clone(args), ArgLabels: slices.Clone(labels)})
		}
		p := t.Fn.Params[i]
		if p.ArcType == ArcOptional || p.Default != nil {
			if !enumerate(i+1, args, labels, positional && !p.Positional) {
				return false
			}
		}
		for _, value := range values {
			if capabilityMember(c, t.Env, p.Value, value) != proofEstablished {
				continue
			}
			if p.Positional && positional {
				if !enumerate(i+1, append(args, value), append(labels, InvalidLabel), true) {
					return false
				}
			}
			if p.Label != InvalidLabel {
				if !enumerate(i+1, append(args, value), append(labels, p.Label), false) {
					return false
				}
			}
		}
		return true
	}
	enumerate(0, nil, nil, true)
}

func refuteCapability(c *OpContext, impl *FuncValue, t FuncType) (err *Bottom) {
	// Speculative refutation must not execute a foreign operation, demand a
	// captured computation, or unfold recursion. The larger expression
	// language remains a residual obligation until a sound rule handles it.
	if !probeBody(impl.Fn.Body) || !fixedCapabilityExpr(impl.Fn.Ret) {
		return nil
	}
	for _, p := range impl.Fn.Params {
		if !fixedCapabilityExpr(p.Value) || !fixedCapabilityExpr(p.Default) {
			return nil
		}
	}
	// Remove the target and every other asserted contract from the trial.
	// An implementation must not use its own obligation as its evidence.
	raw := *impl
	raw.Types = nil
	capabilityPackets(c, t, func(packet *CallExpr) bool {
		saved := c.PushState(impl.Env, t.Fn.Source())
		result := raw.call(c, packet, Flags{})
		if b := c.PopState(saved); b != nil {
			result = b
		}
		if result == nil {
			return true
		}
		if b, ok := Unwrap(result).(*Bottom); ok {
			if b.IsIncomplete() {
				return true
			}
			err = c.NewErrf("function rejects admitted packet %s: %s", capabilityPacket(c, packet), b.Err)
			return false
		}
		if fixedCapabilityExpr(t.Fn.Ret) &&
			capabilityMember(c, t.Env, t.Fn.Ret, result) == proofRefuted {
			err = c.NewErrf("function result conflicts with its contract at packet %s", capabilityPacket(c, packet))
			return false
		}
		return true
	})
	return err
}

func probeBody(x Expr) bool {
	if fixedCapabilityExpr(x) {
		return true
	}
	switch x := x.(type) {
	case *FieldReference:
		return x.UpCount == 0
	case *UnaryExpr:
		return probeBody(x.X)
	case *BinaryExpr:
		return probeBody(x.X) && probeBody(x.Y)
	case *Interpolation:
		for _, p := range x.Parts {
			if !probeBody(p) {
				return false
			}
		}
		return true
	}
	return false
}

func refuteArrowIntersection(c *OpContext, a, b FuncType) *Bottom {
	// Disjoint successful results do not refute two contracts that admit
	// the same failure outcome. Such an implementation may raise that
	// effect at every packet in their overlapping domain.
	if a.Fn.Src != nil && b.Fn.Src != nil &&
		a.Fn.Src.Effect != nil && b.Fn.Src.Effect != nil &&
		a.Fn.Src.Effect.Name == b.Fn.Src.Effect.Name {
		return nil
	}
	if !fixedCapabilityExpr(a.Fn.Ret) || !fixedCapabilityExpr(b.Fn.Ret) ||
		len(a.Fn.Params) != len(b.Fn.Params) {
		return nil
	}
	// Only identical packet protocols are compared here; unequal protocols
	// remain separate clauses until a packet proves that their domains meet.
	for i, p := range a.Fn.Params {
		q := b.Fn.Params[i]
		if p.Positional != q.Positional || p.Label != q.Label ||
			p.ArcType != q.ArcType || p.Default != nil || q.Default != nil ||
			!fixedCapabilityExpr(q.Value) {
			return nil
		}
	}
	if a.Fn.Ret == nil || b.Fn.Ret == nil {
		return nil
	}
	v := c.newInlineVertex(nil, nil,
		MakeRootConjunct(a.Env, a.Fn.Ret), MakeRootConjunct(b.Env, b.Fn.Ret))
	v.Finalize(c)
	if bottom := v.Bottom(); bottom == nil || bottom.IsIncomplete() {
		return nil
	}
	var witness string
	capabilityPackets(c, a, func(packet *CallExpr) bool {
		if len(packet.Args) != len(b.Fn.Params) {
			return true
		}
		for i, value := range packet.Args {
			if capabilityMember(c, b.Env, b.Fn.Params[i].Value, value.(Value)) != proofEstablished {
				return true
			}
		}
		witness = capabilityPacket(c, packet)
		return false
	})
	if witness != "" {
		return c.NewErrf("incompatible function results at admitted packet %s", witness)
	}
	return nil
}

func (n *nodeContext) scheduleCapabilityResults(ref *FuncCallRef, env *Environment, ci CloseInfo) {
	for _, t := range ref.types {
		if t.Fn.Body != nil {
			continue
		}
		result := proofUnknown
		var admitted *packetAdmission
		if env.packet != nil {
			var packet callPacket
			packet, result = env.packet.project(t, ref.fn)
			if result == proofEstablished {
				admitted, result = packet.admit(n.ctx, t, false)
			}
		}
		switch result {
		case proofEstablished:
			t := admitted.clause
			if t.Fn.Ret != nil {
				n.scheduleConjunct(MakeConjunct(t.Env, t.Fn.Ret, ci), ci)
			}
		case proofUnknown:
			n.addBottom(&Bottom{Code: IncompleteError,
				Err: n.ctx.Newf("incomplete function contract domain")})
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

func capabilityPacket(c *OpContext, packet *CallExpr) string {
	values := make([]string, len(packet.Args))
	for i, x := range packet.Args {
		if i < len(packet.ArgLabels) && packet.ArgLabels[i] != InvalidLabel {
			values[i] = packet.ArgLabels[i].SelectorString(c) + ": "
		}
		values[i] += c.String(x)
	}
	return "(" + strings.Join(values, ", ") + ")"
}
