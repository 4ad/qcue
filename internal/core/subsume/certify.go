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
	"cuelang.org/go/internal/core/adt"
	"maps"
)

// Certify checks quantified-profile implementations under arbitrary packets
// and rigid type variables. This is deliberately independent of evaluation's
// finite counterexample search. Unsupported proof obligations are incomplete,
// not contradictory, and the target annotation is never evidence for itself.
func Certify(ctx *adt.OpContext, root *adt.Vertex) *adt.Bottom {
	p := &certifier{ctx: ctx, active: make(map[*adt.Function]bool),
		hypotheses: make(map[*adt.FuncValue]bool), scopes: make(map[*adt.Environment]*proofScope)}
	seen := make(map[*adt.Vertex]bool)
	var visit func(*adt.Vertex) *adt.Bottom
	visit = func(v *adt.Vertex) *adt.Bottom {
		v = v.DerefValue()
		if seen[v] {
			return nil
		}
		seen[v] = true
		v.Finalize(ctx)
		if b := v.Bottom(); b != nil {
			return b
		}
		if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok && f.Fn.Quantified {
			if !p.implementation(f) {
				return &adt.Bottom{Src: f.Source(), Code: adt.IncompleteError,
					Err: ctx.Newf("function conformance remains unproved")}
			}
		}
		for _, a := range v.Arcs {
			if a.ArcType == adt.ArcMember && !a.Label.IsLet() && a.Label.IsRegular() {
				if b := visit(a); b != nil {
					return b
				}
			}
		}
		return nil
	}
	return visit(root)
}

type proofScope struct {
	values map[adt.Feature]adt.Value
	fields map[adt.Feature]adt.Expr
	active map[adt.Feature]bool
}

type certifier struct {
	ctx        *adt.OpContext
	active     map[*adt.Function]bool
	hypotheses map[*adt.FuncValue]bool
	scopes     map[*adt.Environment]*proofScope
}

