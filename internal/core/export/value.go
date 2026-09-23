// Copyright 2020 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package export

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
)

func (e *exporter) bareValue(v adt.Value) ast.Expr {
	switch x := v.(type) {
	case *adt.Vertex:
		return e.vertex(x)
	case adt.Value:
		a := &adt.Vertex{BaseValue: x}
		return e.vertex(a)
	default:
		panic("unreachable")
	}
	// TODO: allow a Value context wrapper.
}

// TODO: if the original value was a single reference, we could replace the
// value with a reference in graph mode.

func (e *exporter) vertex(n *adt.Vertex) (result ast.Expr) {
	// Guard against infinite recursion when a vertex cycles back to itself
	// through BuiltinValidator arguments or other value-level cycles.
	for i := range e.stack {
		if e.stack[i].node == n {
			return ast.NewIdent("_")
		}
	}

	var attrs []*ast.Attribute
	if e.cfg.ShowAttributes {
		attrs = ExtractDeclAttrs(n)
	}

	s, saved := e.pushFrame(n, n.Conjuncts)
	e.top().upCount++
	defer func() {
		e.top().upCount--
		e.popFrame(saved)
	}()

	for c := range n.LeafConjuncts() {
		e.markLets(c.Expr().Source(), s)
	}

	switch x := n.BaseValue.(type) {
	case nil:
		// bare
	case *adt.StructMarker:
		result = e.structComposite(n, attrs)

	case *adt.ListMarker:
		if e.showArcs(n) || attrs != nil {
			result = e.structComposite(n, attrs)
		} else {
			result = e.listComposite(n)
		}

	case *adt.Bottom:
		switch {
		case n.ArcType == adt.ArcOptional:
			// Optional fields may always be the original value.

		case e.cfg.ShowErrors && x.ChildError:
			// TODO(perf): use precompiled arc statistics
			if len(n.Arcs) > 0 && n.Arcs[0].Label.IsInt() && !e.showArcs(n) && attrs == nil {
				result = e.listComposite(n)
			} else {
				result = e.structComposite(n, attrs)
			}

		case !x.IsIncomplete() || !n.HasConjuncts() || e.cfg.Final:
			result = e.bottom(x)
		}

	case adt.Value:
		if e.showArcs(n) || attrs != nil {
			result = e.structComposite(n, attrs)
		} else {
			result = e.value(x, n.Conjuncts...)
		}

	default:
		panic("unknown value")
	}
	if result == nil {
		// fall back to expression mode
		// Use stable sort to ensure that tie breaks (for instance if elements
		// are not associated with a position) are deterministic.
		a := slices.SortedStableFunc(n.LeafConjuncts(), cmpConjuncts)

		// Dedup conjuncts that share the same body AST. Pushdown lands a
		// `for x in xs { … }` over N items as N body conjuncts on the
		// target, one per yielded env. The envs differ but symbolic
		// rendering ignores them, so without dedup the fallback produces
		// `X & X & …`.
		seen := map[adt.Elem]bool{}
		exprs := make([]ast.Expr, 0, len(a))
		for _, c := range a {
			elem := c.Elem()
			if seen[elem] {
				continue
			}
			seen[elem] = true
			if x := e.expr(c.Env, elem); x != dummyTop {
				exprs = append(exprs, x)
			}
		}

		result = ast.NewBinExpr(token.AND, exprs...)
	}

	filterUnusedLets(s)
	if result != s && len(s.Elts) > 0 {
		// There are used let expressions within a non-struct.
		// For now we just fall back to the original expressions.
		result = e.adt(nil, n)
	}

	return result
}

