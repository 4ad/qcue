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
				return e.predicateValue(v)
			}
		}
		v, complete := e.ctx.Evaluate(t.Env, ref)
		if !complete || v == nil || (runtime && !e.exportableCapture(v, make(map[adt.Value]bool))) {
			id := ref.Source().(*ast.Ident)
			return e.quantifiedExportError("captured value %s cannot be exported independently", id.Name)
		}
		if runtime {
			return e.runtimeCaptureValue(v)
		}
		return e.predicateValue(v)
	}
	return e.originApplication(origin, value)
}

// Runtime environments contain all fields observable by the code, including
// hidden fields. JSON's regular-field projection is not closure conversion.
// Use evaluated values to avoid carrying their old lexical scopes along.
func (e *exporter) runtimeCaptureValue(v adt.Value) ast.Expr {
	saved := e.cfg
	profile := *Final
	profile.ShowHidden = true
	profile.ShowDefinitions = true
	e.cfg = &profile
	defer func() { e.cfg = saved }()
	return e.value(v)
}

// Predicates control future calls and refinements. Data output options such
// as Final and omission of optional fields must never weaken them. Keep this
// in the current exporter so callable dependencies share their code origins.
func (e *exporter) predicateValue(v adt.Value) ast.Expr {
	saved := e.cfg
	profile := *All
	profile.SelfContained = true
	e.cfg = &profile
	defer func() { e.cfg = saved }()
	if v, ok := v.(*adt.Vertex); ok {
		if v.Kind()&(adt.StructKind|adt.ListKind) == 0 && len(v.Arcs) == 0 {
			// Evaluated scalar predicates already carry their resolved
			// bounds. Reusing their source could reintroduce a free name.
			return e.value(v.Value())
		}
		closed := v.IsRecursivelyClosed()
		if closed {
			e.inDefinition++
			defer func() { e.inDefinition-- }()
		}
		x := e.expr(nil, v)
		if closed && v.Kind() == adt.StructKind {
			name := e.uniqueAlias("#CUEType")
			decl := &ast.Field{Label: ast.NewIdent(name), Value: x}
			e.originDecls = append(e.originDecls, decl)
			id := ast.NewIdent(name)
			id.Node = x
			x = id
		}
		return x
	}
	return e.value(v)
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
	if _, ok := e.quantifierCode[fn]; ok {
		e.quantifiedExportError("shared composite code cannot be exported in separate lexical origins")
	}
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
	visited := make(map[adt.Node]bool)
	var w walk.Visitor
	w.Before = func(n adt.Node) bool {
		if visited[n] {
			return false
		}
		visited[n] = true
		switch x := n.(type) {
		case *adt.AliasApplication:
			w.Elem(x.Template.Body)
			for _, arg := range x.Args {
				w.Elem(arg)
			}
			return false
		case *adt.Quantified:
			if f, ok := x.Body.(*adt.Function); ok && f.Body != nil {
				nested = append(nested, nestedFunction{x.Src, f, x.Params})
				return false
			}
			// Keep composite introductions intact inside this code origin.
			// Lifting their individual methods would give those methods a
			// different telescope from the surrounding record or list.
			for _, f := range quantifierFunctions(x) {
				if _, ok := e.quantifierCode[f]; ok || e.functionOrigins[f] != nil {
					e.quantifiedExportError("shared composite code cannot be exported in separate lexical origins")
				}
				if e.quantifierCode == nil {
					e.quantifierCode = make(map[*adt.Function]quantifierOriginKey)
				}
				e.quantifierCode[f] = quantifierOriginKey{q: x}
			}
			return false
		case *adt.Function:
			if x != fn && x.Body != nil {
				nested = append(nested, nestedFunction{src: x.Src, fn: x})
				return false
			}
		}
		return true
	}
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
	body = e.withLexicalAliases(body, replacements, names)
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

// Retain lexical abbreviations with their binders and bounds, rather than
// exporting an unbound alias name or inlining away its admissibility checks.
// Free references keep their original node identities until the enclosing
// origin substitutes its predicate and runtime dependencies below.
func (e *exporter) withLexicalAliases(body ast.Expr, replacements map[ast.Node]ast.Expr, names map[ast.Node]string) ast.Expr {
	var decls []ast.Decl
	seen := make(map[ast.Node]bool)
	local := make(map[ast.Node]bool)
	var visit func(ast.Node)
	visit = func(src ast.Node) {
		ast.Walk(src, func(n ast.Node) bool { local[n] = true; return true }, nil)
		ast.Walk(src, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || local[id.Node] || seen[id.Node] || names[id.Node] != "" {
				return true
			}
			if e.originNames[id.Name] == id.Node {
				return true // Already emitted once as a shared code origin.
			}
			var decl ast.Decl
			switch original := id.Node.(type) {
			case *ast.ParametricAlias:
				copy := ast.Clone(original)
				copy.Body = cloneFunctionSource(original.Body, replacements)
				decl = copy
			case *ast.LetClause:
				copy := ast.Clone(original)
				copy.Expr = cloneFunctionSource(original.Expr, replacements)
				decl = copy
			default:
				return true
			}
			seen[id.Node] = true
			name := e.uniqueAlias("CUEAlias")
			names[id.Node] = name
			switch d := decl.(type) {
			case *ast.ParametricAlias:
				d.Name = ast.NewIdent(name)
			case *ast.LetClause:
				d.Ident = ast.NewIdent(name)
			}
			visit(decl)
			decls = append(decls, decl)
			return true
		}, nil)
	}
	visit(body)
	if len(decls) == 0 {
		return body
	}
	return &ast.StructLit{Elts: append([]ast.Decl{&ast.EmbedDecl{Expr: body}}, decls...)}
}
