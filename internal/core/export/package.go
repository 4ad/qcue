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
	"fmt"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/core/adt"
)

// Keep a package's reconstruction environment outside the resulting value.
// Embedding a seal beside its helpers would put those helpers in its lexical
// refinement scope: refining the package could then construct a fresh seal.
func (e *exporter) withPackageDecls(x ast.Expr) ast.Expr {
	if len(e.originDecls) == 0 {
		return x
	}
	name := e.uniqueAlias("CUEExport")
	s := ast.NewStruct(ast.NewIdent(name), x)
	s.Elts = append(s.Elts, e.originDecls...)
	e.originDecls = nil
	return ast.NewSel(s, name)
}

func (e *exporter) packageValue(v *adt.Vertex) ast.Expr {
	if b := v.Bottom(); b != nil {
		return e.bottom(b)
	}
	source := v.PackageSource()
	view := v.PackageView()
	// A result annotation may close the public record without otherwise
	// refining it. Reapply that closedness without replaying a factory call,
	// which would construct a different seal.
	closed := view != nil && v.IsClosedStruct() && !view.IsClosedStruct()
	if closed {
		copy := *view
		copy.ClosedRecursive, copy.ClosedNonRecursive = v.ClosedRecursive, v.ClosedNonRecursive
		view = &copy
	}
	if source.Seal == nil || e.samePackageView(v, view, make(map[[2]*adt.Vertex]bool)) {
		x := e.packageSource(source)
		if closed {
			x = ast.NewCall(ast.NewIdent("close"), x)
		}
		return x
	}
	var conjuncts []conjunct
	for c := range v.LeafConjuncts() {
		conjuncts = append(conjuncts, conjunct{c: c})
	}
	if len(conjuncts) == 1 {
		c := conjuncts[0].c
		if c.Expr() == source.Seal && c.Env == source.Env {
			return e.packageSource(source)
		}
	}
	if f := e.packageValues[v]; f != nil {
		return e.closureReference(f)
	}
	if e.packageValues == nil {
		e.packageValues = make(map[*adt.Vertex]*ast.Field)
	}
	f := e.closureField()
	e.packageValues[v] = f
	// Keep refinements in their source form, including optional fields and
	// patterns. Export their free references in the same graph, independently
	// of the caller's lexical scope and output projection options.
	stack, pivot, residual, inline, cfg := e.stack, e.pivotter, e.packageResidual, e.inlineFreeRefs, e.cfg
	e.stack, e.pivotter, e.packageResidual, e.inlineFreeRefs, e.cfg = nil, nil, v, true, All
	g := e.graph()
	g.depth++
	f.Value.(*ast.ParenExpr).X = e.mergeValues(adt.InvalidLabel, v, conjuncts, v.Conjuncts...)
	g.depth--
	e.stack, e.pivotter, e.packageResidual, e.inlineFreeRefs, e.cfg = stack, pivot, residual, inline, cfg
	return e.closureReference(f)
}

// Structural equality alone omits pattern constraints. Only elide a view's
// source refinements when its predicates as well as its materialized fields
// are unchanged from the original public view.
func (e *exporter) samePackageView(a, b *adt.Vertex, seen map[[2]*adt.Vertex]bool) bool {
	if b == nil {
		return false
	}
	a, b = a.DerefValue(), b.DerefValue()
	if a == b {
		return true
	}
	key := [2]*adt.Vertex{a, b}
	if seen[key] {
		return false
	}
	seen[key] = true
	defer delete(seen, key)
	if a.PatternConstraints != b.PatternConstraints || !adt.Equal(e.ctx, a, b, adt.CheckStructural) {
		return false
	}
	for _, arc := range a.Arcs {
		if arc.Label.IsLet() {
			continue
		}
		other := b.LookupRaw(arc.Label)
		if other == nil || !e.samePackageView(arc, other, seen) {
			return false
		}
	}
	return true
}

// A seal is a generative expression. Emit each evaluated construction once,
// even when several fields or operations refer to it. Its source, including
// its private implementation, belongs in CUE output; its evaluated public
// fields alone are not an equivalent representation.
func (e *exporter) packageSource(source adt.PackageSource) ast.Expr {
	if source.Seal == nil || source.Seal.Src == nil {
		return e.quantifiedExportError("package construction is unavailable for export")
	}
	if f := e.packages[source]; f != nil {
		return e.closureReference(f)
	}
	if e.packages == nil {
		e.packages = make(map[adt.PackageSource]*ast.Field)
	}
	e.markUsedFeatures(source.Seal)
	f := e.closureField()
	e.packages[source] = f
	g := e.graph()
	g.depth++
	f.Value.(*ast.ParenExpr).X = e.scopedSource(source.Env, source.Seal.Src, source.Seal.References, source.Seal.Captures)
	g.depth--
	return e.closureReference(f)
}

// scopedSource retains binders and lexical aliases while substituting free
// references from their original environment. Predicate dependencies retain
// their constraints; runtime captures retain their evaluated identity.
func (e *exporter) scopedSource(env *adt.Environment, src ast.Expr, references, captures []adt.Expr) ast.Expr {
	if src == nil {
		return e.quantifiedExportError("package source is unavailable for export")
	}
	refs := make(map[ast.Node]ast.Expr)
	names := make(map[ast.Node]string)
	runtime := make(map[ast.Node]bool)
	for _, ref := range captures {
		runtime[referenceKey(ref)] = true
	}
	for _, ref := range references {
		v, complete := e.ctx.Evaluate(env, ref)
		if !complete || v == nil {
			return e.quantifiedExportError("package dependency %s is unresolved", ref.Source())
		}
		key := referenceKey(ref)
		if runtime[key] {
			if !e.exportableCapture(v) {
				return e.quantifiedExportError("package capture %s cannot be exported independently", ref.Source())
			}
			refs[key] = e.runtimeCaptureValue(v)
		} else {
			refs[key] = e.predicateValue(v)
		}
	}
	body := e.withLexicalAliases(ast.Clone(src), nil, names)
	body = astutil.Apply(body, func(c astutil.Cursor) bool {
		if id, ok := c.Node().(*ast.Ident); ok {
			if name := names[id.Node]; name != "" {
				c.Replace(ast.NewIdent(name))
				return false
			}
			if ref := refs[id.Node]; ref != nil {
				c.Replace(&ast.ParenExpr{X: ast.Clone(ref)})
				return false
			}
		}
		return true
	}, nil).(ast.Expr)
	return body
}

func (e *exporter) packageOperation(call *adt.OpaqueCall) ast.Expr {
	source, path := call.PackageProjection()
	if source.Seal == nil {
		return e.quantifiedExportError("opaque operation has no package projection for export")
	}
	typ := ast.NewIdent(e.uniqueAlias("CUEAbstract"))
	view := ast.NewIdent(e.uniqueAlias("CUEPackage"))
	var body ast.Expr = ast.NewIdent(view.Name)
	for _, label := range path {
		if label.IsInt() {
			body = &ast.IndexExpr{X: body, Index: ast.NewLit(token.INT, fmt.Sprint(label.Index()))}
		} else {
			body = &ast.SelectorExpr{X: body, Sel: e.stringLabel(label)}
		}
	}
	result := e.uniqueAlias("CUEOperation")
	opened := &ast.OpenExpr{Value: e.packageSource(source), Type: typ, View: view,
		Body: ast.NewStruct(ast.NewIdent(result), body)}
	return ast.NewSel(&ast.ParenExpr{X: opened}, result)
}
