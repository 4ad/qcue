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
	"slices"

	"cuelang.org/go/internal/core/adt"
)

// Identity of an observed scalar, not equality of its type. Distinct integer
// parameters cannot share a guard merely because both have the type int.
type conditionSubject struct {
	env    *adt.Environment
	label  adt.Feature
	length bool
	other  adt.Expr
	// A comparison between two stable bindings supplies one Boolean
	// observation, even when neither operand has a concrete value.
	rightEnv    *adt.Environment
	rightLabel  adt.Feature
	rightLength bool
	comparison  adt.Op
}

type checkedCondition struct {
	subject         conditionSubject
	domain, yes, no adt.Value
}

type conditionalRecordState struct {
	fields map[adt.Feature]adt.Value
	region map[conditionSubject]adt.Value
	exact  bool
}

// Enumerate the finite presence branches, retaining correlations between
// guards on the same scalar. All conditions and bodies are checked before
// excluding an impossible region. A projected field must consequently be
// present in every surviving record shape.
func (p *certifier) conditionalRecord(env *adt.Environment, base *adt.StructLit, comps []*adt.Comprehension) adt.Value {
	initial := conditionalRecordState{
		fields: make(map[adt.Feature]adt.Value),
		region: make(map[conditionSubject]adt.Value),
		exact:  true,
	}
	for _, decl := range base.Decls {
		field := decl.(*adt.Field)
		initial.fields[field.Label] = field.Value.(adt.Value)
	}
	states := []conditionalRecordState{initial}
	for _, comp := range comps {
		// General for/let comprehension scopes need separate iteration
		// rules; a conditional record has one Boolean presence decision.
		if len(comp.Clauses) != 1 {
			return nil
		}
		clause, ok := comp.Clauses[0].(*adt.IfClause)
		if !ok {
			return nil
		}
		condition, ok := p.condition(env, clause.Condition)
		if !ok {
			return nil
		}
		body := p.expr(env, comp.Value)
		if body == nil {
			return nil
		}
		var fallback adt.Value
		if comp.Fallback != nil {
			fallback = p.expr(env, comp.Fallback)
			if fallback == nil {
				return nil
			}
		}
		var next []conditionalRecordState
		for _, state := range states {
			for i, guard := range []adt.Value{condition.yes, condition.no} {
				if !p.step() {
					return nil
				}
				prior := state.region[condition.subject]
				if prior == nil {
					prior = condition.domain
				}
				region := p.eagerMeet(prior, guard)
				if region == nil {
					return nil
				}
				if refuted(region) {
					continue
				}
				branch := state
				branch.region = maps.Clone(state.region)
				branch.region[condition.subject] = region
				value := body
				if i == 1 {
					value = fallback
				}
				if value == nil {
					next = append(next, branch)
					continue
				}
				alternatives, ok := p.addConditionalFields(branch, value)
				if !ok {
					return nil
				}
				next = append(next, alternatives...)
			}
		}
		states = next
	}
	var results []adt.Value
	for _, state := range states {
		out := &adt.StructLit{}
		for _, label := range slices.Sorted(maps.Keys(state.fields)) {
			out.Decls = append(out.Decls, &adt.Field{Label: label, Value: state.fields[label]})
		}
		value := p.schema(nil, out)
		if value == nil {
			return nil
		}
		if v, ok := value.(*adt.Vertex); ok && v.Bottom() == nil {
			if state.exact {
				v = v.ToDataSingle()
				v.ClosedNonRecursive = true
				p.constructors[v] = out
			}
			p.projections[v] = state.fields
			value = v
		}
		results = append(results, value)
	}
	if len(results) == 0 {
		return &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("no consistent conditional branch")}
	}
	return proofUnion(results)
}

func (p *certifier) addConditionalFields(state conditionalRecordState, value adt.Value) ([]conditionalRecordState, bool) {
	if !p.step() {
		return nil, false
	}
	if union, ok := adt.Unwrap(value).(*adt.Disjunction); ok {
		var out []conditionalRecordState
		for _, branch := range union.Values {
			states, ok := p.addConditionalFields(state, branch)
			if !ok {
				return nil, false
			}
			out = append(out, states...)
		}
		return out, true
	}
	record, ok := value.(*adt.Vertex)
	if !ok || record.Kind() != adt.StructKind {
		return nil, false
	}
	// An embedded input may contain undeclared extra fields. Its declared
	// inventory justifies selections but not an exact data-closed shape.
	state.exact = state.exact && p.constructors[record] != nil
	state.fields = maps.Clone(state.fields)
	for _, field := range record.Arcs {
		if field.Label.IsLet() || field.Label.IsDef() {
			continue
		}
		if field.ArcType != adt.ArcMember && field.ArcType != adt.ArcRequired {
			return nil, false
		}
		v := p.project(record, field.Label)
		if old := state.fields[field.Label]; old != nil {
			v = p.sourceMeet(old, v)
		}
		if v == nil {
			return nil, false
		}
		state.fields[field.Label] = v
	}
	return []conditionalRecordState{state}, true
}

