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
	Src           *ast.TypeParam
	Bound         Expr
	Level         int
	ExplicitLevel bool
	// ValueRange is non-nil for the finite value-binder fragment. General
	// dependent ranges and signatures remain outside this profile.
	ValueRange Expr
	// References records lexical dependencies of the bound or value range
	// for exporting a telescope at a different lexical position.
	References []Expr
}

// Quantified is a retained predicate template over one subject.
type Quantified struct {
	Src    *ast.Quantifier
	Params []*TypeParameter
	Body   Expr
	// References are free references in the surrounding environment. They
	// include erased predicates so residual templates can be exported with
	// their lexical dependencies intact.
	References []Expr
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
	quantifier    *Quantified
	arguments     map[*TypeParameter]Value
	finite        *finiteExpansionBudget
	erasedIndices map[*IndexExpr]bool
}

// AliasApplication substitutes predicates into an abbreviation. It creates
// no shared universal subject and binds arguments in the caller's scope.
type AliasApplication struct {
	Src      *ast.CallExpr
	Template *Quantified
	UpCount  int32
	Args     []Expr
	// ErasedIndices guards runtime indexing only for this substitution.
	// The underlying template remains shared with fixed-argument uses.
	ErasedIndices map[*IndexExpr]bool
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
	for e := c.Env(0); e != nil; e = e.Up {
		if e.types != nil && len(e.types.erasedIndices) != 0 {
			if env.types.erasedIndices == nil {
				env.types.erasedIndices = make(map[*IndexExpr]bool)
			}
			maps.Copy(env.types.erasedIndices, e.types.erasedIndices)
		}
	}
	if len(a.ErasedIndices) != 0 {
		if env.types.erasedIndices == nil {
			env.types.erasedIndices = make(map[*IndexExpr]bool)
		}
		maps.Copy(env.types.erasedIndices, a.ErasedIndices)
	}
	f := &FuncValue{Env: env}
	inst, b := f.instantiate(c, args)
	if b != nil {
		return b
	}
	v, _ := c.Evaluate(inst.Env, a.Template.Body)
	return v
}

func (c *OpContext) erasedAliasIndex(index *IndexExpr) bool {
	for e := c.Env(0); e != nil; e = e.Up {
		if e.types != nil && e.types.erasedIndices[index] {
			return true
		}
	}
	return false
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
	env := q.lexicalScope(c)
	// Universals commute with fixed record projections. Reduce covariant
	// data fields even when a sibling field is a generic function. Never
	// substitute bottom through an arrow's negative domain.
	params := make(map[*TypeParameter]bool)
	for _, p := range typeParameters(env) {
		params[p] = true
	}
	body := universalDataMinimum(c, q.Body, params)
	v, _ := c.Evaluate(env, body)
	if vertex, ok := v.(*Vertex); ok && vertex.Kind()&(StructKind|ListKind) != 0 {
		if !slices.ContainsFunc(vertex.schemes, func(s subjectScheme) bool { return s.origin == env }) {
			vertex.schemes = append(vertex.schemes, subjectScheme{origin: env, env: env})
		}
	}
	return v
}

// Re-evaluation of one lexical introduction must not create another
// telescope on the same subject. Runtime witness cells identify its scope;
// equal approximations from separate activations do not establish identity.
func (q *Quantified) lexicalScope(c *OpContext) *Environment {
	outer := c.Env(0)
	key := quantifiedScopeKey{q: q}
	if outer != nil {
		key.vertex = outer.DerefVertex(c)
	}
	for _, env := range c.quantifiedScopes[key] {
		if sameTypeEnvironment(c, env.Up, outer) {
			return env
		}
	}
	env := &Environment{Up: outer, Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: q}}
	if c.quantifiedScopes == nil {
		c.quantifiedScopes = make(map[quantifiedScopeKey][]*Environment)
	}
	c.quantifiedScopes[key] = append(c.quantifiedScopes[key], env)
	return env
}

type quantifiedScopeKey struct {
	q      *Quantified
	vertex *Vertex
}