func (e *exporter) value(n adt.Value, a ...adt.Conjunct) (result ast.Expr) {
	if e.cfg.TakeDefaults {
		n = adt.Default(n)
	}
	// Evaluate arc if needed?

	// if e.concrete && !adt.IsConcrete(n.Value) {
	// 	return e.errf("non-concrete value: %v", e.bareValue(n.Value))
	// }

	switch x := n.(type) {
	case *adt.Bottom:
		result = e.bottom(x)

	case *adt.Null:
		result = e.null(x)

	case *adt.Bool:
		result = e.bool(x)

	case *adt.Num:
		result = e.num(x, a)

	case *adt.String:
		result = e.string(x, a)

	case *adt.Bytes:
		result = e.bytes(x, a)

	case *adt.BasicType:
		result = e.basicType(x)

	case *adt.Top:
		result = ast.NewIdent("_")

	case *adt.BoundValue:
		result = e.boundValue(x)

	case *adt.Builtin:
		result = e.builtin(x)

	case *adt.ExternalFunc:
		// TODO: we might be able to represent this as a reference or some
		// other expression in the future.
		result = e.bottom(&adt.Bottom{
			Err: errors.Newf(token.NoPos, "cannot convert function %q to CUE", x.Name),
		})

	case *adt.ExternalValidator:
		result = e.bottom(&adt.Bottom{
			Err: errors.Newf(token.NoPos, "cannot convert validator %q to CUE", x.Name),
		})

	case *adt.BuiltinValidator:
		result = e.builtinValidator(x)

	case *adt.FuncValue:
		if x.Fn.Quantified && x.IsPartial() {
			result = e.quantifiedExportError("partial closure cannot be exported without its bound argument environment")
		} else if x.Fn.Quantified {
			result = e.quantifiedFuncValue(x)
		} else {
			result = e.withFuncTypes(e.funcTypeSrc(adt.FuncType{Fn: x.Fn, Env: x.Env}), x.Types)
		}

	case *adt.Existential:
		result = e.quantifierSrc(x.Template, x.Env)

	case *adt.Universal:
		result = e.quantifierSrc(x.Template, x.Env)

	case *adt.AbstractResult:
		result = e.quantifiedExportError("cannot export an unresolved function execution")

	case *adt.OpaqueType, *adt.OpaqueValue:
		result = e.quantifiedExportError("opaque values require an interface codec for export")
	case *adt.RigidType:
		result = e.quantifiedExportError("proof variable cannot be exported")
	case *adt.WitnessType:
		result = e.innerExpr(x.Env, x.Ref.X)

	case *adt.Vertex:
		result = e.vertex(x)

	case *adt.Conjunction:
		switch len(x.Values) {
		case 0:
			return ast.NewIdent("_")
		case 1:
			if e.cfg.Simplify {
				return e.expr(nil, x.Values[0])
			}
			return e.bareValue(x.Values[0])
		}

		if e.cfg.Simplify {
			if name := adt.MatchBuiltinRange(x); name != "" {
				return ast.NewIdent(name)
			}
		}

		a := []adt.Value{}
		b := boundSimplifier{e: e}
		for _, v := range x.Values {
			if !e.cfg.Simplify || !b.add(v) {
				a = append(a, v)
			}
		}

		result = b.expr(e.ctx)
		if result == nil {
			a = x.Values
		}

		slices.SortStableFunc(a, cmpLeafNodes)

		for _, x := range a {
			result = wrapBin(result, e.bareValue(x), adt.AndOp)
		}

	case *adt.Disjunction:
		a := []ast.Expr{}

		for i, v := range x.Values {
			var expr ast.Expr
			if e.cfg.Simplify {
				expr = e.bareValue(v)
			} else {
				expr = e.expr(nil, v)
			}
			if i < x.NumDefaults {
				expr = &ast.UnaryExpr{Op: token.MUL, X: expr}
			}
			a = append(a, expr)
		}
		result = ast.NewBinExpr(token.OR, a...)

	case *adt.NodeLink:
		return e.value(x.Node, a...)

	default:
		panic(fmt.Sprintf("unsupported type %T", x))
	}

	// TODO: Add comments from original.

	return result
}

func (e *exporter) bottom(n *adt.Bottom) *ast.BottomLit {
	err := &ast.BottomLit{}
	if x := n.Err; x != nil {
		msg := x.Error()
		comment := &ast.Comment{Text: "// " + msg}
		ast.AddComment(err, &ast.CommentGroup{
			Line:     true,
			Position: 2,
			List:     []*ast.Comment{comment},
		})
	}
	return err
}

func (e *exporter) null(n *adt.Null) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.NULL, Value: "null"}
}

func (e *exporter) bool(n *adt.Bool) (b *ast.BasicLit) {
	return ast.NewBool(n.B)
}

