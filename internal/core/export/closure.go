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
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/walk"
)

// Each code literal is emitted once. Predicate dependencies are arguments of
// an abbreviation, while runtime captures are arguments of a closure factory.
// Reusing the factory preserves code identity without conflating environments.
type functionOrigin struct {
	decl       ast.Decl
	name       string
	predicates []adt.Expr
	captures   []adt.Expr
}

// Expression-only APIs must carry the declarations inside their returned
// expression; they do not return the file assembled by finalize.
func (e *exporter) withOriginDecls(x ast.Expr) ast.Expr {
	if len(e.originDecls) == 0 {
		return x
	}
	s, ok := x.(*ast.StructLit)
	if !ok {
		s = &ast.StructLit{Elts: []ast.Decl{&ast.EmbedDecl{Expr: x}}}
	}
	s.Elts = append(s.Elts, e.originDecls...)
	e.originDecls = nil
	return s
}

func referenceKey(x adt.Expr) ast.Node {
	if id, ok := x.Source().(*ast.Ident); ok {
		return id.Node
	}
	return nil
}

func (e *exporter) functionOriginValue(t adt.FuncType) ast.Expr {
	params := adt.FunctionTypeParameters(t)
	origin := e.functionOrigin(t.Fn, params)
	args := adt.FunctionTypeArguments(t)
	value := func(ref adt.Expr, runtime bool) ast.Expr {
		if r, ok := ref.(*adt.TypeReference); ok {
			if v := args[r.Param.Src]; v != nil {
				return e.value(v)
			}
		}
		v, complete := e.ctx.Evaluate(t.Env, ref)
		if !complete || v == nil || (runtime && !e.exportableCapture(v, make(map[adt.Value]bool))) {
			id := ref.Source().(*ast.Ident)
			return e.quantifiedExportError("captured value %s cannot be exported independently", id.Name)
		}
		if runtime {
			if vtx, ok := v.(*adt.Vertex); ok {
				v = vtx.ToDataAll(e.ctx)
			}
		}
		return e.value(v)
	}
	return e.originApplication(origin, value)
}

func (e *exporter) originApplication(o *functionOrigin, value func(adt.Expr, bool) ast.Expr) ast.Expr {
	id := ast.NewIdent(o.name)
	id.Node = o.decl
	var x ast.Expr = id
	if len(o.predicates) > 0 {
		call := ast.NewCall(x)
		for _, ref := range o.predicates {
			call.Args = append(call.Args, value(ref, false))
		}
		x = call
	}
	if len(o.captures) > 0 {
		call := ast.NewCall(x)
		for _, ref := range o.captures {
			call.Args = append(call.Args, value(ref, true))
		}
		x = call
	}
	return x
}

