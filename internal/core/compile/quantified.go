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

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
)

// Alias bodies have the same two interpretations as ordinary abbreviations:
// a value expression and a predicate that retains live dependencies.
// Both interpretations share lexical binders and function code origins.
type aliasContext struct {
	src       *ast.ParametricAlias
	predicate bool
}

func (c *compiler) aliasTemplate(src *ast.ParametricAlias, level int) adt.Expr {
	if c.parametricAliases == nil {
		c.parametricAliases = make(map[aliasContext]adt.Expr)
	}
	key := aliasContext{src, c.typePosition}
	if x, ok := c.parametricAliases[key]; ok {
		if x == nil {
			return c.errf(src, "cyclic parametric alias %s", src.Name.Name)
		}
		return x
	}
	c.parametricAliases[key] = nil
	saved := c.stack
	c.stack = slices.Clone(c.stack[:level+1])
	q := &ast.Quantifier{Quantifier: src.Pos(), Params: src.Params, Body: src.Body}
	x := c.quantifiedTemplate(q, src)
	c.stack = saved
	c.parametricAliases[key] = x
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
		application.Args = append(application.Args, c.typeExpr(a))
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
	defer func() {
		c.popScope()
		if q.Body != nil {
			q.References = c.freeReferences(src, q, false)
		}
	}()
	if c.typeParameters == nil {
		c.typeParameters = make(map[*ast.TypeParam]*adt.TypeParameter)
	}
	for _, p := range src.Params {
		if param := c.typeParameters[p]; param != nil {
			q.Params = append(q.Params, param)
			continue
		}
		param := &adt.TypeParameter{Src: p}
		param.Bound = c.typeExpr(p.Bound)
		param.References = c.freeReferences(p, param.Bound, false)
		c.typeParameters[p] = param
		q.Params = append(q.Params, param)
	}
	q.Body = c.expr(src.Body)
	return q
}

func (c *compiler) typeExpr(x ast.Expr) adt.Expr {
	saved := c.typePosition
	c.typePosition = c.experiments.Quantified
	defer func() { c.typePosition = saved }()
	return c.expr(x)
}

func (c *compiler) valueExpr(x ast.Expr) adt.Expr {
	saved := c.typePosition
	c.typePosition = false
	defer func() { c.typePosition = saved }()
	return c.expr(x)
}

func liveDescriptionReference(x adt.Expr) bool {
	switch x := x.(type) {
	case *adt.FieldReference:
		if x.Label.IsDef() {
			return false
		}

		return true
	case *adt.SelectorExpr:
		return !x.Sel.IsDef() && liveDescriptionReference(x.X)
	case *adt.IndexExpr:
		return liveDescriptionReference(x.X)
	case *adt.LetReference:
		return !x.IsPredicate && liveDescriptionReference(x.X)
	}
	return false
}