func extractBasic(a []adt.Conjunct) *ast.BasicLit {
	for c := range adt.ConjunctsSeq(a) {
		if b, ok := c.Source().(*ast.BasicLit); ok {
			return &ast.BasicLit{Kind: b.Kind, Value: b.Value}
		}
	}
	return nil
}

func (e *exporter) num(n *adt.Num, orig []adt.Conjunct) *ast.BasicLit {
	// TODO: take original formatting into account.
	if b := extractBasic(orig); b != nil {
		return b
	}
	kind := token.FLOAT
	if n.K&adt.IntKind != 0 {
		kind = token.INT
	}
	s := n.X.String()
	// A float must carry a decimal point or an exponent, so that it is not
	// mistaken for an integer. Append a zero along with the point, as a bare
	// "2." reads oddly in the formats we export to, even where it is valid.
	if kind == token.FLOAT && !strings.ContainsAny(s, "eE.") {
		s += ".0"
	}
	return &ast.BasicLit{Kind: kind, Value: s}
}

func (e *exporter) string(n *adt.String, orig []adt.Conjunct) *ast.BasicLit {
	// TODO: take original formatting into account.
	if b := extractBasic(orig); b != nil {
		return b
	}
	s := literal.String.WithOptionalTabIndent(len(e.stack)).Quote(n.Str)
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: s,
	}
}

func (e *exporter) bytes(n *adt.Bytes, orig []adt.Conjunct) *ast.BasicLit {
	// TODO: take original formatting into account.
	if b := extractBasic(orig); b != nil {
		return b
	}
	s := literal.Bytes.WithOptionalTabIndent(len(e.stack)).Quote(string(n.B))
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: s,
	}
}

func (e *exporter) basicType(n *adt.BasicType) ast.Expr {
	// TODO: allow multi-bit types?
	return ast.NewIdent(n.K.String())
}

func (e *exporter) boundValue(n *adt.BoundValue) ast.Expr {
	return &ast.UnaryExpr{Op: n.Op.Token(), X: e.value(n.Value)}
}

func (e *exporter) builtin(x *adt.Builtin) ast.Expr {
	var result ast.Expr
	if x.Package == 0 {
		result = ast.NewPredeclared(x.Name)
	} else {
		spec := ast.NewImport(nil, x.Package.StringValue(e.index))
		info, _ := astutil.ParseImportSpec(spec)
		ident := ast.NewIdent(info.Ident)
		ident.Node = spec
		result = ast.NewSel(ident, x.Name)
	}
	// The import restores the builtin's own declarations. Every additional
	// contract must survive export, including unproved capability clauses.
	return e.withFuncTypes(result, x.AdditionalTypes())
}

// A selected view and its retained universal clause have one code origin.
// Emit that implementation once, followed by the original type selections.
// Printing a separate body for each clause would create distinct closures on
// reimport; printing only the selected body would lose universal obligations.
func (e *exporter) quantifiedFuncValue(f *adt.FuncValue) ast.Expr {
	head := adt.FuncType{Fn: f.Fn, Env: f.Env}
	origin := head
	var types []adt.FuncType
	for _, t := range f.Types {
		if t.Fn != f.Fn {
			types = append(types, t)
			continue
		}
		if len(adt.FunctionTypeParameters(t)) > len(adt.FunctionTypeParameters(origin)) {
			origin = t
		}
	}
	x := e.funcTypeSrc(origin)
	args := adt.FunctionTypeArguments(head)
	for _, p := range adt.FunctionTypeParameters(origin) {
		v := args[p.Src]
		if v == nil {
			break
		}
		x = &ast.IndexExpr{X: &ast.ParenExpr{X: x}, Index: e.value(v)}
	}
	return e.withFuncTypes(x, types)
}

