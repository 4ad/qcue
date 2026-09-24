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
	"maps"

	"cuelang.org/go/internal/core/adt"
)

// ValidateFunction checks an implementation's contracts under arbitrary
// packets and rigid type variables. Concrete validation calls this alongside
// the checks on the closure's implementation identity and captured values.
// Unsupported proofs remain incomplete; successful concrete calls and the
// target annotation itself are not evidence of universal conformance.
func ValidateFunction(ctx *adt.OpContext, f *adt.FuncValue) *adt.Bottom {
	p := newCertifier(ctx)
	defer p.enter()()
	return p.validateFunction(ctx, f)
}

// Evaluation during a proof must retain the same active dependencies and
// budget. A fresh certifier could let a callback obligation justify itself.
func (p *certifier) enter() func() {
	check, inclusion := p.ctx.CheckFunction, p.ctx.ProveInclusion
	p.ctx.CheckFunction = p.validateFunction
	p.ctx.ProveInclusion = p.proveInclusion
	return func() {
		p.ctx.CheckFunction, p.ctx.ProveInclusion = check, inclusion
	}
}

func newCertifier(ctx *adt.OpContext) *certifier {
	return &certifier{ctx: ctx,
		hypotheses: make(map[*adt.FuncValue]bool), scopes: make(map[*adt.Environment]*proofScope),
		completed: make(map[proofKey][]*adt.FuncValue), remaining: 10000}
}

// Reuse the current proof context when validating captured composites.
// Starting another certifier would forget active proof dependencies and
// permit cycles through records or lists to justify their own annotations.
func (p *certifier) validateFunction(_ *adt.OpContext, f *adt.FuncValue) *adt.Bottom {
	if !p.implementation(f) {
		if p.remaining == 0 {
			return &adt.Bottom{Src: f.Source(), Code: adt.IncompleteError,
				Err: p.ctx.Newf("function conformance remains unproved: proof work limit reached")}
		}
		return &adt.Bottom{Src: f.Source(), Code: adt.IncompleteError,
			Err: p.ctx.Newf("function conformance remains unproved")}
	}
	return nil
}

type proofScope struct {
	values map[adt.Feature]adt.Value
	fields map[adt.Feature]adt.Expr
	active map[adt.Feature]bool
}

type certifier struct {
	ctx        *adt.OpContext
	active     []*adt.FuncValue
	hypotheses map[*adt.FuncValue]bool
	scopes     map[*adt.Environment]*proofScope
	completed  map[proofKey][]*adt.FuncValue
	attempts   []*proofAttempt
	remaining  int
}

type proofKey struct {
	f      *adt.FuncValue
	target adt.FuncType
}

// A completed proof can be reused only while its inherited hypotheses are
// still available. Hypotheses introduced by the function's own packet are
// discharged by that proof and are not prerequisites for its callers.
type proofAttempt struct {
	inherited map[*adt.FuncValue]bool
	required  map[*adt.FuncValue]bool
}

func (p *certifier) useHypothesis(f *adt.FuncValue) {
	for _, a := range p.attempts {
		if a.inherited[f] {
			a.required[f] = true
		}
	}
}

func (p *certifier) step() bool {
	if p.remaining == 0 {
		return false
	}
	p.remaining--
	return true
}

