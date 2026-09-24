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
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/walk"
)

// Each code literal is emitted once, inside an environment record template.
// Predicate dependencies are arguments of an abbreviation; runtime captures
// are fields supplied by unification. Unlike a factory call, instantiating a
// record does not demand the captures before constructing the closure. This
// lets recursive environments refer back to their closures.
type functionOrigin struct {
	decl         *ast.ParametricAlias
	name         string
	predicates   []adt.Expr
	captures     []adt.Expr
	captureNames []string
	result       string
	environment  string
}

// closureGraph names runtime environments independently of code origins.
// Its fields may refer to each other, including through captured records.
// References within the graph use field names: referring to the enclosing
// binding would introduce a recursive reference to the whole graph instead.
type closureGraph struct {
	decl      *ast.Field
	fields    *ast.StructLit
	depth     int
	functions map[adt.FuncType]*ast.Field
	values    map[*adt.Vertex]*ast.Field
}

func (e *exporter) graph() *closureGraph {
	if e.closures == nil {
		fields := &ast.StructLit{}
		// A let is an abbreviation and may reconstruct the record at each
		// use. A definition provides stable bindings, so references to the
		// same recursive closure also retain the same environment identity.
		decl := &ast.Field{Label: ast.NewIdent(e.uniqueAlias("#CUEClosures")), Value: fields}
		e.closures = &closureGraph{decl: decl, fields: fields,
			functions: make(map[adt.FuncType]*ast.Field), values: make(map[*adt.Vertex]*ast.Field)}
		e.originDecls = append(e.originDecls, decl)
	}
	return e.closures
}

func (e *exporter) closureField() *ast.Field {
	g := e.graph()
	f := &ast.Field{Label: ast.NewIdent(e.uniqueAlias("CUEClosure")), Value: &ast.ParenExpr{}}
	g.fields.Elts = append(g.fields.Elts, f)
	return f
}

func (e *exporter) closureReference(f *ast.Field) ast.Expr {
	g := e.graph()
	name := f.Label.(*ast.Ident).Name
	if g.depth > 0 {
		id := ast.NewIdent(name)
		id.Node = f.Value
		return id
	}
	id := ast.NewIdent(g.decl.Label.(*ast.Ident).Name)
	id.Node = g.decl.Value
	return ast.NewSel(id, name)
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
	g := e.graph()
	if f := g.functions[t]; f != nil {
		return e.closureReference(f)
	}
	f := e.closureField()
	g.functions[t] = f
	args := adt.FunctionTypeArguments(t)
	value := func(ref adt.Expr, runtime bool) ast.Expr {
		if r, ok := ref.(*adt.TypeReference); ok {
			if v := args[r.Param.Src]; v != nil {
				return e.predicateValue(v)
			}
		}
		v, complete := e.ctx.Evaluate(t.Env, ref)
		if !complete || v == nil || (runtime && !e.exportableCapture(v)) {
			id := ref.Source().(*ast.Ident)
			return e.quantifiedExportError("captured value %s cannot be exported independently", id.Name)
		}
		if runtime {
			return e.runtimeCaptureValue(v)
		}
		return e.predicateValue(v)
	}
	g.depth++
	f.Value.(*ast.ParenExpr).X = e.originApplication(origin, value)
	g.depth--
	return e.closureReference(f)
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
	if v, ok := v.(*adt.Vertex); ok {
		if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
			return e.value(f)
		}
		if v.Kind()&(adt.StructKind|adt.ListKind) != 0 {
			g := e.graph()
			if f := g.values[v]; f != nil {
				return e.closureReference(f)
			}
			f := e.closureField()
			g.values[v] = f
			g.depth++
			// This value is emitted in the graph, outside the current output
			// scope. In particular it may be an ancestor of that scope; the
			// ordinary vertex stack must not replace it with top as a cycle.
			saved := e.stack
			e.stack = nil
			f.Value.(*ast.ParenExpr).X = e.value(v)
			e.stack = saved
			g.depth--
			return e.closureReference(f)
		}
	}
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
		if v.Kind()&(adt.StructKind|adt.ListKind) == 0 && len(v.Arcs) == 0 && !v.HasSubjectSchemes() {
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
	call := ast.NewCall(id)
	if len(o.predicates) == 0 {
		call.Args = append(call.Args, ast.NewIdent("_"))
	} else {
		for _, ref := range o.predicates {
			call.Args = append(call.Args, value(ref, false))
		}
	}
	var x ast.Expr = call
	if len(o.captures) > 0 {
		captures := &ast.StructLit{}
		for i, ref := range o.captures {
			captures.Elts = append(captures.Elts, &ast.Field{
				Label: ast.NewIdent(o.captureNames[i]), Value: value(ref, true),
			})
		}
		// Give the environment a field before projecting its function. An
		// inline conjunction followed immediately by a selector has no stable
		// vertex for recursive captures, notably inside a factory's body.
		x = &ast.StructLit{Elts: []ast.Decl{
			&ast.Field{Label: ast.NewIdent(o.environment), Value: ast.NewBinExpr(token.AND, x, captures)},
			&ast.Field{Label: ast.NewIdent(o.result), Value: ast.NewSel(ast.NewIdent(o.environment), o.result)},
		}}
	}
	return ast.NewSel(x, o.result)
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
	o := &functionOrigin{
		name: e.uniqueAlias("CUECode"), captures: fn.Captures,
		result: e.uniqueAlias("CUEFunction"), environment: e.uniqueAlias("CUEEnvironment"),
	}
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
	if len(aliasParams) == 0 {
		// This is a lexical template, not an incomplete captured runtime
		// record. CUE requires at least one alias parameter; an unused
		// predicate keeps that distinction on subsequent source exports.
		aliasParams = append(aliasParams, &ast.TypeParam{Name: ast.NewIdent(e.uniqueAlias("CUEType"))})
	}
	environment := &ast.StructLit{}
	for _, ref := range o.captures {
		name := e.uniqueAlias("CUECapture")
		names[referenceKey(ref)] = name
		o.captureNames = append(o.captureNames, name)
		environment.Elts = append(environment.Elts, &ast.Field{Label: ast.NewIdent(name), Value: ast.NewIdent("_")})
	}
	// Allocate the declaration before descending so every reference links to
	// the same node, including references from nested code origins.
	o.decl = &ast.ParametricAlias{Name: ast.NewIdent(o.name), Params: aliasParams}
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
	environment.Elts = append(environment.Elts, &ast.Field{Label: ast.NewIdent(o.result), Value: body})
	body = e.funcExprSrc(environment, "quantified")
	o.decl.Body = body
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
		// A reference to the enclosing field may resolve to the literal
		// itself. Clone treats that node as local, but closure conversion
		// must still substitute the field's runtime capture below.
		if id, ok := originals[i].(*ast.Ident); ok && id.Node == src {
			n.(*ast.Ident).Node = id.Node
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