func universalDataMinimum(c *OpContext, x Expr, params map[*TypeParameter]bool) Expr {
	if !covariantData(x) {
		if record, ok := x.(*StructLit); ok {
			copy := *record
			copy.Decls = slices.Clone(record.Decls)
			for i, d := range copy.Decls {
				if field, ok := d.(*Field); ok {
					f := *field
					f.Value = universalDataMinimum(c, f.Value, params)
					copy.Decls[i] = &f
				}
			}
			return &copy
		}
		return x
	}
	switch x := x.(type) {
	case *TypeReference:
		if params[x.Param] {
			return &Bottom{Src: x.Src, Code: EvalError,
				Err: c.Newf("universal type parameter has an empty instance")}
		}
	case *StructLit:
		copy := *x
		copy.Decls = slices.Clone(x.Decls)
		for i, d := range x.Decls {
			f := *d.(*Field)
			f.Value = universalDataMinimum(c, f.Value, params)
			copy.Decls[i] = &f
		}
		return &copy
	case *ListLit:
		copy := *x
		copy.Elems = slices.Clone(x.Elems)
		for i, e := range x.Elems {
			if rest, ok := e.(*Ellipsis); ok {
				r := *rest
				r.Value = universalDataMinimum(c, rest.Value, params)
				copy.Elems[i] = &r
			} else {
				copy.Elems[i] = universalDataMinimum(c, e.(Expr), params).(Elem)
			}
		}
		return &copy
	case *BinaryExpr:
		copy := *x
		copy.X = universalDataMinimum(c, x.X, params)
		copy.Y = universalDataMinimum(c, x.Y, params)
		return &copy
	case *DisjunctionExpr:
		copy := *x
		copy.Values = slices.Clone(x.Values)
		for i, d := range x.Values {
			copy.Values[i].Val = universalDataMinimum(c, d.Val, params)
		}
		return &copy
	}
	return x
}

// Limit work, rather than the size of the result alone: many different
// assignments can yield the same value. Exhaustion retains the entire scoped
// predicate, never a partially enumerated conjunction or disjunction.
const finiteExpansionLimit = 1024

type finiteExpansionBudget struct {
	remaining int
	exhausted bool
}

func (q *Quantified) finiteResidual(env *Environment) Value {
	if q.Src.Exists {
		return &Existential{Template: q, Env: env}
	}
	return &Universal{Template: q, Env: env}
}

