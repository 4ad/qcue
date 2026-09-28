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

package subsume

import (
	"slices"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/internal/core/adt"
)

// recordShape retains the source's structural operators while erasing field
// contents already checked by recordMeet. Replaying this skeleton through the
// evaluator preserves definition boundaries, embeddings, and recursive
// closedness without implementing a second set of CUE closedness rules.
func (p *inference) recordShape(record *proofRecord, bindings []proofBinding, active map[proofBinding]bool) adt.Expr {
	var shape adt.Expr
	for _, binding := range bindings {
		x := p.recordShapeExpr(record, binding, active)
		if x == nil {
			return nil
		}
		if shape == nil {
			shape = x
		} else {
			shape = &adt.BinaryExpr{Op: adt.AndOp, X: shape, Y: x}
		}
	}
	return shape
}

func (p *inference) recordShapeExpr(record *proofRecord, binding proofBinding, active map[proofBinding]bool) adt.Expr {
	if !p.step() || active[binding] {
		return nil
	}
	active[binding] = true
	defer delete(active, binding)
	shape := func(env *adt.Environment, expr adt.Expr) adt.Expr {
		return p.recordShapeExpr(record, proofBinding{env, expr}, active)
	}
	switch x := binding.expr.(type) {
	case *adt.ListLit:
		env := p.frame(binding.env, make(map[adt.Feature]adt.Value))
		out := &adt.ListLit{Src: x.Src}
		for _, elem := range x.Elems {
			if tail, ok := elem.(*adt.Ellipsis); ok {
				var value adt.Expr
				if tail.Value != nil {
					value = shape(env, tail.Value)
					if value == nil {
						return nil
					}
				}
				out.Elems = append(out.Elems, &adt.Ellipsis{Value: value})
			} else if expr, ok := elem.(adt.Expr); ok {
				value := shape(env, expr)
				if value == nil {
					return nil
				}
				out.Elems = append(out.Elems, value)
			} else {
				return nil
			}
		}
		return out
	case *adt.DisjunctionExpr:
		out := *x
		out.Values = slices.Clone(x.Values)
		for i := range out.Values {
			out.Values[i].Val = shape(binding.env, out.Values[i].Val)
			if out.Values[i].Val == nil {
				return nil
			}
		}
		return &out
	case *adt.StructLit:
		env := record.frames[binding]
		if env == nil {
			record = p.prepareRecord([]proofBinding{binding})
			if record == nil {
				return nil
			}
			env = record.frames[binding]
		}
		out := &adt.StructLit{Src: x.Src}
		for _, decl := range x.Decls {
			switch field := decl.(type) {
			case *adt.Field:
				value := p.recordShapeExpr(record, proofBinding{env, field.Value}, active)
				if value == nil {
					return nil
				}
				copy := *field
				copy.Value = value
				out.Decls = append(out.Decls, &copy)
			case *adt.LetField:
				// Uses are expanded in their declaration scopes below.
			case *adt.Ellipsis:
				if field.Value != nil {
					return nil
				}
				out.Decls = append(out.Decls, field)
			case adt.Expr:
				value := p.recordShapeExpr(record, proofBinding{env, field}, active)
				if value == nil {
					return nil
				}
				out.Decls = append(out.Decls, value)
			default:
				return nil
			}
		}
		return out
	case *adt.FieldReference:
		fields := p.referenceBindings(binding.env, x)
		if len(fields) == 0 {
			return &adt.Top{}
		}
		value := p.recordShape(record, fields, active)
		if value != nil && x.Label.IsDef() {
			value = definitionShape(p.ctx, value)
		}
		return value
	case *adt.SelectorExpr:
		base := shape(binding.env, x.X)
		if base == nil {
			return nil
		}
		return &adt.SelectorExpr{Src: x.Src, X: base, Sel: x.Sel}
	case *adt.LetReference:
		env := binding.env
		for range x.UpCount {
			if env == nil {
				return nil
			}
			env = env.Up
		}
		return shape(env, x.X)
	case *adt.AliasApplication:
		args := make([]adt.Value, len(x.Args))
		for i, arg := range x.Args {
			args[i] = p.schema(binding.env, arg)
			if args[i] == nil {
				return nil
			}
		}
		env, err := x.Expand(p.ctx, binding.env, args)
		if err != nil {
			return nil
		}
		return shape(env, x.Template.Body)
	case *adt.BinaryExpr:
		if x.Op == adt.AndOp {
			a, b := shape(binding.env, x.X), shape(binding.env, x.Y)
			if a == nil || b == nil {
				return nil
			}
			return &adt.BinaryExpr{Op: adt.AndOp, X: a, Y: b}
		}
	case *adt.CallExpr:
		if builtin, ok := x.Fun.(*adt.Builtin); ok && builtin.Package == adt.InvalidLabel && builtin.Name == "close" && len(x.Args) == 1 {
			value := shape(binding.env, x.Args[0])
			if value == nil {
				return nil
			}
			return &adt.CallExpr{Fun: builtin, Args: []adt.Expr{value}}
		}
	}
	return &adt.Top{}
}

func definitionShape(ctx *adt.OpContext, shape adt.Expr) adt.Expr {
	label := adt.MakeIdentLabel(ctx, "#ProofRecord", "")
	return &adt.SelectorExpr{
		Src: &ast.SelectorExpr{X: ast.NewIdent("_"), Sel: ast.NewIdent("#ProofRecord")},
		X:   &adt.StructLit{Decls: []adt.Decl{&adt.Field{Label: label, Value: shape}}},
		Sel: label,
	}
}

// Transport checked member evidence onto the evaluator's constrained shape.
// Projections must see its retained closedness as well as callable evidence.
func (p *inference) recordEvidence(source, target *adt.Vertex) {
	if !p.step() {
		return
	}
	fields := make(map[adt.Feature]adt.Value)
	for _, field := range target.Arcs {
		value := p.projections[source][field.Label]
		if value == nil {
			value = field
		}
		if v, ok := value.(*adt.Vertex); ok && field.Kind() == adt.StructKind {
			p.recordEvidence(v, field)
			value = field
		}
		if field.ArcType != adt.ArcOptional {
			fields[field.Label] = value
		}
	}
	p.projections[target] = fields
	p.memberships[target] = p.memberships[source]
}