func (p *certifier) condition(env *adt.Environment, expr adt.Expr) (checkedCondition, bool) {
	value := p.expr(env, expr)
	if value == nil || value.Kind() != adt.BoolKind {
		return checkedCondition{}, false
	}
	c := checkedCondition{subject: conditionSubject{env: env, other: expr},
		domain: value, yes: &adt.Bool{B: true}, no: &adt.Bool{B: false}}
	if unary, ok := expr.(*adt.UnaryExpr); ok && unary.Op == adt.NotOp {
		inner, ok := p.condition(env, unary.X)
		inner.yes, inner.no = inner.no, inner.yes
		return inner, ok
	}
	if subject, domain, ok := p.conditionScalar(env, expr); ok {
		c.subject, c.domain = subject, domain
		return c, true
	}
	binary, ok := expr.(*adt.BinaryExpr)
	if !ok {
		return c, true
	}
	subject, domain, ok := p.conditionScalar(env, binary.X)
	literal := p.expr(env, binary.Y)
	if !ok || literal == nil || !adt.IsConcrete(literal) {
		if right, _, rightOK := p.conditionScalar(env, binary.Y); ok && rightOK {
			op := binary.Op
			negated := false
			switch op {
			case adt.GreaterEqualOp:
				op, negated = adt.LessThanOp, true
			case adt.GreaterThanOp:
				op, negated = adt.LessEqualOp, true
			case adt.NotEqualOp:
				op, negated = adt.EqualOp, true
			case adt.LessThanOp, adt.LessEqualOp, adt.EqualOp:
			default:
				return c, true
			}
			subject.rightEnv, subject.rightLabel, subject.rightLength = right.env, right.label, right.length
			subject.comparison = op
			c.subject = subject
			if negated {
				c.yes, c.no = c.no, c.yes
			}
		}
		return c, true
	}
	var yes, no adt.Value
	bound := func(op adt.Op) adt.Value { return &adt.BoundValue{Op: op, Value: literal} }
	switch binary.Op {
	case adt.EqualOp, adt.NotEqualOp:
		yes, no = literal, bound(adt.NotEqualOp)
		if literal.Kind()&^adt.NumberKind == 0 {
			yes = &adt.Conjunction{Values: []adt.Value{bound(adt.GreaterEqualOp), bound(adt.LessEqualOp)}}
			// Keep the two open regions explicit. The ordinary eager
			// service need not reduce >=n & <=n & !=n by itself.
			no = proofUnion([]adt.Value{bound(adt.LessThanOp), bound(adt.GreaterThanOp)})
		}
		if binary.Op == adt.NotEqualOp {
			yes, no = no, yes
		}
	case adt.LessThanOp:
		yes, no = bound(adt.LessThanOp), bound(adt.GreaterEqualOp)
	case adt.LessEqualOp:
		yes, no = bound(adt.LessEqualOp), bound(adt.GreaterThanOp)
	case adt.GreaterThanOp:
		yes, no = bound(adt.GreaterThanOp), bound(adt.LessEqualOp)
	case adt.GreaterEqualOp:
		yes, no = bound(adt.GreaterEqualOp), bound(adt.LessThanOp)
	default:
		return c, true
	}
	c.subject, c.domain, c.yes, c.no = subject, domain, yes, no
	return c, true
}

// Recognize stable local scalar bindings and the length of a stable binding.
// Unsupported expressions remain independent Boolean conditions; they can
// still use an explicit else, but cannot establish cross-condition coverage.
func (p *certifier) conditionScalar(env *adt.Environment, expr adt.Expr) (conditionSubject, adt.Value, bool) {
	key := conditionSubject{}
	value := p.expr(env, expr)
	if call, ok := expr.(*adt.CallExpr); ok {
		builtin, ok := adt.Unwrap(p.expr(env, call.Fun)).(*adt.Builtin)
		if !ok || builtin.Package != adt.InvalidLabel || builtin.Name != "len" || len(call.Args) != 1 {
			return key, nil, false
		}
		key.length = true
		expr = call.Args[0]
	}
	ref, ok := expr.(*adt.FieldReference)
	if !ok || value == nil {
		return key, nil, false
	}
	for range ref.UpCount {
		if env == nil {
			return key, nil, false
		}
		env = env.Up
	}
	key.env, key.label = env, ref.Label
	return key, value, true
}
