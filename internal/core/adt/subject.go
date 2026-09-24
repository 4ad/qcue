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
)

// subjectScheme records one lexical introduction, independently of the
// subject's evaluated kind. Normalizing data must not erase formation or
// elimination information.
// origin identifies the introduction independently of its selected instances.
type subjectScheme struct {
	origin *Environment
	env    *Environment
	// An excluded clause remains an obligation on the shared subject, but
	// no longer participates in subsequent selections of this view.
	excluded bool
}

type compositeSelection struct {
	subject  *Vertex
	argument Value
}

func retainSubjectIntroduction(c *OpContext, value Value, env *Environment) Value {
	if _, ok := Unwrap(value).(*FuncValue); ok {
		return value // A callable retains its telescopes on its scoped clauses.
	}
	// A subject view owns its introduction. Sharing and graph equality must
	// preserve this metadata, just as they preserve its data constraints.
	vertex := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, value))
	vertex.schemes = []subjectScheme{{origin: env, env: env}}
	vertex.Finalize(c)
	if b := vertex.Bottom(); b != nil && !b.IsIncomplete() {
		// This private normalization vertex is not a child of the caller.
		// Expose its contradiction at the quantifier boundary.
		copy := *b
		copy.ChildError, copy.HasRecursive = false, false
		return &copy
	}
	return vertex
}

// sameSubjectSchemes is a sufficient equality check for constraint-graph
// sharing. Runtime data equality is weaker: equal data may retain different
// formation obligations or different remaining selection telescopes.
func sameSubjectSchemes(c *OpContext, a, b []subjectScheme) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.ContainsFunc(b, func(y subjectScheme) bool {
			return x.excluded == y.excluded && sameSubjectScope(c, x.origin, y.origin) &&
				sameSubjectScope(c, x.env, y.env)
		}) {
			return false
		}
	}
	return true
}

func sameSubjectScope(c *OpContext, a, b *Environment) bool {
	if sameTypeEnvironment(c, a, b) {
		return true
	}
	if a == nil || b == nil || a.types == nil || b.types == nil ||
		a.types.quantifier != b.types.quantifier || !slices.Equal(typeParameters(a), typeParameters(b)) {
		return false
	}
	equal := func(x, y Value) bool {
		if x == y {
			return true
		}
		if v, ok := x.(*Vertex); ok && v.HasSubjectSchemes() {
			return Equal(c, x, y, CheckStructural)
		}
		if v, ok := y.(*Vertex); ok && v.HasSubjectSchemes() {
			return Equal(c, x, y, CheckStructural)
		}
		x, y = Unwrap(x), Unwrap(y)
		return fixedCapabilityExpr(x) && fixedCapabilityExpr(y) && Equal(c, x, y, CheckStructural)
	}
	if !maps.EqualFunc(a.types.arguments, b.types.arguments, equal) ||
		!maps.Equal(a.types.erasedIndices, b.types.erasedIndices) {
		return false
	}
	// Erased arguments contribute to formation even when unused. Finite
	// value assignments contribute only through free references: for example
	// an unused enclosing x=0 versus x=1 cannot distinguish two vacuous
	// universals over the same empty range.
	erased := func(env *Environment) map[*TypeParameter]Value {
		out := make(map[*TypeParameter]Value)
		for e := env; e != nil; e = e.Up {
			if e.types != nil {
				for p, value := range e.types.arguments {
					if p.ValueRange == nil {
						out[p] = value
					}
				}
			}
		}
		return out
	}
	if !maps.EqualFunc(erased(a.Up), erased(b.Up), equal) {
		return false
	}
	q := a.types.quantifier
	if !slices.ContainsFunc(q.Params, func(p *TypeParameter) bool { return p.ValueRange == nil }) {
		// A completed finite expansion stores the entire meet in the
		// subject's constraints. Those constraints are compared separately;
		// this metadata adds only the remaining elimination domain. Requiring
		// equal captured body inputs here would distinguish equal finite
		// normal forms, including vacuous quantifiers over an empty range.
		return true
	}
	predicate := &Existential{Template: q, Env: a.Up}
	return predicate.sameEnvironment(c, b.Up)
}

// SubjectSelection exposes an exact retained type elimination. Additional
// refinements remain conjuncts on its containing vertex.
func (v *Vertex) SubjectSelection() (*Vertex, Value) {
	if s := v.DerefValue().subjectSelection; s != nil {
		return s.subject, s.argument
	}
	return nil, nil
}

