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

func (p *sealedPackage) transportFunction(c *OpContext, schema *FuncValue, value Value, outward bool) Value {
	f, ok := Unwrap(value).(*FuncValue)
	if !ok {
		return c.NewErrf("interface requires a function implementation")
	}
	for _, a := range p.operations {
		if a.schema == schema.Fn && a.env == schema.Env && a.outward == outward &&
			equalFuncTypes(a.types, schema.Types) && equalFuncTypes(a.target.Types, f.Types) &&
			closureIdentity(c, a.target, f) == proofEstablished {
			return a.value
		}
	}
	s := &OpaqueCall{owner: p, private: f, signature: schema.Fn, env: schema.Env, outward: outward,
		clauses: schema.selectionAndOriginalClauses(), dispatch: len(schema.Types) != 0}
	adapter := s.adapter()
	for _, t := range schema.Types {
		adapter.Types = append(adapter.Types, s.advertised(t))
	}
	p.operations = append(p.operations, opaqueAdapter{schema: schema.Fn, types: schema.Types,
		env: schema.Env, target: f, outward: outward, value: adapter})
	return adapter
}

func (s *OpaqueCall) origin() *OpaqueCall {
	if s.root != nil {
		return s.root
	}
	return s
}

func (s *OpaqueCall) adapter() *FuncValue {
	fn := *s.signature
	fn.Params = slices.Clone(fn.Params)
	for i := range fn.Params {
		if fn.Params[i].Default != nil {
			// Omission belongs to the public protocol. Only the private
			// implementation supplies the value of an omitted argument.
			fn.Params[i].Default = nil
			fn.Params[i].ArcType = ArcOptional
		}
	}
	fn.Captures = nil
	fn.Body = s
	return &FuncValue{Src: fn.Src, Fn: &fn, Env: s.advertised(FuncType{Env: s.env}).Env}
}

func (s *OpaqueCall) advertised(t FuncType) FuncType {
	if !s.outward {
		args := make(map[*TypeParameter]Value, len(s.owner.carriers))
		for param, carrier := range s.owner.carriers {
			args[param] = carrier.representation
		}
		t.Env = instantiateEnvironment(t.Env, args)
	}
	return t
}

// A view can select ordinary type arguments, but its representation frame
// must not overwrite the public carrier frame used to direct transport.
func (s *OpaqueCall) publicEnvironment(public, selected *Environment) *Environment {
	args := make(map[*TypeParameter]Value)
	for e := selected; e != nil; e = e.Up {
		if e.types != nil {
			for p, v := range e.types.arguments {
				if s.owner.carriers[p] == nil {
					args[p] = v
				}
			}
		}
	}
	return instantiateEnvironment(public, args)
}

func (s *OpaqueCall) clause(t FuncType) *OpaqueCall {
	return &OpaqueCall{owner: s.owner, private: s.private, signature: t.Fn,
		env: t.Env, outward: s.outward, root: s.origin()}
}

// ProofAlternatives exposes each complete advertised arrow with its own
// lexical environment. The private implementation stays behind OpaqueCall.
func (s *OpaqueCall) ProofAlternatives() []*FuncValue {
	var out []*FuncValue
	for _, t := range s.origin().clauses {
		out = append(out, s.clause(t).adapter())
	}
	return out
}

