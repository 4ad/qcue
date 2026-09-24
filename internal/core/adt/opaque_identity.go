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

// publicExport is an identity in a seal's declared export graph. Predicate
// sharing does not share nodes: two fields using the same type abbreviation
// get distinct nodes. Explicit references between exported values identify
// nodes, independently of the private implementation later assigned to them.
type publicExport struct {
	schema *Vertex
	parent *publicExport
	label  Feature
	fields map[Feature]*publicExport
	alias  *publicExport
}

func (x *publicExport) canonical() *publicExport {
	for x != nil && x.alias != nil {
		x = x.alias
	}
	return x
}

func (x *publicExport) field(label Feature) *publicExport {
	if x == nil {
		return nil
	}
	x = x.canonical()
	if child := x.fields[label]; child != nil {
		return child
	}
	// A pattern or open list can declare exports whose labels become known
	// only during projection. Their identity is still the public path.
	child := &publicExport{parent: x, label: label, fields: make(map[Feature]*publicExport)}
	x.fields[label] = child
	return child
}

func identifyExports(a, b *publicExport) {
	a, b = a.canonical(), b.canonical()
	if a == nil || b == nil || a == b {
		return
	}
	a.alias = b
	for label, child := range a.fields {
		if other := b.fields[label]; other != nil {
			identifyExports(child, other)
		} else {
			b.fields[label] = child
		}
	}
}

func publicExportGraph(c *OpContext, schema *Vertex) *publicExport {
	var nodes []*publicExport
	active := make(map[*Vertex]bool)
	var build func(*Vertex, *publicExport) *publicExport
	build = func(v *Vertex, parent *publicExport) *publicExport {
		n := &publicExport{schema: v, parent: parent, fields: make(map[Feature]*publicExport)}
		nodes = append(nodes, n)
		v = v.DerefValue()
		if active[v] {
			return n // Recursive transports require an independent totality proof.
		}
		active[v] = true
		defer delete(active, v)
		for _, a := range v.Arcs {
			if !a.Label.IsDef() && !a.Label.IsLet() {
				n.fields[a.Label] = build(a, n)
				n.fields[a.Label].label = a.Label
			}
		}
		return n
	}
	root := build(schema, nil)
	for _, n := range nodes {
		var visit func(*Environment, Expr)
		visit = func(env *Environment, expr Expr) {
			if b, ok := expr.(*BinaryExpr); ok && b.Op == AndOp {
				visit(env, b.X)
				visit(env, b.Y)
				return
			}
			identifyExports(n, n.reference(c, env, expr, make(map[Expr]bool)))
		}
		for conjunct := range n.schema.LeafConjuncts() {
			env, expr := conjunct.EnvExpr()
			visit(env, expr)
		}
	}
	return root
}

// Follow value references through their declared paths. Looking only at the
// final dereferenced schema would conflate two uses of the same abbreviation.
func (n *publicExport) reference(c *OpContext, env *Environment, expr Expr, seen map[Expr]bool) *publicExport {
	if seen[expr] {
		return nil
	}
	seen[expr] = true
	defer delete(seen, expr)
	switch x := expr.(type) {
	case *SelectorExpr:
		if x.Sel.IsDef() {
			return nil
		}
		return n.reference(c, env, x.X, seen).field(x.Sel)
	case *IndexExpr:
		base := n.reference(c, env, x.X, seen)
		if base == nil {
			return nil
		}
		if base.schema != nil && base.schema.HasSubjectSchemes() {
			return base // Type elimination preserves the exported subject.
		}
		if base.schema != nil {
			if f, ok := Unwrap(base.schema).(*FuncValue); ok && f.hasTypeSelection() {
				return base
			}
		}
		index, complete := c.Evaluate(env, x.Index)
		if !complete || !IsConcrete(index) {
			return nil
		}
		saved := c.PushState(env, expr.Source())
		label := LabelFromValue(c, x.Index, index)
		if c.PopState(saved) != nil {
			return nil
		}
		return base.field(label)
	case *FieldReference, *LetReference:
		if r, ok := x.(*LetReference); ok && r.IsPredicate {
			return nil
		}
		ref := x.(Resolver)
		saved := c.PushState(env, expr.Source())
		target := ref.resolve(c, Flags{status: partial, condition: arcTypeKnown, mode: yield})
		b := c.PopState(saved)
		if b != nil || target == nil || target.Label.IsDef() {
			return nil
		}
		// A reused record predicate can have several public occurrences.
		// Resolve aliases in the nearest enclosing occurrence of its scope.
		for scope := n.parent; scope != nil; scope = scope.parent {
			for _, child := range scope.schema.DerefValue().Arcs {
				if child == target {
					return scope.field(child.Label)
				}
			}
		}
		if target.Label.IsLet() {
			for conjunct := range target.LeafConjuncts() {
				e, value := conjunct.EnvExpr()
				if found := n.reference(c, e, value, seen); found != nil {
					return found
				}
			}
		}
	}
	return nil
}
