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
	local := make(map[ast.Node]bool)
	ast.Walk(src, func(n ast.Node) bool {
		local[n] = true
		return true
	}, nil)
	seen := make(map[ast.Node]bool)
	var captures []adt.Expr
	w := walk.Visitor{Before: func(n adt.Node) bool {
		if n == nil {
			return false
		}
		if _, ok := n.(adt.Resolver); !ok {
			if r, ok := n.(*adt.TypeReference); !ok || r.Param.ValueRange == nil {
				return true
			}
		}
		id, ok := n.Source().(*ast.Ident)
		if !ok || local[id.Scope] {
			return true
		}
		if p, ok := id.Node.(*ast.TypeParam); ok {
			if c.typeParameters[p].ValueRange == nil {
				return false
			}
		}
		if _, imported := id.Node.(*ast.ImportSpec); imported {
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
	}}
	w.Elem(fn)
	return captures
}
