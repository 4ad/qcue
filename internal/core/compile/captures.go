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

package compile

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/walk"
)

// functionCaptures performs closure conversion's free-variable analysis. It
// inspects compiled references, so declaration labels and parameter names
// cannot accidentally become captures. Recompiling only the free references
// in the surrounding scope removes the body's parameter and constructor
// scopes from their environment offsets.
func (c *compiler) functionCaptures(src *ast.Func, fn *adt.Function) []adt.Expr {
	return c.freeReferences(src, fn, true)
}

// freeReferences compiles free references in the surrounding scope. Runtime
// captures exclude erased predicates; residual syntax needs both kinds.
func (c *compiler) freeReferences(src ast.Node, expr adt.Expr, runtimeOnly bool) []adt.Expr {
	local := make(map[ast.Node]bool)
	ast.Walk(src, func(n ast.Node) bool {
		local[n] = true
		return true
	}, nil)
	seen := make(map[ast.Node]bool)
	var captures []adt.Expr
	var w walk.Visitor
	typePosition := false
	w.Before = func(n adt.Node) bool {
		if n == nil {
			return false
		}
		switch x := n.(type) {
		case *adt.Function:
			saved := typePosition
			for _, p := range x.Params {
				typePosition = true
				w.Elem(p.Value)
				typePosition = false
				w.Elem(p.Default)
			}
			typePosition = true
			w.Elem(x.Ret)
			typePosition = false
			w.Elem(x.Body)
			typePosition = saved
			return false
		case *adt.WitnessReference:
			// A value used as a singleton remains a runtime dependency.
			// Other references in annotations describe erased predicates.
			saved := typePosition
			typePosition = false
			w.Elem(x.X)
			typePosition = saved
			return false
		}
		if _, ok := n.(adt.Resolver); !ok {
			if r, ok := n.(*adt.TypeReference); !ok || r.Param.ValueRange == nil {
				return true
			}
		} else if runtimeOnly && typePosition {
			return false
		}
		id, ok := n.Source().(*ast.Ident)
		if !ok || local[id.Scope] || local[id.Node] {
			return true
		}
		if p, ok := id.Node.(*ast.TypeParam); ok && runtimeOnly {
			if c.typeParameters[p].ValueRange == nil {
				return false
			}
		}
		if _, imported := id.Node.(*ast.ImportSpec); imported {
			return false
		}
		if scope, ok := id.Scope.(*ast.OpenExpr); ok && id.Node == scope.Type {
			return false
		}
		key := id.Node
		if key == nil {
			key = id
		}
		if !seen[key] {
			seen[key] = true
			captures = append(captures, c.resolve(id))
		}
		return false
	}
	w.Elem(expr)
	return captures
}