func (e *exporter) functionOrigin(fn *adt.Function, params []*adt.TypeParameter) *functionOrigin {
	if o := e.functionOrigins[fn]; o != nil {
		return o
	}
	if e.functionOrigins == nil {
		e.functionOrigins = make(map[*adt.Function]*functionOrigin)
		e.originNames = make(map[string]ast.Node)
	}
	// Reserve local names as well as field labels before making helper names.
	reserve := func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			label := adt.MakeIdentLabel(e.ctx, id.Name, "")
			if _, ok := e.usedFeature[label]; !ok {
				e.usedFeature[label] = nil
			}
		}
		return true
	}
	ast.Walk(fn.Src, reserve, nil)
	for _, p := range params {
		ast.Walk(p.Src, reserve, nil)
	}
	o := &functionOrigin{name: e.uniqueAlias("CUECode"), captures: fn.Captures}
	e.functionOrigins[fn] = o
	own := make(map[ast.Node]bool)
	for _, p := range params {
		own[p.Src] = true
	}
	captured := make(map[ast.Node]bool)
	for _, ref := range o.captures {
		captured[referenceKey(ref)] = true
	}
	refs := append([]adt.Expr(nil), fn.References...)
	for _, p := range params {
		refs = append(refs, p.References...)
	}
	seen := make(map[ast.Node]bool)
	for _, ref := range refs {
		key := referenceKey(ref)
		if !own[key] && !captured[key] && !seen[key] {
			seen[key] = true
			o.predicates = append(o.predicates, ref)
		}
	}
	names := make(map[ast.Node]string)
	var aliasParams []*ast.TypeParam
	for _, ref := range o.predicates {
		name := e.uniqueAlias("CUEType")
		names[referenceKey(ref)] = name
		aliasParams = append(aliasParams, &ast.TypeParam{Name: ast.NewIdent(name)})
	}
	var captureParams []*ast.FuncParam
	for _, ref := range o.captures {
		name := e.uniqueAlias("CUECapture")
		names[referenceKey(ref)] = name
		captureParams = append(captureParams, &ast.FuncParam{Label: ast.NewIdent(name), Value: ast.NewIdent("_")})
	}
	// Allocate the declaration before descending so every reference links to
	// the same node, including references from nested code origins.
	if len(aliasParams) > 0 {
		o.decl = &ast.ParametricAlias{Name: ast.NewIdent(o.name), Params: aliasParams}
	} else {
		o.decl = &ast.LetClause{Ident: ast.NewIdent(o.name)}
	}
	e.originNames[o.name] = o.decl

	type nestedFunction struct {
		src    ast.Node
		fn     *adt.Function
		params []*adt.TypeParameter
	}
	var nested []nestedFunction
	w := walk.Visitor{Before: func(n adt.Node) bool {
		switch x := n.(type) {
		case *adt.AliasApplication:
			return false
		case *adt.Quantified:
			if f, ok := x.Body.(*adt.Function); ok && f.Body != nil {
				nested = append(nested, nestedFunction{x.Src, f, x.Params})
				return false
			}
		case *adt.Function:
			if x != fn && x.Body != nil {
				nested = append(nested, nestedFunction{src: x.Src, fn: x})
				return false
			}
		}
		return true
	}}
	w.Elem(fn)
	// Clone first, but remember which copies correspond to nested literals.
	replacements := make(map[ast.Node]ast.Expr)
	for _, n := range nested {
		no := e.functionOrigin(n.fn, n.params)
		replacements[n.src] = e.originApplication(no, func(ref adt.Expr, _ bool) ast.Expr {
			return ast.Clone(ref.Source().(*ast.Ident))
		})
	}
	body := cloneFunctionSource(fn.Src, replacements)
	if len(params) > 0 {
		q := &ast.Quantifier{Body: body}
		for _, p := range params {
			q.Params = append(q.Params, ast.Clone(p.Src))
		}
		body = q
	}
	body = astutil.Apply(body, func(c astutil.Cursor) bool {
		if id, ok := c.Node().(*ast.Ident); ok {
			if name := names[id.Node]; name != "" {
				c.Replace(ast.NewIdent(name))
				return false
			}
		}
		return true
	}, nil).(ast.Expr)
	if len(captureParams) > 0 {
		body = &ast.Func{Params: captureParams, Ret: ast.NewIdent("_"), Body: body}
	}
	body = e.funcExprSrc(body, "quantified")
	switch d := o.decl.(type) {
	case *ast.ParametricAlias:
		d.Body = body
	case *ast.LetClause:
		d.Expr = body
	}
	e.originDecls = append(e.originDecls, o.decl)
	return o
}

func cloneFunctionSource(src ast.Expr, replacements map[ast.Node]ast.Expr) ast.Expr {
	// Pair the two traversals rather than relying on source positions, which
	// need not be unique in programmatically constructed syntax trees.
	var originals []ast.Node
	ast.Walk(src, func(n ast.Node) bool {
		originals = append(originals, n)
		return true
	}, nil)
	copied := ast.Clone(src)
	byCopy := make(map[ast.Node]ast.Expr)
	i := 0
	ast.Walk(copied, func(n ast.Node) bool {
		if replacement := replacements[originals[i]]; replacement != nil {
			byCopy[n] = replacement
		}
		i++
		return true
	}, nil)
	return astutil.Apply(copied, func(c astutil.Cursor) bool {
		if replacement := byCopy[c.Node()]; replacement != nil {
			c.Replace(ast.Clone(replacement))
			return false
		}
		return true
	}, nil).(ast.Expr)
}
