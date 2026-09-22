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
	"maps"
	"slices"

	"cuelang.org/go/cue/ast"
)

// TypeParameter is identified by its declaration, never by its spelling.
// Bounds remain expressions in their telescope scope until an instance is
// selected. Level is the predicative universe of admissible arguments.
type TypeParameter struct {
	Src   *ast.TypeParam
	Bound Expr
	Level int
	// ValueRange is non-nil for the finite value-binder fragment. General
	// dependent ranges and signatures remain outside this profile.
	ValueRange Expr
}

// Quantified is a retained predicate template over one subject.
type Quantified struct {
	Src    *ast.Quantifier
	Params []*TypeParameter
	Body   Expr
}

func (q *Quantified) Source() ast.Node { return q.Src }
func (*Quantified) node()              {}
func (*Quantified) expr()              {}
func (*Quantified) declNode()          {}
func (*Quantified) elemNode()          {}

// TypeReference is distinct from an ordinary CUE witness reference. A type
// argument denotes its semantic predicate; an ordinary witness in a type
// position still denotes the singleton of that witness.
type TypeReference struct {
	Src     *ast.Ident
	Param   *TypeParameter
	UpCount int32
}

func (r *TypeReference) Source() ast.Node { return r.Src }
func (*TypeReference) node()              {}
func (*TypeReference) expr()              {}
func (*TypeReference) declNode()          {}
func (*TypeReference) elemNode()          {}

// typeScope holds one introduction of a quantified template. Argument maps
// are immutable after construction; uses instantiate independent copies.
type typeScope struct {
	quantifier *Quantified
	arguments  map[*TypeParameter]Value
}

// AliasApplication substitutes predicates into an abbreviation. It creates
// no shared universal subject and binds arguments in the caller's scope.
type AliasApplication struct {
	Src      *ast.CallExpr
	Template *Quantified
	UpCount  int32
	Args     []Expr
}

func (a *AliasApplication) Source() ast.Node { return a.Src }
func (*AliasApplication) node()              {}
func (*AliasApplication) expr()              {}
func (*AliasApplication) declNode()          {}
func (*AliasApplication) elemNode()          {}

func (a *AliasApplication) evaluate(c *OpContext, state Flags) Value {
	args := make(map[*TypeParameter]Value, len(a.Args))
	for i, x := range a.Args {
		v, _ := c.Evaluate(c.Env(0), x)
		if v == nil {
			return nil
		}
		args[a.Template.Params[i]] = v
	}
	scope := c.newInlineVertex(nil, &StructMarker{})
	env := &Environment{Up: c.Env(a.UpCount), Vertex: scope,
		types: &typeScope{quantifier: a.Template}}
	f := &FuncValue{Env: env}
	inst, b := f.instantiate(c, args)
	if b != nil {
		return b
	}
	v, _ := c.Evaluate(inst.Env, a.Template.Body)
	return v
}

func (q *Quantified) evaluate(c *OpContext, state Flags) Value {
	for _, p := range q.Params {
		if p.ValueRange != nil {
			return q.evaluateFinite(c)
		}
	}
	if q.Src.Exists {
		return &Existential{Template: q, Env: c.Env(0)}
	}
	if !covariantData(q.Body) && !distributableUniversal(q.Body) {
		return &Universal{Template: q, Env: c.Env(0)}
	}
	scope := c.newInlineVertex(nil, nil)
	scope.BaseValue = &StructMarker{}
	env := &Environment{
		Up: c.Env(0), Vertex: scope,
		types: &typeScope{quantifier: q},
	}
	if covariantData(q.Body) {
		// Data constructors, intersections, and unions are monotone in
		// their element predicates. Their universal meet is therefore
		// attained at bottom, which belongs to every upper-bounded type
		// telescope. This rule never crosses an arrow's negative domain.
		env.types.arguments = make(map[*TypeParameter]Value, len(q.Params))
		for _, p := range q.Params {
			env.types.arguments[p] = &Bottom{Code: EvalError,
				Err: c.Newf("universal type parameter has an empty instance")}
		}
	}
	v, _ := c.Evaluate(env, q.Body)
	return v
}