func (p *certifier) schema(env *adt.Environment, x adt.Expr) adt.Value {
	if x == nil {
		return &adt.Top{}
	}
	if env == nil {
		env = &adt.Environment{Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	}
	v, ok := p.ctx.Evaluate(env, x)
	if !ok || v == nil {
		return nil
	}
	if vertex, ok := v.(*adt.Vertex); ok {
		vertex.Finalize(p.ctx)
	}
	if _, ok := adt.Unwrap(v).(*adt.Bottom); ok {
		return nil
	}
	return v
}

func (p *certifier) includes(want, got adt.Value) bool {
	if want == nil || got == nil {
		return false
	}
	s := &subsumer{ctx: p.ctx}
	return s.values(want, got)
}

func (p *certifier) frame(up *adt.Environment, values map[adt.Feature]adt.Value) *adt.Environment {
	// The proof frame is never passed to the runtime evaluator as a packet.
	// References to these fields are synthesized by expr below.
	env := &adt.Environment{Up: up, Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	p.scopes[env] = &proofScope{values: values, active: make(map[adt.Feature]bool)}
	return env
}

func (p *certifier) implementation(f *adt.FuncValue) bool {
	clauses := append([]adt.FuncType{{Fn: f.Fn, Env: f.Env}}, f.Types...)
	for _, target := range clauses {
		if !p.function(f, target) {
			return false
		}
	}
	return true
}

func (p *certifier) function(f *adt.FuncValue, target adt.FuncType) bool {
	if f.Fn.Body == nil || f.IsPartial() || p.active[f.Fn] {
		return false
	}
	if f.Src != nil && (f.Src.Extern.IsValid() || f.Src.Effect != nil) {
		return false
	}
	savedHypotheses := p.hypotheses
	p.hypotheses = maps.Clone(p.hypotheses)
	defer func() { p.hypotheses = savedHypotheses }()
	p.active[f.Fn] = true
	defer delete(p.active, f.Fn)
	s := &subsumer{ctx: p.ctx}
	target, source, ok := s.capabilityScopes(target, adt.FuncType{Fn: f.Fn, Env: f.Env})
	if !ok {
		return false
	}
	// Prove coverage without using the implementation's result annotation.
	a, b := *target.Fn, *source.Fn
	a.Ret, b.Ret = nil, nil
	if !s.capabilitySignature(adt.FuncType{Fn: &a, Env: target.Env}, adt.FuncType{Fn: &b, Env: source.Env}) {
		return false
	}
	matches := adt.MatchFuncValueParams(target.Fn, &adt.FuncValue{Fn: source.Fn})
	values := make(map[adt.Feature]adt.Value)
	for i, j := range matches {
		arg := target.Fn.Params[i]
		if arg.ArcType == adt.ArcOptional || arg.Default != nil {
			// An optional packet has presence branches. The current rule
			// leaves their joint proof pending instead of assuming presence.
			return false
		}
		v := p.schema(target.Env, arg.Value)
		if v == nil {
			return false
		}
		values[source.Fn.Params[j].Local] = v
		p.assume(v, make(map[adt.Value]bool))
	}
	for _, arg := range source.Fn.Params {
		if _, ok := values[arg.Local]; ok {
			continue
		}
		if arg.Default != nil {
			v := p.expr(source.Env, arg.Default)
			if !p.includes(p.schema(source.Env, arg.Value), v) {
				return false
			}
			values[arg.Local] = v
		}
	}
	env := p.frame(source.Env, values)
	body := p.expr(env, source.Fn.Body)
	return p.includes(p.schema(target.Env, target.Fn.Ret), body)
}

// Only an admitted callback packet introduces a conformance hypothesis.
// Ordinary named functions must have their bodies checked before a call can
// use their result annotations. This prevents circular annotation proofs.
func (p *certifier) assume(v adt.Value, seen map[adt.Value]bool) {
	if v == nil || seen[v] {
		return
	}
	seen[v] = true
	if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
		if f.Fn.Body == nil {
			p.hypotheses[f] = true
		}
		return
	}
	if v, ok := v.(*adt.Vertex); ok {
		for _, a := range v.Arcs {
			p.assume(a, seen)
		}
	}
}

func (p *certifier) expr(env *adt.Environment, expr adt.Expr) adt.Value {
	switch x := expr.(type) {
	case *adt.Null, *adt.Bool, *adt.Num, *adt.String, *adt.Bytes:
		return x.(adt.Value)
	case *adt.Builtin:
		return x
	case *adt.FieldReference:
		e := env
		for range x.UpCount {
			if e == nil {
				return nil
			}
			e = e.Up
		}
		if scope := p.scopes[e]; scope != nil {
			if v := scope.values[x.Label]; v != nil {
				return v
			}
			if scope.active[x.Label] {
				return nil
			}
			if field := scope.fields[x.Label]; field != nil {
				scope.active[x.Label] = true
				v := p.expr(e, field)
				delete(scope.active, x.Label)
				scope.values[x.Label] = v
				return v
			}
			return nil
		}
		v := p.schema(env, x)
		if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
			if !p.implementation(f) {
				return nil
			}
			return f
		}
		if vertex, ok := v.(*adt.Vertex); ok {
			if adt.Validate(p.ctx, vertex, &adt.ValidateConfig{Concrete: true}) != nil {
				return nil
			}
		} else if v == nil || !adt.IsConcrete(v) {
			return nil
		}
		return v
	case *adt.SelectorExpr:
		v, ok := p.expr(env, x.X).(*adt.Vertex)
		if !ok {
			return nil
		}
		field := v.LookupRaw(x.Sel)
		if field == nil || (field.ArcType != adt.ArcMember && field.ArcType != adt.ArcRequired) {
			return nil
		}
		return field
	case *adt.IndexExpr:
		v, ok := p.expr(env, x.X).(*adt.Vertex)
		if !ok || !v.IsList() {
			return nil
		}
		n, ok := x.Index.(*adt.Num)
		if !ok {
			return nil
		}
		i, err := n.X.Int64()
		if err != nil || i < 0 {
			return nil
		}
		for a := range v.Elems() {
			if i == 0 {
				return a
			}
			i--
		}
		return nil
	case *adt.Function:
		f := &adt.FuncValue{Fn: x, Src: x.Src, Env: env}
		if !p.function(f, adt.FuncType{Fn: x, Env: env}) {
			return nil
		}
		return f
	case *adt.Quantified:
		// Creating the lexical telescope does not execute the body.
		v := p.schema(env, x)
		f, ok := adt.Unwrap(v).(*adt.FuncValue)
		if !ok || !p.implementation(f) {
			return nil
		}
		return f
	case *adt.StructLit:
		e := p.frame(env, make(map[adt.Feature]adt.Value))
		if len(x.Decls) == 1 {
			if embedded, ok := x.Decls[0].(adt.Expr); ok {
				return p.expr(e, embedded)
			}
		}
		scope := p.scopes[e]
		scope.fields = make(map[adt.Feature]adt.Expr)
		for _, decl := range x.Decls {
			f, ok := decl.(*adt.Field)
			if !ok || f.ArcType != adt.ArcMember {
				return nil
			}
			if scope.fields[f.Label] != nil {
				return nil
			}
			scope.fields[f.Label] = f.Value
		}
		out := &adt.StructLit{}
		for _, decl := range x.Decls {
			f := decl.(*adt.Field)
			v := p.expr(e, &adt.FieldReference{Label: f.Label})
			if v == nil {
				return nil
			}
			out.Decls = append(out.Decls, &adt.Field{Label: f.Label, Value: v})
		}
		return p.schema(nil, out)
	case *adt.ListLit:
		env = p.frame(env, make(map[adt.Feature]adt.Value))
		out := &adt.ListLit{}
		var elements []adt.Value
		variable := false
		for _, elem := range x.Elems {
			if comp, ok := elem.(*adt.Comprehension); ok {
				variable = true
				v, empty := p.comprehension(env, comp)
				if empty {
					continue
				}
				if v == nil {
					return nil
				}
				elements = append(elements, v)
				continue
			}
			e, ok := elem.(adt.Expr)
			if !ok {
				return nil
			}
			v := p.expr(env, e)
			if v == nil {
				return nil
			}
			out.Elems = append(out.Elems, v)
			elements = append(elements, v)
		}
		if variable {
			out.Elems = nil
			if len(elements) != 0 {
				out.Elems = append(out.Elems, &adt.Ellipsis{Value: proofUnion(elements)})
			}
		}
		return p.schema(nil, out)
	case *adt.Interpolation:
		for _, part := range x.Parts {
			v := p.expr(env, part)
			if v == nil || v.Kind()&(adt.NumberKind|adt.StringKind|adt.BoolKind) != v.Kind() {
				return nil
			}
		}
		return &adt.BasicType{K: x.K}
	case *adt.BinaryExpr:
		a, b := p.expr(env, x.X), p.expr(env, x.Y)
		if a == nil || b == nil {
			return nil
		}
		ka, kb := a.Kind(), b.Kind()
		switch x.Op {
		case adt.AddOp, adt.SubtractOp, adt.MultiplyOp:
			if ka&adt.NumberKind == ka && kb&adt.NumberKind == kb {
				return &adt.BasicType{K: ka | kb}
			}
			if x.Op == adt.AddOp && ka == adt.StringKind && kb == adt.StringKind {
				return &adt.BasicType{K: adt.StringKind}
			}
		case adt.EqualOp, adt.NotEqualOp:
			if ka == kb && ka&(adt.NumberKind|adt.StringKind|adt.BoolKind|adt.NullKind) == ka {
				return &adt.BasicType{K: adt.BoolKind}
			}
		case adt.LessThanOp, adt.LessEqualOp, adt.GreaterThanOp, adt.GreaterEqualOp:
			if ka&adt.NumberKind == ka && kb&adt.NumberKind == kb {
				return &adt.BasicType{K: adt.BoolKind}
			}
		}
		return nil
	case *adt.CallExpr:
		return p.call(env, x)
	}
	return nil
}