// withFuncTypes renders the function types a function value, function type,
// or builtin has been unified with: the conjunction of the value itself and
// every recorded signature, so that the exported expression carries the same
// constraints as the value it represents. Each function literal is
// parenthesized: an unparenthesized signature would otherwise absorb the &
// operand into its result or body expression when parsed back.
func (e *exporter) withFuncTypes(x ast.Expr, types []adt.FuncType) ast.Expr {
	if len(types) == 0 {
		return x
	}
	if _, ok := x.(*ast.Func); ok {
		x = &ast.ParenExpr{X: x}
	}
	for _, t := range types {
		y := e.funcTypeSrc(t)
		if _, ok := y.(*ast.Func); ok {
			y = &ast.ParenExpr{X: y}
		}
		x = &ast.BinaryExpr{Op: token.AND, X: x, Y: y}
	}
	return x
}

// funcSrc returns the syntax with which to render a function literal in
// exported output. The compiled literal's source carries resolution links
// into the input syntax tree: the references its expressions capture point
// at nodes that do not occur in the output file, so sanitization would
// treat them as shadowed and rewrite them through top-level lets — lets
// that dangle whenever the captured field is not itself a top-level field.
// The literal is therefore re-parsed from its formatted source, anchoring
// its references lexically at the position where the literal is emitted,
// which is where equivalent struct-embedded references resolve. Only
// import references keep their binding, matched by name after local
// resolution, so that sanitization re-adds the corresponding import specs.
//
// Note that a function literal emitted in a structural position different
// from its source — for example when a subtree is exported self-contained —
// may still carry references that do not resolve in the output; hoisting
// the dependencies of function bodies is not yet supported.
func (e *exporter) funcSrc(src *ast.Func) ast.Expr {
	if src == nil {
		return ast.NewIdent("_")
	}
	return e.funcExprSrc(src, "functions")
}

func (e *exporter) quantifierSrc(q *adt.Quantified, env *adt.Environment) ast.Expr {
	if fn, ok := q.Body.(*adt.Function); ok && fn.Body != nil && env != nil {
		if v, ok := e.ctx.Evaluate(env, q); ok {
			if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
				return e.quantifiedFuncValue(f)
			}
		}
	}
	args := adt.FunctionTypeArguments(adt.FuncType{Env: env})
	refs := make(map[ast.Node]adt.Value)
	for _, ref := range q.References {
		id, ok := ref.Source().(*ast.Ident)
		if !ok || id.Node == nil {
			continue
		}
		value, complete := e.ctx.Evaluate(env, ref)
		if !complete || value == nil {
			return e.quantifiedExportError("quantifier dependency %s is unresolved", id.Name)
		}
		refs[id.Node] = value
	}
	src := astutil.Apply(ast.Clone(q.Src), func(c astutil.Cursor) bool {
		id, ok := c.Node().(*ast.Ident)
		if !ok {
			return true
		}
		value := refs[id.Node]
		if p, ok := id.Node.(*ast.TypeParam); ok && args[p] != nil {
			value = args[p]
		}
		if value != nil {
			// Parentheses keep an inserted arrow or quantifier from taking
			// ownership of operators in the surrounding template.
			c.Replace(&ast.ParenExpr{X: e.value(value)})
			return false
		}
		return true
	}, nil).(ast.Expr)
	return e.funcExprSrc(src, "quantified")
}

func (e *exporter) funcTypeSrc(t adt.FuncType) ast.Expr {
	if t.Fn == nil {
		return e.funcSrc(nil)
	}
	if !t.Fn.Quantified {
		return e.funcSrc(t.Fn.Src)
	}
	if t.Fn.Src == nil {
		return e.quantifiedExportError("function source is unavailable for export")
	}
	if t.Fn.Body != nil {
		return e.functionOriginValue(t)
	}
	var src ast.Expr = ast.Clone(t.Fn.Src)
	params := adt.FunctionTypeParameters(t)
	if len(params) != 0 {
		q := &ast.Quantifier{Body: src}
		for _, p := range params {
			q.Params = append(q.Params, ast.Clone(p.Src))
		}
		src = q
	}
	args := adt.FunctionTypeArguments(t)
	captures := make(map[ast.Node]adt.Value)
	for _, capture := range t.Fn.Captures {
		id, ok := capture.Source().(*ast.Ident)
		if !ok || id.Node == nil {
			continue
		}
		v, complete := e.ctx.Evaluate(t.Env, capture)
		if !complete || !e.exportableCapture(v, make(map[adt.Value]bool)) {
			return e.quantifiedExportError("captured value %s cannot be exported independently", id.Name)
		}
		if vertex, ok := v.(*adt.Vertex); ok {
			v = vertex.ToDataAll(e.ctx)
		}
		captures[id.Node] = adt.Unwrap(v)
	}
	src = astutil.Apply(src, func(c astutil.Cursor) bool {
		if id, ok := c.Node().(*ast.Ident); ok {
			if value := captures[id.Node]; value != nil {
				c.Replace(e.value(value))
				return false
			}
			if param, ok := id.Node.(*ast.TypeParam); ok {
				if value := args[param]; value != nil {
					c.Replace(e.value(value))
					return false
				}
			}
		}
		return true
	}, nil).(ast.Expr)
	return e.funcExprSrc(src, "quantified")
}

