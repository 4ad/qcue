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

// ExplicitClauses returns the source description accumulated for this
// subject, in its original declaration scopes. Type selection, inference,
// composite projection, and partial application preserve these clauses;
// only another explicit declaration adds one. The returned slice is borrowed
// and must not be modified. It is independent of executable call frontiers
// and of the additional consequences retained by Obligations.
func (f *FuncValue) ExplicitClauses() []FuncType { return f.explicit }

// Obligations returns every scoped contract retained by this inhabitant.
// These are proof obligations, not permissions to execute a selected view.
// In particular an erased universal remains an obligation after selection
// without reopening that view's call domain.
func (f *FuncValue) Obligations() []FuncType {
	return f.selectionAndOriginalClauses()
}

// CallClauses describes the call domains of this view. A consumer may use
// these clauses only after establishing implementation conformance or under
// an admitted callback hypothesis. This operation supplies no execution or
// conformance evidence by itself.
//
// Selection and partial application are handled here, rather than by clients
// assuming that Fn or Types is the whole callable interface. Saved arguments
// must satisfy a full clause before that clause can describe residual calls.
func (f *FuncValue) CallClauses(c *OpContext) []FuncType {
	return f.CallClausesFor(c, FuncType{})
}

// CallClausesFor includes the proposed residual packet when inferring an
// instance of a partial polymorphic closure. Saved arguments constrain that
// choice but do not freeze it before the rest of the packet is available.
func (f *FuncValue) CallClausesFor(c *OpContext, packet FuncType) []FuncType {
	clauses := f.selectionClauses()

	return f.residualClauses(c, clauses, packet)
}

// ResultClauses retains additional guarded consequences without enlarging a
// checked view's executable domain. Admission must first be proved from its
// CallClauses. Both operations preserve the coordinates of partial packets.
func (f *FuncValue) ResultClauses(c *OpContext) []FuncType {
	return f.ResultClausesFor(c, FuncType{})
}

func (f *FuncValue) ResultClausesFor(c *OpContext, packet FuncType) []FuncType {
	return f.residualClauses(c, f.Obligations(), packet)
}

func (f *FuncValue) residualClauses(c *OpContext, clauses []FuncType, remaining FuncType) []FuncType {
	if !f.IsPartial() {
		return clauses
	}
	var residual []FuncType
	packet := callPacket{args: f.args}
	for _, clause := range clauses {
		bound, result := packet.project(clause, f.Fn)
		if result != proofEstablished {
			continue
		}
		hasBound := false
		for _, arg := range bound.args {
			hasBound = hasBound || arg.expr != nil
		}
		if hasBound {
			if remaining.Fn != nil && len(typeParameters(clause.Env)) != 0 {
				view := &FuncValue{Fn: clause.Fn, Env: clause.Env, args: bound.args}
				rest, slots := view.residualSignature()
				combined := slices.Clone(bound.args)
				for i, j := range matchFuncParams(remaining.Fn, rest, false) {
					if j >= 0 {
						combined[slots[j]] = funcArg{expr: remaining.Fn.Params[i].Value, env: remaining.Env}
					}
				}
				inst, b := view.inferInstance(c, combined)
				if b != nil {
					continue
				}
				clause.Env = inst.Env
			}
			// A static residual view may save an arbitrary admitted value,
			// represented by its type. Concrete packet membership would
			// wrongly demand that value's materialization here.
			instance, ok := bound.typeAdmission(c, clause)
			if !ok {
				continue
			}
			clause = instance
		}
		view := &FuncValue{Fn: clause.Fn, Env: clause.Env, args: bound.args}
		fn, _ := view.residualSignature()
		residual = append(residual, FuncType{Fn: fn, Env: clause.Env})
	}
	return residual
}

func (packet callPacket) typeAdmission(c *OpContext, clause FuncType) (FuncType, bool) {
	if len(typeParameters(clause.Env)) != 0 {
		instance, b := (&FuncValue{Fn: clause.Fn, Env: clause.Env}).inferInstance(c, packet.args)
		if b != nil {
			return clause, false
		}
		clause.Env = instance.Env
	}
	for i, arg := range packet.args {
		if arg.expr == nil || clause.Fn.Params[i].Value == nil {
			continue
		}
		var value Value
		ok := false
		if c.CheckArgument != nil {
			value = c.CheckArgument(arg.env, arg.expr)
			ok = value != nil
		} else {
			value, ok = c.Evaluate(arg.env, arg.expr)
		}
		if !ok {
			return clause, false
		}
		want, ok := c.Evaluate(clause.Env, clause.Fn.Params[i].Value)
		if !ok || c.ProveInclusion == nil {
			return clause, false
		}
		if c.ProveInclusion(c, want, value) {
			continue
		}
		if capabilityHasCallable(want, make(map[Value]bool)) {
			return clause, false
		}

	}
	return clause, true
}