func (p *certifier) call(env *adt.Environment, call *adt.CallExpr) adt.Value {
	if call.Partial {
		return nil
	}
	callee := adt.Unwrap(p.expr(env, call.Fun))
	if b, ok := callee.(*adt.Builtin); ok && b.Name == "len" && len(call.Args) == 1 {
		v := p.expr(env, call.Args[0])
		if v != nil && v.Kind()&(adt.ListKind|adt.StructKind|adt.StringKind|adt.BytesKind) == v.Kind() {
			return &adt.BasicType{K: adt.IntKind}
		}
		return nil
	}
	f, ok := callee.(*adt.FuncValue)
	if !ok {
		return nil
	}
	if f.Src != nil && f.Src.Effect != nil {
		return nil
	}
	packet := &adt.Function{}
	args := make([]adt.Value, len(call.Args))
	for i, arg := range call.Args {
		args[i] = p.expr(env, arg)
		if args[i] == nil {
			return nil
		}
		label := adt.InvalidLabel
		if i < len(call.ArgLabels) {
			label = call.ArgLabels[i]
		}
		packet.Params = append(packet.Params, adt.FuncParam{Value: args[i], Label: label, Positional: label == adt.InvalidLabel})
	}
	target := adt.FuncType{Fn: packet}
	source := adt.FuncType{Fn: f.Fn, Env: f.Env}
	if len(adt.FunctionTypeParameters(source)) != 0 {
		var b *adt.Bottom
		source, b = adt.InstantiateFunctionType(p.ctx, source, target)
		if b != nil {
			return nil
		}
	}
	s := &subsumer{ctx: p.ctx}
	noResult := *source.Fn
	noResult.Ret = nil
	if !s.capabilitySignature(target, adt.FuncType{Fn: &noResult, Env: source.Env}) {
		return nil
	}
	if !p.hypotheses[f] && !p.implementation(f) {
		return nil
	}
	return p.schema(source.Env, source.Fn.Ret)
}

