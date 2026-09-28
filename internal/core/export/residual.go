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

package export

import (
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/walk"
)

// A computed call may have unfinished or later-refinable packet demands even
// when its current result is an atom. Value-oriented CUE output must preserve
// the source relation of that call on the same exposed coordinates. JSON
// materialization has its own validation path and does not use this exporter.
func (e *exporter) hasInvocation(n *adt.Vertex, seen map[*adt.Vertex]bool) (result bool) {
	if n == nil || seen[n] || n.IsData() {
		return false
	}
	if found, ok := e.invocations[n]; ok {
		return found
	}
	if e.invocations == nil {
		e.invocations = make(map[*adt.Vertex]bool)
	}
	defer func() { e.invocations[n] = result }()
	seen[n] = true
	for _, field := range n.Arcs {
		if e.hasInvocation(field, seen) {
			return true
		}
	}
	for c := range n.LeafConjuncts() {
		if e.expressionInvocation(c.Env, c.Expr(), n, seen) {
			return true
		}
	}
	return false
}

func (e *exporter) expressionInvocation(env *adt.Environment, expr adt.Expr, owner *adt.Vertex, seen map[*adt.Vertex]bool) bool {
	found := false
	visitor := walk.Visitor{Before: func(node adt.Node) bool {
		if found || node == nil {
			return false
		}
		switch x := node.(type) {
		case *adt.Function, *adt.Quantified:
			// A declaration alone is not an invocation of its body.
			return false
		case *adt.StructLit:
			if node == expr && owner != nil {
				// Fields were visited in their evaluated lexical scopes.
				// Embeddings can contribute a scalar without leaving an arc.
				scope := &adt.Environment{Up: env, Vertex: owner}
				for _, decl := range x.Decls {
					if embedded, ok := decl.(adt.Expr); ok && e.expressionInvocation(scope, embedded, nil, seen) {
						found = true
						break
					}
				}
				return false
			}
			value, _ := e.ctx.Evaluate(env, x)
			if vertex, ok := value.(*adt.Vertex); ok {
				found = e.hasInvocation(vertex, seen)
			}
			return false
		case *adt.ListLit:
			if node == expr && owner != nil {
				return false
			}
			value, _ := e.ctx.Evaluate(env, x)
			if vertex, ok := value.(*adt.Vertex); ok {
				found = e.hasInvocation(vertex, seen)
			}
			return false
		case *adt.CallExpr:
			value, _ := e.ctx.Evaluate(env, x.Fun)
			switch f := adt.Unwrap(value).(type) {
			case *adt.FuncValue:
				found = f.Fn != nil && f.Fn.Quantified
			case *adt.Builtin:
				for _, clause := range f.AdditionalTypes() {
					found = found || clause.Fn.Quantified
				}
			}
		}
		return !found
	}}
	visitor.Elem(expr)
	return found
}
