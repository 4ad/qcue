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

import "cuelang.org/go/internal/core/adt"

type proofRecordKey struct {
	// A source expression has one checking context in a lexical scope.
	binding proofBinding
	// A field's conjunct slice is immutable and names a collected subject.
	join *proofBinding
}

type proofRecord struct {
	env    *adt.Environment
	scope  *proofScope
	labels []adt.Feature
}

// Collect record conjuncts before checking methods that capture their fields.
// Every field expression keeps its own lexical scope, while references to the
// unified subject see all its declarations. Only source fields contribute:
// result annotations cannot supply a missing capture or its type.
func (p *certifier) recordMeet(bindings []proofBinding) (adt.Value, bool) {
	record := p.prepareRecord(bindings)
	if record == nil {
		return nil, false
	}
	out := &adt.StructLit{}
	for _, label := range record.labels {
		value := p.expr(record.env, &adt.FieldReference{Label: label})
		if value == nil {
			return nil, true
		}
		if !label.IsLet() {
			out.Decls = append(out.Decls, &adt.Field{Label: label, Value: value})
		}
	}
	value := p.schema(nil, out)
	if vertex, ok := value.(*adt.Vertex); ok && vertex.Bottom() == nil {
		vertex = vertex.ToDataSingle()
		vertex.ClosedNonRecursive = true
		p.projections[vertex] = record.scope.values
		p.constructors[vertex] = out
		value = vertex
	}
	return value, true
}

// Preparing a record installs lexical field bindings without checking or
// evaluating their terms. Reuse that context when locating the annotated
// implementation behind a projection, so recursive references can tie the
// same function knot as a direct function declaration.
func (p *certifier) prepareRecord(bindings []proofBinding) *proofRecord {
	if len(bindings) == 0 || !p.step() {
		return nil
	}
	key := proofRecordKey{binding: bindings[0]}
	if len(bindings) > 1 {
		key = proofRecordKey{join: &bindings[0]}
	}
	if record := p.records[key]; record != nil {
		return record
	}
	var records []proofBinding
	for _, binding := range bindings {
		parts, ok := p.recordParts(binding)
		if !ok {
			return nil
		}
		records = append(records, parts...)
	}
	scope := &proofScope{
		values:   make(map[adt.Feature]adt.Value),
		fields:   make(map[adt.Feature]adt.Expr),
		bindings: make(map[adt.Feature][]proofBinding),
		active:   make(map[adt.Feature]bool),
	}
	var labels []adt.Feature
	var first *adt.Environment
	for _, record := range records {
		env := p.frame(record.env, scope.values)
		p.scopes[env] = scope
		if first == nil {
			first = env
		}
		for _, decl := range record.expr.(*adt.StructLit).Decls {
			var label adt.Feature
			var expr adt.Expr
			switch field := decl.(type) {
			case *adt.Field:
				label, expr = field.Label, field.Value
			case *adt.LetField:
				label, expr = field.Label, field.Value
			}
			if scope.fields[label] == nil {
				labels = append(labels, label)
				scope.fields[label] = expr
			}
			scope.bindings[label] = append(scope.bindings[label], proofBinding{env, expr})
		}
	}
	record := &proofRecord{env: first, scope: scope, labels: labels}
	p.records[key] = record
	return record
}

func (p *certifier) functionBinding(binding proofBinding, seen map[proofBinding]bool) *adt.FuncValue {
	if seen[binding] || !p.step() {
		return nil
	}
	seen[binding] = true
	switch x := binding.expr.(type) {
	case *adt.Function:
		if x.Body != nil {
			return &adt.FuncValue{Fn: x, Src: x.Src, Env: binding.env}
		}
	case *adt.Quantified:
		if f, ok := x.Body.(*adt.Function); ok && f.Body != nil && !x.Src.Exists {
			value, _ := adt.Unwrap(p.schema(binding.env, x)).(*adt.FuncValue)
			return value
		}
	case *adt.SelectorExpr:
		if record := p.recordBinding(proofBinding{binding.env, x.X}, seen); record != nil {
			fields := record.scope.bindings[x.Sel]
			if len(fields) == 1 {
				return p.functionBinding(fields[0], seen)
			}
		}
	case *adt.FieldReference:
		if fields := p.referenceBindings(binding.env, x); len(fields) == 1 {
			return p.functionBinding(fields[0], seen)
		}
	}
	return nil
}

func (p *certifier) recordBinding(binding proofBinding, seen map[proofBinding]bool) *proofRecord {
	if seen[binding] || !p.step() {
		return nil
	}
	seen[binding] = true
	switch x := binding.expr.(type) {
	case *adt.FieldReference:
		fields := p.referenceBindings(binding.env, x)
		if len(fields) == 1 {
			return p.recordBinding(fields[0], seen)
		}
		return p.prepareRecord(fields)
	case *adt.SelectorExpr:
		if record := p.recordBinding(proofBinding{binding.env, x.X}, seen); record != nil {
			fields := record.scope.bindings[x.Sel]
			if len(fields) == 1 {
				return p.recordBinding(fields[0], seen)
			}
			return p.prepareRecord(fields)
		}
		return nil
	}
	return p.prepareRecord([]proofBinding{binding})
}

func (p *certifier) referenceBindings(env *adt.Environment, ref *adt.FieldReference) []proofBinding {
	for range ref.UpCount {
		if env == nil {
			return nil
		}
		env = env.Up
	}
	if scope := p.scopes[env]; scope != nil {
		if bindings := scope.bindings[ref.Label]; bindings != nil {
			return bindings
		}
		if expr := scope.fields[ref.Label]; expr != nil {
			return []proofBinding{{env, expr}}
		}
	}
	return nil
}

func (p *certifier) bindingMeet(bindings []proofBinding) adt.Value {
	if value, handled := p.recordMeet(bindings); handled {
		return value
	}
	var value adt.Value
	for _, binding := range bindings {
		x := p.expr(binding.env, binding.expr)
		if x == nil {
			return nil
		}
		if value == nil {
			value = x
		} else {
			value = p.sourceMeet(value, x)
			if value == nil {
				return nil
			}
		}
	}
	return value
}

// This collection rule covers fixed record constructors and abbreviations.
// Other operands keep their independent expression and source-meet rules.
func (p *certifier) recordParts(binding proofBinding) ([]proofBinding, bool) {
	if !p.step() {
		return nil, false
	}
	switch x := binding.expr.(type) {
	case *adt.BinaryExpr:
		if x.Op != adt.AndOp {
			return nil, false
		}
		a, ok := p.recordParts(proofBinding{binding.env, x.X})
		if !ok {
			return nil, false
		}
		b, ok := p.recordParts(proofBinding{binding.env, x.Y})
		return append(a, b...), ok
	case *adt.AliasApplication:
		args := make([]adt.Value, len(x.Args))
		for i, arg := range x.Args {
			if !p.typeOperations(binding.env, arg) {
				return nil, false
			}
			args[i] = p.schema(binding.env, arg)
			if args[i] == nil {
				return nil, false
			}
		}
		env, b := x.Expand(p.ctx, binding.env, args)
		if b != nil {
			return nil, false
		}
		return p.recordParts(proofBinding{env, x.Template.Body})
	case *adt.StructLit:
		for _, decl := range x.Decls {
			switch field := decl.(type) {
			case *adt.Field:
				if field.ArcType != adt.ArcMember {
					return nil, false
				}
			case *adt.LetField:
			default:
				return nil, false
			}
		}
		return []proofBinding{binding}, true
	}
	return nil, false
}