func (q *Quantified) evaluateFinite(c *OpContext) Value {
	for _, p := range q.Params {
		if p.ValueRange == nil {
			// Mixed prefixes are retained rather than commuting type and
			// value scopes to force a finite expansion.
			if q.Src.Exists {
				return &Existential{Template: q, Env: c.Env(0)}
			}
			return &Universal{Template: q, Env: c.Env(0)}
		}
	}
	var expand func(*Environment, int) Value
	expand = func(env *Environment, i int) Value {
		if i == len(q.Params) {
			v, _ := c.Evaluate(env, q.Body)
			return v
		}
		p := q.Params[i]
		saved := c.PushState(env, p.Src)
		rangeValue, _ := c.Evaluate(env, p.ValueRange)
		if b := c.PopState(saved); b != nil {
			rangeValue = b
		}
		var candidates []Value
		switch v := Unwrap(rangeValue).(type) {
		case *Bottom:
			if v.IsIncomplete() {
				return v
			}
		case *Disjunction:
			candidates = v.Values
		case *Null, *Bool, *Num, *String, *Bytes:
			candidates = []Value{v}
		default:
			return &Bottom{Code: IncompleteError, Err: c.Newf("finite binder range remains unresolved")}
		}
		if len(candidates) == 0 {
			if !q.Src.Exists {
				return &Top{}
			}
			return c.NewErrf("existential binder has an empty range")
		}
		values := make([]Value, 0, len(candidates))
		for _, v := range candidates {
			args := maps.Clone(env.types.arguments)
			args[p] = v
			frame := *env
			frame.types = &typeScope{quantifier: q, arguments: args}
			frame.cache = nil
			value := expand(&frame, i+1)
			if value == nil {
				return nil
			}
			values = append(values, value)
		}
		if q.Src.Exists {
			return &Disjunction{Values: values}
		}
		return &Conjunction{Values: values}
	}
	env := &Environment{Up: c.Env(0), Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: q, arguments: make(map[*TypeParameter]Value)}}
	return expand(env, 0)
}

// Universals commute with conjunction and fixed record projections, but
// generally not with a union of arrows. Keep unsupported Boolean placement
// as one exact scoped predicate rather than strengthening each union arm.
func distributableUniversal(x Expr) bool {
	if covariantData(x) {
		return true
	}
	switch x := x.(type) {
	case *Function, *Quantified:
		return true
	case *BinaryExpr:
		return x.Op == AndOp && distributableUniversal(x.X) && distributableUniversal(x.Y)
	case *StructLit:
		for _, d := range x.Decls {
			f, ok := d.(*Field)
			if !ok || !distributableUniversal(f.Value) {
				return false
			}
		}
		return true
	}
	return false
}

func covariantData(x Expr) bool {
	switch x := x.(type) {
	case nil, *Top, *Bottom, *BasicType, *Num, *String, *Bytes, *Bool, *Null, *TypeReference:
		return true
	case *BinaryExpr:
		return x.Op == AndOp && covariantData(x.X) && covariantData(x.Y)
	case *DisjunctionExpr:
		for _, d := range x.Values {
			if !covariantData(d.Val) {
				return false
			}
		}
		return true
	case *StructLit:
		for _, d := range x.Decls {
			f, ok := d.(*Field)
			if !ok || !covariantData(f.Value) {
				return false
			}
		}
		return true
	case *ListLit:
		for _, e := range x.Elems {
			if rest, ok := e.(*Ellipsis); ok {
				if !covariantData(rest.Value) {
					return false
				}
			} else if e, ok := e.(Expr); !ok || !covariantData(e) {
				return false
			}
		}
		return true
	}
	return false
}

func (r *TypeReference) evaluate(c *OpContext, state Flags) Value {
	env := c.Env(r.UpCount)
	if env.types != nil {
		if v := env.types.arguments[r.Param]; v != nil {
			return v
		}
	}
	return &Bottom{Src: r.Src, Code: IncompleteError,
		Err: c.Newf("uninstantiated type parameter %s", r.Param.Src.Name.Name)}
}

// typeParameters returns the uninstantiated telescope in lexical order.
// Nested functions inherit type arguments selected by the enclosing call;
// only a newly introduced quantifier generalizes them again.
func typeParameters(env *Environment) []*TypeParameter {
	if env == nil {
		return nil
	}
	params := typeParameters(env.Up)
	if s := env.types; s != nil {
		for _, p := range s.quantifier.Params {
			if s.arguments[p] == nil {
				params = append(params, p)
			}
		}
	}
	return params
}

