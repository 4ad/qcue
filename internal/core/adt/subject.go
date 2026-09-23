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

// subjectScheme records one lexical introduction on a composite subject.
// origin identifies the introduction independently of its selected instances.
type subjectScheme struct {
	origin *Environment
	env    *Environment
	// An excluded clause remains an obligation on the shared subject, but
	// no longer participates in subsequent selections of this view.
	excluded bool
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
		view := instantiateSubjectView(c, subject, args, make(map[*Vertex]*Vertex))
		for i := range view.schemes {
			s := &view.schemes[i]
			s.excluded = s.excluded || !selected[s.origin]
		}
		return view, true
	}
	if found {
		c.AddBottom(err)
		return emptyNode, true
	}
	return nil, false
}

// instantiateSubjectView overlays selected function views on the original
// subject. It never reevaluates a record constructor or allocates independent
// witnesses for its data fields. Keeping the original conjunct also retains
// presence, closedness, and every universal obligation.
func instantiateSubjectView(c *OpContext, subject *Vertex, args map[*TypeParameter]Value, seen map[*Vertex]*Vertex) *Vertex {
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
		copy.Types = mergeFuncTypes(copy.Types, f.selectionAndOriginalClauses())
		v := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, &copy))
		v.Finalize(c)
		return v
	}
	if subject.Kind()&(StructKind|ListKind) == 0 {
		return subject
	}
	view := c.newInlineVertex(nil, nil)
	seen[subject] = view
	for _, s := range subject.schemes {
		s.env = instantiateEnvironment(s.env, args)
		view.schemes = append(view.schemes, s)
	}
	var overlay Expr
	if subject.IsList() {
		list := &ListLit{}
		for a := range subject.Elems() {
			list.Elems = append(list.Elems, instantiateSubjectView(c, a, args, seen))
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
				Value: instantiateSubjectView(c, a, args, seen)})
		}
		overlay = record
	}
	view.Conjuncts = []Conjunct{MakeRootConjunct(nil, overlay), MakeRootConjunct(nil, subject)}
	view.Finalize(c)
	return view
}
