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

// subjectScheme records one lexical introduction on a composite subject.
// origin identifies the introduction independently of its selected instances.
type subjectScheme struct {
	origin *Environment
	env    *Environment
}

func instantiateSubject(c *OpContext, subject *Vertex, argument Expr) (*Vertex, bool) {
	subject = subject.DerefValue()
	for _, s := range subject.schemes {
		params := typeParameters(s.env)
		if len(params) == 0 {
			continue
		}
		arg, _ := c.Evaluate(c.Env(0), argument)
		args := map[*TypeParameter]Value{params[0]: arg}
		if _, b := (&FuncValue{Env: s.env}).instantiate(c, args); b != nil {
			c.AddBottom(b)
			return emptyNode, true
		}
		return instantiateSubjectView(c, subject, args, make(map[*Vertex]*Vertex)), true
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
		if copy.Env == f.Env {
			return subject
		}
		copy.Types = mergeFuncTypes(copy.Types, []FuncType{{Fn: f.Fn, Env: f.Env}})
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
		view.schemes = append(view.schemes, subjectScheme{s.origin, instantiateEnvironment(s.env, args)})
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