// instantiateEnvironment copies lexical frames, never value cells. Type
// arguments are immutable predicates and cannot refine another use of the
// universal subject. Clearing the let cache prevents instance cross-talk.
func instantiateEnvironment(env *Environment, args map[*TypeParameter]Value) *Environment {
	if env == nil {
		return nil
	}
	up := instantiateEnvironment(env.Up, args)
	s := env.types
	if s != nil {
		for _, p := range s.quantifier.Params {
			if args[p] != nil {
				copy := *s
				copy.arguments = maps.Clone(s.arguments)
				if copy.arguments == nil {
					copy.arguments = make(map[*TypeParameter]Value)
				}
				for _, p := range s.quantifier.Params {
					if v := args[p]; v != nil {
						copy.arguments[p] = v
					}
				}
				s = &copy
				break
			}
		}
	}
	if up == env.Up && s == env.types {
		return env
	}
	copy := *env
	copy.Up, copy.types, copy.cache = up, s, nil
	return &copy
}

func (f *FuncValue) instantiate(c *OpContext, args map[*TypeParameter]Value) (*FuncValue, *Bottom) {
	env := instantiateEnvironment(f.Env, args)
	for e := env; e != nil; e = e.Up {
		if e.types == nil {
			continue
		}
		for _, p := range e.types.quantifier.Params {
			v := args[p]
			if v == nil || p.Bound == nil {
				continue
			}
			bound, _ := c.Evaluate(e, p.Bound)
			switch typeArgumentFits(c, bound, v) {
			case proofRefuted:
				return nil, c.NewErrf("type argument %s does not satisfy bound of %s", v, p.Src.Name.Name)
			case proofUnknown:
				return nil, &Bottom{Src: p.Src, Code: IncompleteError,
					Err: c.Newf("unresolved type argument bound for %s", p.Src.Name.Name)}
			}
		}
	}
	copy := *f
	copy.Env = env
	return &copy, nil
}

// typeArgumentFits is a sufficient inclusion check, with concrete witnesses
// for refutation. Compatibility of two incomplete predicates proves neither
// inclusion nor its negation.
func typeArgumentFits(c *OpContext, bound, arg Value) proofResult {
	bound, arg = Unwrap(bound), Unwrap(arg)
	if bound == nil || arg == nil {
		return proofUnknown
	}
	if bound == arg {
		return proofEstablished
	}
	switch b := bound.(type) {
	case *Top:
		return proofEstablished
	case *BasicType:
		if arg.Kind()&b.K == arg.Kind() {
			return proofEstablished
		}
		if _, ok := arg.(*BasicType); ok {
			return proofRefuted
		}
	}
	if d, ok := arg.(*Disjunction); ok {
		result := proofEstablished
		for _, v := range d.Values {
			switch typeArgumentFits(c, bound, v) {
			case proofRefuted:
				return proofRefuted
			case proofUnknown:
				result = proofUnknown
			}
		}
		return result
	}
	if b, ok := arg.(*Bottom); ok && !b.IsIncomplete() {
		return proofEstablished
	}
	return capabilityMember(c, nil, bound, arg)
}

func (f *FuncValue) inferInstance(c *OpContext, bindings []funcArg) (*FuncValue, *Bottom) {
	args := make(map[*TypeParameter]Value)
	for _, p := range typeParameters(f.Env) {
		args[p] = nil
	}
	// Data arguments constrain the input variables before callback schemes
	// are instantiated. A second pass handles result variables of callbacks.
	for pass := range 2 {
		for i, binding := range bindings {
			if binding.expr == nil {
				continue
			}
			v, _ := c.Evaluate(binding.env, binding.expr)
			if vertex, ok := v.(*Vertex); ok {
				vertex.Finalize(c)
			}
			_, callback := Unwrap(v).(*FuncValue)
			if callback != (pass == 1) {
				continue
			}
			inferTypeArguments(c, f.Env, f.Fn.Params[i].Value, v, args)
		}
	}
	for p, v := range args {
		if v == nil {
			return nil, &Bottom{Src: p.Src, Code: IncompleteError,
				Err: c.Newf("cannot infer type argument %s", p.Src.Name.Name)}
		}
	}
	return f.instantiate(c, args)
}

