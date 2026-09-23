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

// Inference constructs a sufficient witness for one use of a universal.
// Lower and upper constraints remain separate until the telescope is solved:
// a callback's input supplies an upper bound, not an assignment. Failure of
// the chosen witness is incomplete unless an independent necessary condition
// refutes every instance. Guarded calls rely on this distinction.
type typeInference struct {
	c      *OpContext
	params []*TypeParameter
	bounds map[*TypeParameter]*inferenceBounds

	deferCallbacks bool
	callbacks      []callbackInference
}

type callbackInference struct {
	env       *Environment
	pattern   *Function
	value     Value
	covariant bool
}

type inferenceBounds struct {
	lower, upper Value
	env          *Environment
}

func newTypeInference(c *OpContext, env *Environment) *typeInference {
	s := &typeInference{c: c, params: typeParameters(env), bounds: make(map[*TypeParameter]*inferenceBounds)}
	for e := env; e != nil; e = e.Up {
		if e.types != nil {
			for _, p := range e.types.quantifier.Params {
				if e.types.arguments[p] == nil {
					s.bounds[p] = &inferenceBounds{env: e}
				}
			}
		}
	}
	return s
}

func (f *FuncValue) inferInstance(c *OpContext, bindings []funcArg) (*FuncValue, *Bottom) {
	s := newTypeInference(c, f.Env)
	// First learn input witnesses from data, then instantiate callback
	// schemes from those witnesses and collect their result constraints.
	// Defer at the function-pattern boundary, not just at the packet root,
	// so callbacks nested in data learn from all the other data slots too.
	s.deferCallbacks = true
	for i, binding := range bindings {
		if binding.expr == nil {
			continue
		}
		v, _ := c.Evaluate(binding.env, binding.expr)
		if vertex, ok := v.(*Vertex); ok {
			vertex.Finalize(c)
		}
		s.collect(f.Env, f.Fn.Params[i].Value, v, true)
	}
	s.deferCallbacks = false
	for _, cb := range s.callbacks {
		s.collect(cb.env, cb.pattern, cb.value, cb.covariant)
	}
	// Bounds can mention preceding binders. Propagate required lower
	// predicates backwards before choosing any earlier witness.
	for _, p := range slices.Backward(s.params) {
		if b := s.bounds[p]; b.lower != nil && p.Bound != nil {
			s.collect(b.env, p.Bound, b.lower, true)
		}
	}
	args := s.arguments()
	inst, err := f.instantiate(c, args)
	if err == nil {
		for _, p := range s.params {
			if args[p] == nil {
				err = c.NewErrf("cannot infer type argument %s", p.Src.Name.Name)
				break
			}
		}
	}
	if err == nil {
		// A successful bound check only admits the type arguments. Verify
		// that the resulting instance admits every supplied packet slot.
		// Inclusion also works for symbolic packets used by certification.
		for i, binding := range bindings {
			if binding.expr == nil || f.Fn.Params[i].Value == nil {
				continue
			}
			v, _ := c.Evaluate(binding.env, binding.expr)
			bound, complete := c.Evaluate(inst.Env, f.Fn.Params[i].Value)
			if !complete || typeArgumentFits(c, bound, v) != proofEstablished {
				err = c.NewErrf("inferred instance does not prove argument membership")
				break
			}
		}
	}
	if err == nil {
		return inst, nil
	}
	// A direct argument of A must satisfy A's declared upper bound for
	// every admissible A. Test that necessary condition in the original
	// telescope, independently of all speculative substitutions.
	for i, binding := range bindings {
		ref, ok := f.Fn.Params[i].Value.(*TypeReference)
		if !ok || binding.expr == nil {
			continue
		}
		b := s.bounds[ref.Param]
		if b == nil {
			continue
		}
		v, _ := c.Evaluate(binding.env, binding.expr)
		if err := checkTypeUniverse(c, ref.Param, v); err != nil && !err.IsIncomplete() {
			return nil, err
		}
		if bound := s.declaredUpper(ref.Param); bound != nil &&
			capabilityMember(c, nil, bound, v) == proofRefuted {
			return nil, c.NewErrf("argument cannot satisfy bound of %s", ref.Param.Src.Name.Name)
		}
	}
	return nil, &Bottom{Src: f.Source(), Code: IncompleteError,
		Err: c.Newf("type argument inference remains unresolved: %v", err.Err)}
}

// A chain B <: A <: number supplies a necessary ground bound on B without
// choosing A. More general unresolved bound expressions need a proof of
// monotonicity before their variables can be replaced by upper predicates.
func (s *typeInference) declaredUpper(p *TypeParameter) Value {
	if p.Bound == nil {
		return nil
	}
	b := s.bounds[p]
	if b == nil {
		return nil
	}
	bound, complete := s.c.Evaluate(b.env, p.Bound)
	if complete {
		return bound
	}
	if ref, ok := p.Bound.(*TypeReference); ok {
		return s.declaredUpper(ref.Param)
	}
	return nil
}