func (s *OpaqueCall) callOverload(c *OpContext, f *FuncValue, call *CallExpr, state Flags) Value {
	clauses := s.origin().clauses
	if call.Partial || f.IsPartial() {
		// A saved packet has one stable coordinate system. Different rows
		// need a residual row witness; never reinterpret saved positions.
		for _, t := range clauses {
			if !sameOpaqueProtocol(f.Fn, t.Fn) {
				return &Bottom{Code: IncompleteError,
					Err: c.Newf("partial overloaded interface protocols remain unresolved")}
			}
		}
	}
	type admitted struct {
		view     *FuncValue
		bindings []funcArg
	}
	var candidates []admitted
	unknown := false
	for _, t := range clauses {
		// Explicit type selection specializes available views, while the
		// original universal clauses remain attached obligations.
		if t.Fn == s.signature {
			t.Env = s.publicEnvironment(t.Env, f.Env)
		}
		if f.selection != nil {
			for _, selected := range f.selection.clauses {
				matches := selected.Fn == t.Fn
				if view, ok := selected.Fn.Body.(*OpaqueCall); ok {
					matches = matches || view.origin() == s.origin() && view.signature == t.Fn
				}
				if matches {
					t.Env = s.publicEnvironment(t.Env, selected.Env)
				}
			}
		}
		view := s.clause(t).adapter()
		if view.Fn.Open {
			unknown = true
			continue
		}
		originalEnv := view.Env
		view.args = f.args
		bindings, unused, err := view.bindCall(c, call)
		if unused != nil || err != nil {
			continue
		}
		if len(typeParameters(view.Env)) != 0 {
			inst, err := view.inferInstance(c, bindings)
			if err != nil {
				unknown = unknown || err.IsIncomplete()
				continue
			}
			view = inst
		}
		applicable := proofEstablished
		for i, p := range view.Fn.Params {
			arg := bindings[i]
			if arg.expr == nil {
				if !call.Partial && p.ArcType != ArcOptional && p.Default == nil {
					applicable = proofRefuted
					break
				}
				continue
			}
			v, _ := c.Evaluate(arg.env, arg.expr)
			switch capabilityMember(c, view.Env, p.Value, v) {
			case proofRefuted:
				applicable = proofRefuted
			case proofUnknown:
				if applicable != proofRefuted {
					applicable = proofUnknown
				}
			}
		}
		switch applicable {
		case proofEstablished:
			if call.Partial {
				// Partial application saves a packet without fixing the
				// still-polymorphic residual interface to this trial.
				view.Env = originalEnv
			}
			candidates = append(candidates, admitted{view, bindings})
		case proofUnknown:
			unknown = true
		}
	}
	if unknown {
		return &Bottom{Code: IncompleteError, Err: c.Newf("overloaded interface applicability remains unresolved")}
	}
	if len(candidates) == 0 {
		return c.NewErrf("no interface clause admits this call packet")
	}
	chosen := candidates[0]
	for _, other := range candidates[1:] {
		if call.Partial {
			break // No transport occurs until the saved packet is completed.
		}
		publicType := func(f *FuncValue) FuncType {
			x := f.Fn.Body.(*OpaqueCall)
			return FuncType{Fn: x.signature, Env: s.publicEnvironment(x.env, f.Env)}
		}
		a, b := publicType(chosen.view), publicType(other.view)
		if !s.coherentPair(c, a, b) {
			return &Bottom{Code: IncompleteError, Err: c.Newf("overlapping interface transports remain unresolved")}
		}
	}
	chosen.view.Types = slices.Clone(f.Types)
	for _, t := range clauses {
		chosen.view.Types = mergeFuncTypes(chosen.view.Types, []FuncType{s.advertised(t)})
	}
	chosen.view.identities, chosen.view.scopes = f.identities, f.scopes
	chosen.view.selection = f.selection
	if call.Partial {
		chosen.view.args = chosen.bindings
		chosen.view.Fn.Body.(*OpaqueCall).dispatch = true
		return chosen.view
	}
	return chosen.view.call(c, call, state)
}

func sameOpaqueProtocol(a, b *Function) bool {
	if a.Open != b.Open || len(a.Params) != len(b.Params) {
		return false
	}
	for i, p := range a.Params {
		q := b.Params[i]
		if p.Label != q.Label || p.Positional != q.Positional {
			return false
		}
	}
	return true
}

// Separate private proofs do not establish a public intersection when two
// overlapping arrows wrap values differently. Prove transport agreement or
// disjoint packet domains before certifying the complete operation.
func (s *OpaqueCall) coherent(c *OpContext) bool {
	clauses := s.origin().clauses
	for i, a := range clauses {
		for _, b := range clauses[:i] {
			if !s.coherentPair(c, a, b) {
				return false
			}
		}
	}
	return true
}

func (s *OpaqueCall) coherentPair(c *OpContext, a, b FuncType) bool {
	da, db := s.advertised(a), s.advertised(b)
	matches := matchFuncParams(a.Fn, b.Fn, false)
	for i, j := range matches {
		p := a.Fn.Params[i]
		if j < 0 {
			if !b.Fn.Open && p.ArcType != ArcOptional && p.Default == nil {
				return true // The closed second row excludes this required slot.
			}
			continue
		}
		q := b.Fn.Params[j]
		if p.Value == nil || q.Value == nil {
			continue
		}
		if p.ArcType == ArcOptional || p.Default != nil {
			if q.ArcType == ArcOptional || q.Default != nil {
				continue // Both clauses also admit omission.
			}
		}
		meet := c.newInlineVertex(nil, nil, MakeRootConjunct(da.Env, p.Value), MakeRootConjunct(db.Env, q.Value))
		meet.Finalize(c)
		if bottom := meet.Bottom(); bottom != nil && !bottom.IsIncomplete() {
			return true
		}
	}
	for j, q := range b.Fn.Params {
		if !slices.Contains(matches, j) && !a.Fn.Open && q.ArcType != ArcOptional && q.Default == nil {
			return true
		}
	}
	for i, j := range matches {
		if j >= 0 && !s.sameTransport(c, a.Env, a.Fn.Params[i].Value, b.Env, b.Fn.Params[j].Value) {
			return false
		}
	}
	return s.sameTransport(c, a.Env, a.Fn.Ret, b.Env, b.Fn.Ret)
}

func (s *OpaqueCall) sameTransport(c *OpContext, ae *Environment, a Expr, be *Environment, b Expr) bool {
	if a == b && ae == be {
		return true
	}
	var av, bv Value = &Top{}, &Top{}
	var complete bool
	if a != nil {
		av, complete = c.Evaluate(ae, a)
		if !complete || av == nil {
			return false
		}
	}
	if b != nil {
		bv, complete = c.Evaluate(be, b)
		if !complete || bv == nil {
			return false
		}
	}
	if !abstractEscapes(c, av, s.owner, make(map[Value]bool)) &&
		!abstractEscapes(c, bv, s.owner, make(map[Value]bool)) &&
		!capabilityHasCallable(av, make(map[Value]bool)) && !capabilityHasCallable(bv, make(map[Value]bool)) {
		return true
	}
	return c.provesInclusion(av, bv) && c.provesInclusion(bv, av)
}