func inferTypeArguments(c *OpContext, env *Environment, pattern Expr, value Value, args map[*TypeParameter]Value) {
	if value == nil {
		return
	}
	switch p := pattern.(type) {
	case *TypeReference:
		// A known scalar contributes its immutable singleton predicate.
		// Carrying its activation vertex into later type instances would
		// incorrectly retain that argument cell's local sharing topology.
		if scalar := Unwrap(value); scalar != nil {
			value = scalar
		}
		if prev, ok := args[p.Param]; ok {
			if prev == nil {
				args[p.Param] = value
			} else if !Equal(c, prev, value, 0) {
				args[p.Param] = &Disjunction{Values: []Value{prev, value}}
			}
		}
	case *ListLit:
		v, ok := value.(*Vertex)
		if !ok || !v.IsList() {
			return
		}
		v.Finalize(c)
		elems := slices.Collect(v.Elems())
		for i, e := range p.Elems {
			if rest, ok := e.(*Ellipsis); ok {
				for _, item := range elems[min(i, len(elems)):] {
					inferTypeArguments(c, env, rest.Value, item, args)
				}
				break
			}
			if i < len(elems) {
				if e, ok := e.(Expr); ok {
					inferTypeArguments(c, env, e, elems[i], args)
				}
			}
		}
	case *StructLit:
		v, ok := value.(*Vertex)
		if !ok {
			return
		}
		v.Finalize(c)
		for _, d := range p.Decls {
			if field, ok := d.(*Field); ok {
				for _, arc := range v.Arcs {
					if arc.Label == field.Label {
						inferTypeArguments(c, env, field.Value, arc, args)
					}
				}
			}
		}
	case *Function:
		f, ok := Unwrap(value).(*FuncValue)
		if !ok {
			return
		}
		// Instantiate a polymorphic callback from the expected input types,
		// before reading its result predicate. This is elimination of that
		// callback's scheme, not generalization of a monomorphic callback.
		if params := typeParameters(f.Env); len(params) != 0 {
			callbackArgs := make(map[*TypeParameter]Value, len(params))
			for _, param := range params {
				callbackArgs[param] = nil
			}
			expectedEnv := instantiateEnvironment(env, args)
			for i, param := range p.Params {
				if i >= len(f.Fn.Params) {
					break
				}
				v, _ := c.Evaluate(expectedEnv, param.Value)
				if b, ok := Unwrap(v).(*Bottom); !ok || !b.IsIncomplete() {
					inferTypeArguments(c, f.Env, f.Fn.Params[i].Value, v, callbackArgs)
				}
			}
			if inst, b := f.instantiate(c, callbackArgs); b == nil {
				f = inst
			}
		}
		known := maps.Clone(args)
		for i, param := range p.Params {
			if i >= len(f.Fn.Params) {
				break
			}
			v, _ := c.Evaluate(f.Env, f.Fn.Params[i].Value)
			inferTypeArguments(c, env, param.Value, v, args)
		}
		// Input bounds are upper bounds on an instance already learned from
		// data. They must not widen that instance and discard refinements.
		for param, v := range known {
			if v != nil {
				args[param] = v
			}
		}
		if f.Fn.Ret != nil {
			v, _ := c.Evaluate(f.Env, f.Fn.Ret)
			inferTypeArguments(c, env, p.Ret, v, args)
		}
	}
}

// fixedTypeExpression extends the ground counterexample vocabulary only
// with lexical type parameters. Ordinary references can still be refined,
// and evaluating them speculatively could refute just one possible witness.
func fixedTypeExpression(x Expr) bool {
	if fixedCapabilityExpr(x) {
		return true
	}
	switch x := x.(type) {
	case *TypeReference:
		return true
	case *BinaryExpr:
		return x.Op == AndOp && fixedTypeExpression(x.X) && fixedTypeExpression(x.Y)
	case *DisjunctionExpr:
		for _, d := range x.Values {
			if !fixedTypeExpression(d.Val) {
				return false
			}
		}
		return true
	}
	return false
}