func (e *exporter) exportableCapture(value adt.Value, seen map[adt.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	defer delete(seen, value)
	if value.Kind()&(adt.FuncKind|adt.OpaqueKind) != 0 {
		return false
	}
	if v, ok := value.(*adt.Vertex); ok {
		v.Finalize(e.ctx)
		if adt.Validate(e.ctx, v, &adt.ValidateConfig{Concrete: true}) != nil {
			return false
		}
		for _, a := range v.Arcs {
			if a.ArcType == adt.ArcMember && !a.Label.IsLet() && !e.exportableCapture(a, seen) {
				return false
			}
		}
		return true
	}
	return adt.IsConcrete(value)
}

func (e *exporter) funcExprSrc(src ast.Expr, experiment string) ast.Expr {

	// Collect the import bindings of the original literal by name.
	var imports map[string]*ast.ImportSpec
	ast.Walk(src, func(n ast.Node) bool {
		if x, ok := n.(*ast.Ident); ok {
			if spec, ok := x.Node.(*ast.ImportSpec); ok {
				if imports == nil {
					imports = map[string]*ast.ImportSpec{}
				}
				imports[x.Name] = spec
			}
		}
		return true
	}, nil)

	b, err := format.Node(src)
	if err != nil {
		return src
	}
	// The re-parse must have the functions experiment active: without it,
	// the expression after the colon of a literal written without "->" is
	// read as a return type, in which parameter references are rejected.
	// ParseExpr offers no way to enable an experiment, so the literal is
	// parsed as the sole embedding of a synthetic file carrying the
	// experiment attribute.
	f, err := parser.ParseFile("",
		fmt.Sprintf("@experiment(%s)\n\n%s", experiment, b),
		parser.ParseComments)
	if err != nil {
		return src
	}
	// ParseFile resolves the copy internally, binding parameters and other
	// local declarations within the literal; what remains unresolved is
	// either an import reference, restored below, or a capture that must
	// resolve lexically in the output.
	var expr ast.Expr
	for _, d := range f.Decls {
		if embed, ok := d.(*ast.EmbedDecl); ok {
			expr = embed.Expr
			break
		}
	}
	if expr == nil {
		return src
	}
	// The blank line following the attribute leaves the literal positioned
	// at a section start, which would render it on its own line.
	ast.SetRelPos(expr, token.NoRelPos)
	if imports != nil || e.originNames != nil {
		ast.Walk(expr, func(n ast.Node) bool {
			if x, ok := n.(*ast.Ident); ok && x.Node == nil {
				if spec, ok := imports[x.Name]; ok {
					x.Node = spec
				}
				if origin := e.originNames[x.Name]; origin != nil {
					x.Node = origin
				}
			}
			return true
		}, nil)
	}
	return expr
}

func (e *exporter) builtinValidator(n *adt.BuiltinValidator) ast.Expr {
	call := ast.NewCall(e.builtin(n.Builtin))
	for _, a := range n.Args {
		call.Args = append(call.Args, e.value(a))
	}
	return call
}

func (e *exporter) listComposite(v *adt.Vertex) ast.Expr {
	l := &ast.ListLit{}
	for _, a := range v.Arcs {
		if !a.Label.IsInt() {
			continue
		}
		elem := e.vertex(a)

		if e.cfg.ShowDocs {
			docs := ExtractDoc(a)
			ast.SetComments(elem, docs)
		}

		l.Elts = append(l.Elts, elem)
	}
	m, ok := v.BaseValue.(*adt.ListMarker)
	if !e.cfg.TakeDefaults && ok && m.IsOpen {
		ellipsis := &ast.Ellipsis{}
		typ := &adt.Vertex{
			Parent: v,
			Label:  adt.AnyIndex,
		}
		v.MatchAndInsert(e.ctx, typ)
		typ.Finalize(e.ctx)
		if typ.Kind() != adt.TopKind {
			ellipsis.Type = e.value(typ)
		}

		l.Elts = append(l.Elts, ellipsis)
	}
	return l
}

func (e exporter) showArcs(v *adt.Vertex) bool {
	p := e.cfg
	if !p.ShowHidden && !p.ShowDefinitions {
		return false
	}
	for _, a := range v.Arcs {
		switch {
		case a.Label.IsDef() && p.ShowDefinitions:
			return true
		case a.Label.IsHidden() && p.ShowHidden:
			return true
		}
	}
	return false
}

func (e *exporter) structComposite(v *adt.Vertex, attrs []*ast.Attribute) ast.Expr {
	s := e.top().scope

	showRegular := false
	switch x := v.BaseValue.(type) {
	case *adt.StructMarker:
		showRegular = true
	case *adt.ListMarker:
		// As lists may be long, put them at the end.
		defer e.addEmbed(e.listComposite(v))
	case *adt.Bottom:
		if !e.cfg.ShowErrors || !x.ChildError {
			// Should not be reachable, but just in case. The output will be
			// correct.
			e.addEmbed(e.value(x))
			return s
		}
		// Always also show regular fields, even when list, as we are in
		// debugging mode.
		showRegular = true
		// TODO(perf): do something better
		for _, a := range v.Arcs {
			if a.Label.IsInt() {
				defer e.addEmbed(e.listComposite(v))
				break
			}
		}

	case adt.Value:
		e.addEmbed(e.value(x))
	}

	for _, a := range attrs {
		s.Elts = append(s.Elts, a)
	}

	p := e.cfg
	for _, label := range VertexFeatures(e.ctx, v) {
		show := false
		switch label.Typ() {
		case adt.StringLabel:
			show = showRegular
		case adt.IntLabel:
			continue
		case adt.DefinitionLabel:
			show = p.ShowDefinitions
		case adt.HiddenLabel, adt.HiddenDefinitionLabel:
			lpkg := label.PkgID(e.ctx)
			pkgID := cmp.Or(e.pkgID, "_")
			show = p.ShowHidden && lpkg == pkgID
		}
		if !show {
			continue
		}

		f := &ast.Field{Label: e.stringLabel(label)}

		e.addField(label, f, f.Value)

		if label.IsDef() {
			e.inDefinition++
		}

		arc := v.LookupRaw(label)
		if arc == nil {
			continue
		}

		if arc.ArcType == adt.ArcOptional && !p.ShowOptional {
			continue
		}
		// TODO: report an error for required fields in Final mode?
		// This package typically does not create errors that did not result
		// from evaluation already.

		f.Constraint = arc.ArcType.Token()

		f.Value = e.vertex(arc.DerefValue())

		if label.IsDef() {
			e.inDefinition--
		}

		if p.ShowAttributes {
			f.Attrs = ExtractFieldAttrs(arc)
		}

		if p.ShowDocs {
			docs := ExtractDoc(arc)
			ast.SetComments(f, docs)
		}

		s.Elts = append(s.Elts, f)
	}

	return s
}

// An unsupported serialization is an export error, not the bottom predicate
// or a weakened top predicate. The AST placeholder carries the diagnostic for
// callers of Value.Syntax, whose API has no separate error return.
func (e *exporter) quantifiedExportError(format string, args ...interface{}) ast.Expr {
	err := &IncompleteError{errors.Newf(token.NoPos, format, args...)}
	e.errs = errors.Append(e.errs, err)
	return e.bottom(&adt.Bottom{Code: adt.IncompleteError, Err: err})
}

// IncompleteError reports a value or obligation that has no faithful source
// serialization in the current exporter. It is distinct from a malformed
// internal representation and from a contradiction in the input program.
type IncompleteError struct{ incompleteCause }

type incompleteCause = errors.Error