func proofUnion(values []adt.Value) adt.Value {
	if len(values) == 1 {
		return values[0]
	}
	return &adt.Disjunction{Values: values}
}

// Every operational list is finite. Iterating its element predicate proves
// a finite comprehension uniformly without testing a representative list.
// Conditions may suppress elements; they must themselves be total booleans.
func (p *certifier) comprehension(env *adt.Environment, comp *adt.Comprehension) (adt.Value, bool) {
	if comp.Fallback != nil {
		return nil, false
	}
	for _, clause := range comp.Clauses {
		switch x := clause.(type) {
		case *adt.ForClause:
			v, ok := p.expr(env, x.Src).(*adt.Vertex)
			if !ok || !v.IsList() {
				return nil, false
			}
			var elems []adt.Value
			for a := range v.Elems() {
				elems = append(elems, a)
			}
			if !v.IsClosedList() {
				tail := &adt.Vertex{Label: adt.MakeIntLabel(adt.IntLabel, int64(len(elems)))}
				v.MatchAndInsert(p.ctx, tail)
				tail.Finalize(p.ctx)
				if tail.Bottom() != nil {
					return nil, false
				}
				elems = append(elems, tail)
			}
			if len(elems) == 0 {
				return nil, true
			}
			values := map[adt.Feature]adt.Value{x.Value: proofUnion(elems)}
			if x.Key != adt.InvalidLabel {
				values[x.Key] = &adt.BasicType{K: adt.IntKind}
			}
			env = p.frame(env, values)
		case *adt.IfClause:
			v := p.expr(env, x.Condition)
			if v == nil || v.Kind() != adt.BoolKind {
				return nil, false
			}
		case *adt.LetClause:
			v := p.expr(env, x.Expr)
			if v == nil {
				return nil, false
			}
			env = p.frame(env, map[adt.Feature]adt.Value{x.Label: v})
		default:
			return nil, false
		}
	}
	return p.expr(env, comp.Value), false
}