// groundSignature is a temporary proof view. It does not replace or narrow
// the stored universal clause, and never changes a closure's code identity.
func groundSignature(c *OpContext, f *FuncValue) *FuncValue {
	fn := *f.Fn
	fn.Params = slices.Clone(fn.Params)
	ground := func(x Expr) (Expr, bool) {
		if !fixedTypeExpression(x) {
			return nil, false
		}
		if x == nil {
			return nil, true
		}
		v, _ := c.Evaluate(f.Env, x)
		if v == nil {
			return nil, false
		}
		v = Unwrap(v)
		return v, fixedCapabilityExpr(v)
	}
	var ok bool
	if fn.Ret, ok = ground(fn.Ret); !ok {
		return nil
	}
	for i := range fn.Params {
		p := &fn.Params[i]
		if p.Value, ok = ground(p.Value); !ok {
			return nil
		}
		if p.Default, ok = ground(p.Default); !ok {
			return nil
		}
	}
	copy := *f
	copy.Fn = &fn
	return &copy
}

// genericInstances visits a finite family solely to find counterexamples.
// No amount of successful enumeration establishes a universal proposition.
func genericInstances(c *OpContext, f *FuncValue, visit func(*FuncValue) bool) {
	params := typeParameters(f.Env)
	if len(params) == 0 {
		visit(f)
		return
	}
	values := []Value{&BasicType{K: IntKind}, &BasicType{K: StringKind},
		&BasicType{K: BoolKind}, &String{Str: "x"}, &Bool{B: true}}
	for _, s := range []string{"0", "1", "2", "-1", "1.5"} {
		n := &Num{K: IntKind}
		n.X.SetString(s)
		if s == "1.5" {
			n.K = FloatKind
		}
		values = append(values, n)
	}
	budget := 64
	args := make(map[*TypeParameter]Value)
	var search func(int) bool
	search = func(i int) bool {
		if budget == 0 {
			return false
		}
		if i == len(params) {
			budget--
			if inst, b := f.instantiate(c, args); b == nil {
				return visit(inst)
			}
			return true
		}
		for _, v := range values {
			args[params[i]] = v
			if !search(i + 1) {
				return false
			}
		}
		return true
	}
	search(0)
}

func refuteGenericFunction(c *OpContext, f *FuncValue) (err *Bottom) {
	if !probeBody(f.Fn.Body) {
		return nil
	}
	genericInstances(c, f, func(inst *FuncValue) bool {
		if view := groundSignature(c, inst); view != nil {
			err = refuteCapability(c, view, FuncType{Fn: view.Fn, Env: view.Env})
		}
		return err == nil
	})
	return err
}

func refuteGenericIntersection(c *OpContext, a, b FuncType) (err *Bottom) {
	budget := 64
	genericInstances(c, &FuncValue{Fn: a.Fn, Env: a.Env}, func(x *FuncValue) bool {
		x = groundSignature(c, x)
		if x == nil {
			return true
		}
		genericInstances(c, &FuncValue{Fn: b.Fn, Env: b.Env}, func(y *FuncValue) bool {
			if budget == 0 {
				return false
			}
			budget--
			if y = groundSignature(c, y); y != nil {
				err = refuteArrowIntersection(c, FuncType{Fn: x.Fn, Env: x.Env},
					FuncType{Fn: y.Fn, Env: y.Env})
			}
			return err == nil
		})
		return err == nil && budget > 0
	})
	return err
}

func refuteGenericCapability(c *OpContext, impl *FuncValue, t FuncType) (err *Bottom) {
	if !probeBody(impl.Fn.Body) {
		return nil
	}
	genericInstances(c, &FuncValue{Fn: t.Fn, Env: t.Env}, func(target *FuncValue) bool {
		target = groundSignature(c, target)
		if target == nil {
			return true
		}
		view := impl
		if params := typeParameters(impl.Env); len(params) != 0 {
			args := make(map[*TypeParameter]Value, len(params))
			for _, p := range params {
				args[p] = nil
			}
			matches := matchFuncParams(target.Fn, impl.Fn, false)
			for i, j := range matches {
				if j >= 0 && target.Fn.Params[i].Value != nil {
					v, _ := c.Evaluate(target.Env, target.Fn.Params[i].Value)
					inferTypeArguments(c, impl.Env, impl.Fn.Params[j].Value, v, args)
				}
			}
			inst, b := impl.instantiate(c, args)
			if b != nil {
				return true
			}
			view = inst
		}
		if view = groundSignature(c, view); view != nil {
			err = refuteCapability(c, view, FuncType{Fn: target.Fn, Env: target.Env})
		}
		return err == nil
	})
	return err
}
