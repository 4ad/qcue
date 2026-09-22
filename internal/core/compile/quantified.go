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
	"slices"
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
)

func (c *compiler) aliasTemplate(src *ast.ParametricAlias, level int) adt.Expr {
	if c.parametricAliases == nil {
		c.parametricAliases = make(map[*ast.ParametricAlias]adt.Expr)
	}
	if x, ok := c.parametricAliases[src]; ok {
		if x == nil {
			return c.errf(src, "cyclic parametric alias %s", src.Name.Name)
		}
		return x
	}
	c.parametricAliases[src] = nil
	saved := c.stack
	c.stack = slices.Clone(c.stack[:level+1])
	q := &ast.Quantifier{Quantifier: src.Pos(), Params: src.Params, Body: src.Body}
	x := c.quantifiedTemplate(q, src)
	c.stack = saved
	c.parametricAliases[src] = x
	return x
}

func (c *compiler) aliasApplication(src *ast.CallExpr, id *ast.Ident, alias *ast.ParametricAlias) adt.Expr {
	if len(src.Args) != len(alias.Params) || src.Ellipsis != token.NoPos {
		return c.errf(src, "alias %s requires %d type arguments", alias.Name.Name, len(alias.Params))
	}
	for _, label := range src.ArgLabels {
		if label != nil {
			return c.errf(src, "alias arguments must be positional")
		}
	}
	up := int32(0)
	level := len(c.stack) - 1
	for ; level >= 0; level-- {
		if c.stack[level].scope == id.Scope {
			break
		}
		up += c.stack[level].upCount
	}
	if level < 0 {
		return c.errf(id, "parametric alias %s is out of scope", id.Name)
	}
	template := c.aliasTemplate(alias, level)
	q, ok := template.(*adt.Quantified)
	if !ok {
		return template
	}
	application := &adt.AliasApplication{Src: src, Template: q, UpCount: up}
	for _, a := range src.Args {
		application.Args = append(application.Args, c.expr(a))
	}
	return application
}

func (c *compiler) quantifier(src *ast.Quantifier) adt.Expr {
	return c.quantifiedTemplate(src, src)
}

func (c *compiler) quantifiedTemplate(src *ast.Quantifier, scope ast.Node) adt.Expr {
	if !c.experiments.Quantified {
		return c.errf(src, "quantifier syntax requires @experiment(quantified)")
	}
	q := &adt.Quantified{Src: src}
	c.pushScope(nil, 1, scope)
	defer c.popScope()
	if c.typeParameters == nil {
		c.typeParameters = make(map[*ast.TypeParam]*adt.TypeParameter)
	}
	for _, p := range src.Params {
		param := &adt.TypeParameter{Src: p}
		if p.Sort != nil {
			if finiteValueRange(p.Sort) {
				param.ValueRange = c.expr(p.Sort)
				c.typeParameters[p] = param
				q.Params = append(q.Params, param)
				continue
			}
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

func finiteValueRange(x ast.Expr) bool {
	switch x := x.(type) {
	case *ast.BasicLit, *ast.BottomLit:
		return true
	case *ast.ParenExpr:
		return finiteValueRange(x.X)
	case *ast.UnaryExpr:
		return (x.Op == token.ADD || x.Op == token.SUB) && finiteValueRange(x.X)
	case *ast.BinaryExpr:
		return x.Op == token.OR && finiteValueRange(x.X) && finiteValueRange(x.Y)
	}
	return false
}
