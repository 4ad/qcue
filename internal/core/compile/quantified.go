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
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
)

func (c *compiler) quantifier(src *ast.Quantifier) adt.Expr {
	if !c.experiments.Quantified {
		return c.errf(src, "quantifier syntax requires @experiment(quantified)")
	}
	q := &adt.Quantified{Src: src}
	c.pushScope(nil, 1, src)
	defer c.popScope()
	if c.typeParameters == nil {
		c.typeParameters = make(map[*ast.TypeParam]*adt.TypeParameter)
	}
	for _, p := range src.Params {
		param := &adt.TypeParameter{Src: p}
		if p.Sort != nil {
			call, ok := p.Sort.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return c.errf(p, "value binders require the dependent profile D")
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "Type" {
				return c.errf(p, "value binders require the dependent profile D")
			}
			level, ok := call.Args[0].(*ast.BasicLit)
			if !ok || level.Kind != token.INT {
				return c.errf(p, "universe level must be a nonnegative integer literal")
			}
			n, err := strconv.Atoi(level.Value)
			if err != nil || n < 0 {
				return c.errf(p, "invalid universe level")
			}
			param.Level = n
		}
		param.Bound = c.expr(p.Bound)
		c.typeParameters[p] = param
		q.Params = append(q.Params, param)
	}
	q.Body = c.expr(src.Body)
	return q
}