func (q *Quantified) evaluateFinite(c *OpContext) Value {
	outer := c.Env(0)
	for _, p := range q.Params {
		if p.ValueRange == nil {
			// Mixed prefixes are retained rather than commuting type and
			// value scopes to force a finite expansion.
			return q.finiteResidual(outer)
		}
	}
	budget := c.finiteExpansion
	for env := outer; budget == nil && env != nil; env = env.Up {
		if env.types != nil {
			budget = env.types.finite
		}
	}
	if budget == nil {
		budget = &finiteExpansionBudget{remaining: finiteExpansionLimit}
	}
	saved := c.finiteExpansion
	c.finiteExpansion = budget
	defer func() { c.finiteExpansion = saved }()
	// A literal body is independent of every binder. Check each fixed range
	// for emptiness, but one assignment suffices for each nonempty range.
	independent := false
	switch q.Body.(type) {
	case *Top, *Bottom, *BasicType, *Null, *Bool, *Num, *String, *Bytes:
		independent = true
	}
	for _, p := range q.Params {
		independent = independent && fixedCapabilityExpr(p.ValueRange)
	}
	var expand func(*Environment, int) Value
	expand = func(env *Environment, i int) Value {
		if budget.remaining == 0 || budget.exhausted {
			budget.exhausted = true
			return nil
		}
		budget.remaining--
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
		if independent {
			candidates = candidates[:1]
		}
		values := make([]Value, 0, len(candidates))
		for _, v := range candidates {
			args := maps.Clone(env.types.arguments)
			args[p] = v
			frame := *env
			frame.types = &typeScope{quantifier: q, arguments: args, finite: budget}
			frame.cache = nil
			value := expand(&frame, i+1)
			if value == nil || budget.exhausted {
				return nil
			}
			values = append(values, value)
		}
		if q.Src.Exists {
			return &Disjunction{Values: values}
		}
		return &Conjunction{Values: values}
	}
	env := &Environment{Up: outer, Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: q, arguments: make(map[*TypeParameter]Value), finite: budget}}
	value := expand(env, 0)
	if budget.exhausted {
		return q.finiteResidual(outer)
	}
	return value
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
	case *BoundExpr, *BoundValue:
		return fixedCapabilityExpr(x)
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
			if v == nil {
				continue
			}
			if p.ValueRange != nil {
				switch capabilityMember(c, e, p.ValueRange, v) {
				case proofRefuted:
					return nil, c.NewErrf("value argument %s is outside the range of %s", v, p.Src.Name.Name)
				case proofUnknown:
					return nil, &Bottom{Src: p.Src, Code: IncompleteError,
						Err: c.Newf("unresolved value argument range for %s", p.Src.Name.Name)}
				}
				continue
			}
			if b := checkTypeUniverse(c, p, v); b != nil {
				return nil, b
			}
			if p.Bound == nil {
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

type functionSelection struct {
	subject  *FuncValue
	argument Value
	clauses  []FuncType
	types    []FuncType // obligations already entailed by this selection
}

// TypeSelection exposes the retained elimination for faithful source export.
func (f *FuncValue) TypeSelection() (subject *FuncValue, argument Value, extra []FuncType) {
	if s := f.selection; s != nil {
		for _, t := range f.Types {
			if !slices.Contains(s.types, t) {
				extra = append(extra, t)
			}
		}
		return s.subject, s.argument, extra
	}
	return nil, nil, nil
}

func (f *FuncValue) selectionClauses() []FuncType {
	if f.selection != nil {
		return f.selection.clauses
	}
	return f.selectionAndOriginalClauses()
}

// A selected view retains the original clauses as obligations. Checks on the
// whole descriptor (such as universe formation) must not use only the selected
// telescope that the next type application consumes.
func (f *FuncValue) selectionAndOriginalClauses() []FuncType {
	return append([]FuncType{{Fn: f.Fn, Env: f.Env}}, f.Types...)
}

func (f *FuncValue) hasTypeSelection() bool {
	for _, t := range f.selectionClauses() {
		if len(typeParameters(t.Env)) != 0 {
			return true
		}
	}
	return false
}

func (f *FuncValue) selectType(c *OpContext, argument Value) (*FuncValue, *Bottom) {
	copy := *f
	selected := &functionSelection{subject: f, argument: argument}
	var err *Bottom
	for _, t := range f.selectionClauses() {
		params := typeParameters(t.Env)
		if len(params) == 0 {
			continue
		}
		inst, b := (&FuncValue{Fn: t.Fn, Env: t.Env}).instantiate(c,
			map[*TypeParameter]Value{params[0]: argument})
		if b != nil {
			// Other retained clauses may admit the argument. An unknown
			// bound takes precedence if none establishes an instance.
			if err == nil || b.IsIncomplete() {
				err = b
			}
			continue
		}
		view := t
		view.Env = inst.Env
		selected.clauses = append(selected.clauses, view)
		if t.Fn == f.Fn && t.Env == f.Env {
			copy.Env = inst.Env
		}
	}
	if len(selected.clauses) == 0 {
		return nil, err
	}
	copy.selection = selected
	copy.Types = mergeFuncTypes(f.Types, []FuncType{{Fn: f.Fn, Env: f.Env}})
	copy.Types = mergeFuncTypes(copy.Types, selected.clauses)
	selected.types = copy.Types
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
	if c.provesInclusion(bound, arg) {
		return proofEstablished
	}
	// Only concrete scalar singletons can use ordinary value membership
	// as an inclusion check. A record literal is an extensible predicate,
	// and conjoining a function contract does not prove its implementation.
	switch arg.(type) {
	case *Null, *Bool, *Num, *String, *Bytes, *OpaqueValue:
		return capabilityMember(c, nil, bound, arg)
	}
	return proofUnknown
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
		if len(typeParameters(impl.Env)) != 0 {
			bindings := make([]funcArg, len(impl.Fn.Params))
			for i, j := range matchFuncParams(target.Fn, impl.Fn, false) {
				if j >= 0 {
					bindings[j] = funcArg{expr: target.Fn.Params[i].Value, env: target.Env}
				}
			}
			inst, b := impl.inferInstance(c, bindings)
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