func (p *certifier) schema(env *adt.Environment, x adt.Expr) adt.Value {
	if !p.step() {
		return nil
	}
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

// Captured runtime values must be complete, including conformance of any
// functions nested inside them. Reuse this proof's dependency context.
func (p *certifier) captured(v adt.Value) adt.Value {
	if b, ok := adt.Unwrap(v).(*adt.Builtin); ok {
		if ValidateBuiltin(p.ctx, b) != nil {
			return nil
		}
		return b
	}
	if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
		if !p.implementation(f) {
			return nil
		}
		return f
	}
	if vertex, ok := v.(*adt.Vertex); ok {
		if adt.Validate(p.ctx, vertex, &adt.ValidateConfig{
			Concrete: true, Runtime: true, CheckFunction: p.validateFunction, CheckBuiltin: ValidateBuiltin,
		}) != nil {
			return nil
		}
	} else if v == nil || !adt.IsConcrete(v) {
		return nil
	}
	return v
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

func (p *certifier) function(f *adt.FuncValue, target adt.FuncType) (proved bool) {
	if !p.step() || f.Fn.Body == nil || len(p.active) >= 256 {
		return false
	}
	for _, active := range p.active {
		if active.Fn == f.Fn {
			if same, known := adt.SameFunctionInstance(p.ctx, active, f); same || !known {
				return false
			}
		}
	}
	if f.Src != nil && (f.Src.Extern.IsValid() || f.Src.Effect != nil) {
		return false
	}
	key := proofKey{f, target}
	if required, ok := p.completed[key]; ok {
		available := true
		for _, h := range required {
			available = available && p.hypotheses[h]
		}
		if available {
			for _, h := range required {
				p.useHypothesis(h)
			}
			return true
		}
	}
	attempt := &proofAttempt{inherited: p.hypotheses, required: make(map[*adt.FuncValue]bool)}
	p.attempts = append(p.attempts, attempt)
	defer func() {
		p.attempts = p.attempts[:len(p.attempts)-1]
		if proved {
			required := make([]*adt.FuncValue, 0, len(attempt.required))
			for h := range attempt.required {
				required = append(required, h)
			}
			p.completed[key] = required
		}
	}()
	savedHypotheses := p.hypotheses
	p.hypotheses = maps.Clone(p.hypotheses)
	defer func() { p.hypotheses = savedHypotheses }()
	p.active = append(p.active, f)
	defer func() { p.active = p.active[:len(p.active)-1] }()
	boundType, boundErr := f.BoundArgumentInstance(p.ctx)
	if boundErr != nil {
		return false
	}
	for i := range f.Fn.Params {
		env, expr := f.BoundArgument(i)
		if expr == nil {
			continue
		}
		// A saved argument needs both its own implementation proof and
		// membership in the parameter's domain. The original body proof
		// below introduces arbitrary admitted arguments, not these values.
		value := p.captured(p.schema(env, expr))
		want := p.schema(boundType.Env, boundType.Fn.Params[i].Value)
		if value == nil || want == nil || !p.proveInclusion(p.ctx, want, value) {
			return false
		}
	}
	s := &subsumer{ctx: p.ctx}
	source := adt.FuncType{Fn: f.Fn, Env: f.Env}
	partial := target.Partial()
	if partial == nil && f.IsPartial() && target.Fn != f.Fn {
		// A new contract, including a boundary's callback interface,
		// describes the residual packet of the supplied closure.
		partial = f
	}
	if partial != nil {
		source.Fn = partial.ResidualSignature()
	}
	if target.Fn == f.Fn {
		// Retained views of the same erased implementation include its
		// original telescope, even after selecting concrete type arguments.
		// Check each such obligation in its own type scope. Runtime capture
		// equality is checked independently by the closure identity rules.
		source.Env = target.Env
	}
	if len(adt.FunctionTypeParameters(source)) == 0 {
		// Prove a universally constrained monomorphic implementation under
		// fresh rigid inputs. This checks its body; it does not generalize
		// a monomorphic callback from an annotation alone.
		for _, param := range adt.FunctionTypeParameters(target) {
			bound := p.schema(target.Env, param.Bound)
			if bound == nil {
				return false
			}
			target = adt.BindFunctionTypes(target, []adt.Value{&adt.RigidType{Param: param, Bound: bound}})
		}
	}
	if boundary, ok := source.Fn.Body.(*adt.OpaqueCall); ok {
		for _, alternative := range boundary.ProofAlternatives() {
			target, source, ok := s.capabilityScopes(target, adt.FuncType{Fn: alternative.Fn, Env: alternative.Env})
			if !ok {
				continue
			}
			boundary := alternative.Fn.Body.(*adt.OpaqueCall)
			advertised, implementation, required, ok := boundary.ProofTypes(p.ctx, source.Env)
			if ok && s.capabilitySignature(target, advertised) && p.implementation(implementation) &&
				p.function(implementation, required) {
				return true
			}
		}
		return false
	}
	target, source, ok := s.capabilityScopes(target, source)
	if !ok {
		return false
	}
	target, ok = s.completeProtocol(target, source)
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
	if partial != nil {
		for i, arg := range f.Fn.Params {
			env, expr := partial.BoundArgument(i)
			if expr == nil {
				continue
			}
			v := p.captured(p.schema(env, expr))
			want := p.schema(source.Env, arg.Value)
			if want == nil || v == nil || !p.proveInclusion(p.ctx, want, v) {
				return false
			}
			values[arg.Local] = v
		}
	}
	for i, j := range matches {
		arg := target.Fn.Params[i]
		if arg.ArcType == adt.ArcOptional {
			// An optional packet has presence branches. The current rule
			// leaves their joint proof pending instead of assuming presence.
			return false
		}
		v := p.schema(target.Env, arg.Value)
		if v == nil {
			return false
		}
		if arg.Default != nil {
			// The contract admits omission. Prove that the implementation's
			// own default supplies a value in the same domain as an explicit
			// argument, so the body proof covers both cases. A target's
			// default must never stand in for the implementation's default.
			defaultExpr := source.Fn.Params[j].Default
			if defaultExpr == nil || !p.includes(v, p.expr(source.Env, defaultExpr)) {
				return false
			}
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
	if !p.step() {
		return nil
	}
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
		return p.captured(p.schema(env, x))
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
	case *adt.PackageOpen:
		typ, view := x.ProofView(p.ctx, p.expr(env, x.Value))
		if typ == nil || view == nil {
			return nil
		}
		saved := p.hypotheses
		p.hypotheses = maps.Clone(saved)
		defer func() { p.hypotheses = saved }()
		p.assume(view, make(map[adt.Value]bool))
		e := p.frame(env, map[adt.Feature]adt.Value{x.Type: typ, x.View: view})
		result := p.expr(e, x.Body)
		if typ.Escapes(p.ctx, result) {
			return nil
		}
		return result
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
		v := p.schema(nil, out)
		if v, ok := v.(*adt.Vertex); ok {
			// A constructed runtime record has exactly these fields, even
			// though their symbolic values describe many possible packets.
			out := v.ToDataSingle()
			out.ClosedNonRecursive = true
			return out
		}
		return v
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
	case *adt.UnaryExpr:
		v := p.expr(env, x.X)
		if v == nil {
			return nil
		}
		switch x.Op {
		case adt.NotOp:
			if v.Kind() == adt.BoolKind {
				if b, ok := adt.Unwrap(v).(*adt.Bool); ok {
					return &adt.Bool{B: !b.B}
				}
				return &adt.BasicType{K: adt.BoolKind}
			}
		case adt.AddOp, adt.SubtractOp:
			if v.Kind()&adt.NumberKind == v.Kind() {
				if x.Op == adt.AddOp {
					return v
				}
				if result := p.negateNumber(v); result != nil {
					return p.schema(nil, result)
				}
			}
		}
		return nil
	case *adt.BinaryExpr:
		a, b := p.expr(env, x.X), p.expr(env, x.Y)
		if a == nil || b == nil {
			return nil
		}
		ka, kb := a.Kind(), b.Kind()
		switch x.Op {
		case adt.AddOp, adt.SubtractOp, adt.MultiplyOp:
			if ka&adt.NumberKind == ka && kb&adt.NumberKind == kb {
				if x.Op == adt.AddOp || x.Op == adt.SubtractOp {
					if n, ok := adt.Unwrap(b).(*adt.Num); ok {
						return p.schema(nil, p.translateNumber(a, n, x.Op))
					}
					if n, ok := adt.Unwrap(a).(*adt.Num); ok && x.Op == adt.AddOp {
						return p.schema(nil, p.translateNumber(b, n, x.Op))
					}
				}
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

// Negation reverses ordered interval endpoints, while preserving numeric
// kinds, exclusions, and unions. This is a proof for the whole input domain.
func (p *certifier) negateNumber(v adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(v).(type) {
	case *adt.Num:
		return p.schema(nil, &adt.UnaryExpr{Op: adt.SubtractOp, X: x})
	case *adt.BoundValue:
		op := x.Op
		switch op {
		case adt.LessThanOp:
			op = adt.GreaterThanOp
		case adt.LessEqualOp:
			op = adt.GreaterEqualOp
		case adt.GreaterThanOp:
			op = adt.LessThanOp
		case adt.GreaterEqualOp:
			op = adt.LessEqualOp
		case adt.NotEqualOp:
		default:
			return &adt.BasicType{K: v.Kind()}
		}
		if n, ok := x.Value.(*adt.Num); ok {
			value := p.negateNumber(n)
			if value == nil {
				return nil
			}
			return &adt.BoundValue{Op: op, Value: value}
		}
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			v := p.negateNumber(term)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return out
	case *adt.Disjunction:
		out := &adt.Disjunction{}
		for _, term := range x.Values {
			v := p.negateNumber(term)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return out
	}
	return &adt.BasicType{K: v.Kind()}
}

// Translation by a constant preserves numeric interval predicates. This
// proves operations such as a nonnegative counter's successor without
// testing concrete examples or discarding its lower bound.
func (p *certifier) translateNumber(v adt.Value, n *adt.Num, op adt.Op) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(v).(type) {
	case *adt.Num:
		return adt.BinOp(p.ctx, nil, op, x, n)
	case *adt.BoundValue:
		switch x.Op {
		case adt.LessThanOp, adt.LessEqualOp, adt.GreaterThanOp, adt.GreaterEqualOp, adt.NotEqualOp:
			if b, ok := x.Value.(*adt.Num); ok {
				return &adt.BoundValue{Op: x.Op, Value: adt.BinOp(p.ctx, nil, op, b, n)}
			}
		}
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			value := p.translateNumber(term, n, op)
			if value == nil {
				return nil
			}
			out.Values = append(out.Values, value)
		}
		return out
	}
	return &adt.BasicType{K: v.Kind() | n.Kind()}
}

func (p *certifier) call(env *adt.Environment, call *adt.CallExpr) adt.Value {
	if call.Partial {
		return nil
	}
	callee := adt.Unwrap(p.expr(env, call.Fun))
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
	var source adt.FuncType
	switch f := callee.(type) {
	case *adt.FuncValue:
		if f.Src != nil && f.Src.Effect != nil {
			return nil
		}
		source = adt.FuncType{Fn: f.ResidualSignature(), Env: f.Env}
	case *adt.Builtin:
		// Primitive totality and packet coverage are separate obligations.
		// Use the same protocol as builtin capability inclusion, including
		// its label and omission rules.
		source.Fn = primitiveContract(p.ctx, f)
		if source.Fn == nil || ValidateBuiltin(p.ctx, f) != nil {
			return nil
		}
	default:
		return nil
	}
	sources := []adt.FuncType{source}
	if f, ok := callee.(*adt.FuncValue); ok {
		if p.hypotheses[f] {
			p.useHypothesis(f)
		} else if !p.implementation(f) {
			return nil
		}
		if boundary, ok := f.Fn.Body.(*adt.OpaqueCall); ok && !f.IsPartial() {
			sources = nil
			for _, alternative := range boundary.ProofAlternatives() {
				sources = append(sources, adt.FuncType{Fn: alternative.Fn, Env: alternative.Env})
			}
		}
	}
	s := &subsumer{ctx: p.ctx}
	for _, source := range sources {
		if len(adt.FunctionTypeParameters(source)) != 0 {
			var b *adt.Bottom
			source, b = adt.InstantiateFunctionType(p.ctx, source, target)
			if b != nil {
				continue
			}
		}
		noResult := *source.Fn
		noResult.Ret = nil
		if s.capabilitySignature(target, adt.FuncType{Fn: &noResult, Env: source.Env}) {
			return p.schema(source.Env, source.Fn.Ret)
		}
	}
	return nil
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