func (s *typeInference) meet(a, b Value) Value {
	if a == nil {
		return b
	}
	if b == nil || Equal(s.c, a, b, CheckStructural) {
		return a
	}
	v := s.c.newInlineVertex(nil, nil, MakeRootConjunct(nil, a), MakeRootConjunct(nil, b))
	v.Finalize(s.c)
	return Unwrap(v)
}

// arguments selects candidates in lexical order, so dependent upper bounds
// are evaluated with earlier choices. A lower predicate takes precedence;
// callback inputs and declared bounds can only narrow an unconstrained slot.
func (s *typeInference) arguments() map[*TypeParameter]Value {
	args := make(map[*TypeParameter]Value, len(s.params))
	for _, p := range s.params {
		b := s.bounds[p]
		v := b.lower
		if v == nil {
			v = b.upper
			if v != nil && p.Bound != nil {
				bound, complete := s.c.Evaluate(instantiateEnvironment(b.env, args), p.Bound)
				if complete {
					v = s.meet(v, bound)
				}
			}
		}
		if v == nil && p.ValueRange == nil {
			v = &Bottom{Code: EvalError, Err: s.c.Newf("empty type instance")}
		}
		args[p] = v
	}
	return args
}

// collect records value <= pattern when covariant, pattern <= value otherwise.
// Function inputs reverse that relation; results and data constructors keep it.
func (s *typeInference) collect(env *Environment, pattern Expr, value Value, covariant bool) {
	if value == nil {
		return
	}
	if b, ok := Unwrap(value).(*Bottom); ok && b.IsIncomplete() {
		return
	}
	switch p := pattern.(type) {
	case *TypeReference:
		b := s.bounds[p.Param]
		if b == nil {
			return
		}
		value = Unwrap(value)
		if v, ok := value.(*Vertex); ok && concreteCapture(s.c, v) {
			value = v.ToDataAll(s.c)
		}
		if !covariant {
			b.upper = s.meet(b.upper, value)
		} else if b.lower == nil {
			b.lower = value
		} else if !Equal(s.c, b.lower, value, CheckStructural) {
			b.lower = &Disjunction{Values: []Value{b.lower, value}}
		}
	case *ListLit:
		v, ok := value.(*Vertex)
		if !ok || !v.IsList() {
			return
		}
		v.Finalize(s.c)
		template, _ := s.c.Evaluate(env, p)
		if scope, ok := template.(*Vertex); ok {
			env = &Environment{Up: env, Vertex: scope}
		}
		elems := slices.Collect(v.Elems())
		for i, e := range p.Elems {
			if rest, ok := e.(*Ellipsis); ok {
				for _, item := range elems[min(i, len(elems)):] {
					s.collect(env, rest.Value, item, covariant)
				}
				break
			}
			if i < len(elems) {
				if e, ok := e.(Expr); ok {
					s.collect(env, e, elems[i], covariant)
				}
			}
		}
	case *StructLit:
		v, ok := value.(*Vertex)
		if !ok {
			return
		}
		v.Finalize(s.c)
		template, _ := s.c.Evaluate(env, p)
		if scope, ok := template.(*Vertex); ok {
			env = &Environment{Up: env, Vertex: scope}
		}
		for _, d := range p.Decls {
			if field, ok := d.(*Field); ok {
				if arc := v.LookupRaw(field.Label); arc != nil {
					s.collect(env, field.Value, arc, covariant)
				}
			}
		}
	case *Function:
		if s.deferCallbacks {
			s.callbacks = append(s.callbacks, callbackInference{env, p, value, covariant})
			return
		}
		f, ok := Unwrap(value).(*FuncValue)
		if !ok {
			return
		}
		if len(typeParameters(f.Env)) != 0 {
			// Eliminate a callback scheme using expected inputs. This
			// selects a use-site witness; it never generalizes a callback.
			callback := newTypeInference(s.c, f.Env)
			expected := instantiateEnvironment(env, s.arguments())
			for i, j := range matchFuncParams(p, f.Fn, false) {
				if j >= 0 {
					v, _ := s.c.Evaluate(expected, p.Params[i].Value)
					callback.collect(f.Env, f.Fn.Params[j].Value, v, true)
				}
			}
			if inst, err := f.instantiate(s.c, callback.arguments()); err == nil {
				f = inst
			}
		}
		for i, j := range matchFuncParams(p, f.Fn, false) {
			if j >= 0 {
				v, _ := s.c.Evaluate(f.Env, f.Fn.Params[j].Value)
				s.collect(env, p.Params[i].Value, v, !covariant)
			}
		}
		if f.Fn.Ret != nil {
			v, _ := s.c.Evaluate(f.Env, f.Fn.Ret)
			s.collect(env, p.Ret, v, covariant)
		}
	}
}
