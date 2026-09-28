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
// selected. All type parameters range over the same impredicative sort.
type TypeParameter struct {
	Src   *ast.TypeParam
	Bound Expr
	// References retains lexical dependencies of the bound.
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

// TypeReference denotes a universally bound predicate. Ordinary source fields
// used as types instead retain live description coordinates through LiveReference.
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
	args := make([]Value, len(a.Args))
	for i, x := range a.Args {
		v, _ := c.Evaluate(c.Env(0), x)
		if v == nil {
			return nil
		}
		args[i] = v
	}
	env, b := a.Expand(c, c.Env(0), args)
	if b != nil {
		return b
	}
	v, _ := c.Evaluate(env, a.Template.Body)
	return v
}

// Expand substitutes checked predicate arguments into an abbreviation's
// lexical scope without executing its body. Proof synthesis uses this same
// substitution and bound check, with its own symbolic caller environment.
func (a *AliasApplication) Expand(c *OpContext, caller *Environment, arguments []Value) (*Environment, *Bottom) {
	if len(arguments) != len(a.Template.Params) {
		return nil, c.NewErrf("incorrect number of alias arguments")
	}
	args := make(map[*TypeParameter]Value, len(arguments))
	for i, value := range arguments {
		args[a.Template.Params[i]] = value
	}
	outer := caller
	for range a.UpCount {
		outer = outer.Up
	}
	scope := c.newInlineVertex(nil, &StructMarker{})
	env := &Environment{Up: outer, Vertex: scope,
		types: &typeScope{quantifier: a.Template}}
	for e := caller; e != nil; e = e.Up {
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
		return nil, b
	}
	return inst.Env, nil
}

// ErasedRuntimeIndex reports whether this scoped occurrence can only select
// a type instance. Alias substitutions retain their own occurrence markers;
// a fixed argument at another use of the alias is independent.
func (index *IndexExpr) ErasedRuntimeIndex(env *Environment) bool {
	if index.ErasedIndex {
		return true
	}
	for e := env; e != nil; e = e.Up {
		if e.types != nil && e.types.erasedIndices[index] {
			return true
		}
	}
	return false
}

func (q *Quantified) evaluate(c *OpContext, state Flags) Value {
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
	if v == nil {
		return nil
	}
	return retainSubjectIntroduction(c, v, env)
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

// CheckingScope introduces a telescope in a symbolic checking environment.
// It neither substitutes a witness nor normalizes the quantified body, so a
// source checker can inspect every operation before eager refutation.
func (q *Quantified) CheckingScope(c *OpContext, outer *Environment) *Environment {
	return &Environment{Up: outer, Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: q}}
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
		if list, ok := x.(*ListLit); ok {
			copy := *list
			copy.Elems = slices.Clone(list.Elems)
			for i, elem := range copy.Elems {
				if value, ok := elem.(Expr); ok {
					copy.Elems[i] = universalDataMinimum(c, value, params).(Elem)
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
	case *ListLit:
		for _, elem := range x.Elems {
			value, ok := elem.(Expr)
			if !ok || !distributableUniversal(value) {
				return false
			}
		}
		return true
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
			if b := p.checkTypeArgument(c, e, v); b != nil {
				return nil, b
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
	types    []FuncType // obligations already entailed by this selection
}

// SelectedProjection exposes the source of a method specialized by selecting
// its containing record or list. Its remaining telescope is not recoverable
// from the unordered set of retained proof obligations alone.
type functionProjection struct {
	expr  Expr
	types []FuncType
}

func (f *FuncValue) SelectedProjection() (Expr, []FuncType) {
	if f.projection == nil {
		return nil, nil
	}
	var extra []FuncType
	for _, t := range f.Types {
		if !slices.Contains(f.projection.types, t) {
			extra = append(extra, t)
		}
	}
	return f.projection.expr, extra
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
	if f.frontier != nil {
		return f.frontier
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
	copy.projection = nil
	copy.frontier = []FuncType{}
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
		copy.frontier = append(copy.frontier, view)
		if t.Fn == f.Fn && t.Env == f.Env {
			copy.Env = inst.Env
		}
	}
	if len(copy.frontier) == 0 {
		return nil, err
	}
	copy.selection = selected
	if len(f.callViews) != 0 {
		copy.callViews = nil
		for _, view := range f.callViews {
			if selected, b := view.selectType(c, argument); b == nil && selected != nil {
				copy.callViews = append(copy.callViews, selected)
			}
		}
	}
	copy.Types = mergeFuncTypes(f.Types, []FuncType{{Fn: f.Fn, Env: f.Env}})
	copy.Types = mergeFuncTypes(copy.Types, copy.frontier)
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
	case *Null, *Bool, *Num, *String, *Bytes:
		return capabilityMember(c, nil, bound, arg)
	}
	return proofUnknown
}