func instantiateSubject(c *OpContext, subject *Vertex, argument Expr) (*Vertex, bool) {
	subject = subject.DerefValue()
	args := make(map[*TypeParameter]Value)
	selected := make(map[*Environment]bool)
	var err *Bottom
	var arg Value
	found := false
	for _, s := range subject.schemes {
		params := typeParameters(s.env)
		if s.excluded || len(params) == 0 {
			continue
		}
		if !found {
			arg, _ = c.Evaluate(c.Env(0), argument)
			found = true
		}
		binding := map[*TypeParameter]Value{params[0]: arg}
		if _, b := (&FuncValue{Env: s.env}).instantiate(c, binding); b != nil {
			if err == nil || b.IsIncomplete() {
				err = b
			}
			continue
		}
		args[params[0]] = arg
		selected[s.origin] = true
	}
	if len(selected) != 0 {
		view := instantiateSubjectView(c, subject, args, make(map[*Vertex]*Vertex), nil)
		for i := range view.schemes {
			s := &view.schemes[i]
			s.excluded = s.excluded || !selected[s.origin]
		}
		view.subjectSelection = &compositeSelection{subject, arg}
		retainProjectionObligations(view, view, make(map[*Vertex]bool))
		return view, true
	}
	if found {
		c.AddBottom(err)
		return emptyNode, true
	}
	return nil, false
}

// The overlay is merged with the original graph before its projected method
// provenance is complete. Record all obligations entailed by that merge now;
// later refinements will be distinguishable during export. Capturing only the
// pre-merge clauses mistakes re-scoped original clauses for new eliminations.
func retainProjectionObligations(v, root *Vertex, seen map[*Vertex]bool) {
	v = v.DerefValue()
	if seen[v] {
		return
	}
	seen[v] = true
	if f, ok := v.BaseValue.(*FuncValue); ok && f.projection != nil {
		if projectionRoot(f.projection.expr) == root {
			f.projection.types = slices.Clone(f.Types)
		}
	}
	for _, a := range v.Arcs {
		retainProjectionObligations(a, root, seen)
	}
}

func projectionRoot(expr Expr) *Vertex {
	switch x := expr.(type) {
	case *Vertex:
		return x
	case *SelectorExpr:
		return projectionRoot(x.X)
	case *IndexExpr:
		return projectionRoot(x.X)
	}
	return nil
}

// instantiateSubjectView overlays selected function views on the original
// subject. It never reevaluates a record constructor or allocates independent
// witnesses for its data fields. Keeping the original conjunct also retains
// presence, closedness, and every universal obligation.
func instantiateSubjectView(c *OpContext, subject *Vertex, args map[*TypeParameter]Value, seen map[*Vertex]*Vertex, projection Expr) *Vertex {
	if v := seen[subject]; v != nil {
		return v
	}
	subject.Finalize(c)
	if f, ok := Unwrap(subject).(*FuncValue); ok {
		copy := *f
		copy.Env = instantiateEnvironment(f.Env, args)
		copy.Types = slices.Clone(f.Types)
		changed := copy.Env != f.Env
		for i, t := range copy.Types {
			copy.Types[i].Env = instantiateEnvironment(t.Env, args)
			changed = changed || copy.Types[i].Env != t.Env
		}
		if !changed {
			return subject
		}
		copy.frontier = slices.Clone(f.selectionClauses())
		for i := range copy.frontier {
			copy.frontier[i].Env = instantiateEnvironment(copy.frontier[i].Env, args)
		}
		copy.selection = nil
		copy.Types = mergeFuncTypes(copy.Types, f.selectionAndOriginalClauses())
		copy.projection = &functionProjection{expr: projection, types: copy.Types}
		v := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, &copy))
		v.Finalize(c)
		return v
	}
	view := c.newInlineVertex(nil, nil)
	seen[subject] = view
	if projection == nil {
		// The caller installs SubjectSelection on this root. Exporting a
		// projected method can then reconstruct the complete elimination,
		// rather than printing specialized and original clauses as peers.
		projection = view
	}
	for _, s := range subject.schemes {
		s.env = instantiateEnvironment(s.env, args)
		view.schemes = append(view.schemes, s)
	}
	if subject.Kind()&(StructKind|ListKind) == 0 {
		view.Conjuncts = []Conjunct{MakeRootConjunct(nil, subject)}
		view.Finalize(c)
		return view
	}
	var overlay Expr
	if subject.IsList() {
		list := &ListLit{}
		for a := range subject.Elems() {
			path := &IndexExpr{X: projection, Index: c.NewInt64(int64(a.Label.Index()))}
			list.Elems = append(list.Elems, instantiateSubjectView(c, a, args, seen, path))
		}
		if !subject.IsClosedList() {
			list.Elems = append(list.Elems, &Ellipsis{})
		}
		overlay = list
	} else {
		record := &StructLit{}
		for _, a := range subject.Arcs {
			if a.ArcType != ArcMember || a.Label.IsLet() {
				continue
			}
			record.Decls = append(record.Decls, &Field{Label: a.Label,
				Value: instantiateSubjectView(c, a, args, seen, &SelectorExpr{X: projection, Sel: a.Label})})
		}
		overlay = record
	}
	view.Conjuncts = []Conjunct{MakeRootConjunct(nil, overlay), MakeRootConjunct(nil, subject)}
	view.Finalize(c)
	return view
}
