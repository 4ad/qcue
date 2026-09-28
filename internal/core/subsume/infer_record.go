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
	open   bool
	// closed records need their source structure replayed for closedness.
	closed     bool
	frames     map[proofBinding]*adt.Environment
	references []proofBinding
	operations []proofBinding
}

// Collect record conjuncts before checking methods that capture their fields.
// Every field expression keeps its own lexical scope, while references to the
// unified subject see all its declarations. Only source fields contribute:
// result annotations cannot supply a missing capture or its type.
func (p *inference) recordMeet(bindings []proofBinding) (adt.Value, bool) {
	record := p.prepareRecord(bindings)
	if record == nil {
		return nil, false
	}
	out := &adt.StructLit{}
	for _, label := range record.labels {
		var value adt.Value
		mode := record.scope.presence[label]
		if mode == adt.ArcOptional {
			// An optional declaration constrains a possible field without
			// making it available to an unguarded reference.
			value = p.bindingMeet(record.scope.bindings[label])
		} else {
			value = p.expr(record.env, &adt.FieldReference{Label: label})
		}
		if value == nil {
			return nil, true
		}
		if !label.IsLet() {
			out.Decls = append(out.Decls, &adt.Field{Label: label, ArcType: mode, Value: value})
		}
	}
	exact := !record.open
	for _, mode := range record.scope.presence {
		exact = exact && mode == adt.ArcMember
	}
	value := p.schema(nil, out)
	if vertex, ok := value.(*adt.Vertex); ok && vertex.Bottom() == nil {
		p.projections[vertex] = record.scope.values
		if record.closed {
			shape := p.recordShape(record, bindings, make(map[proofBinding]bool))
			if shape == nil {
				return nil, true
			}
			shaped := &adt.Vertex{}
			shaped.AddConjunct(adt.MakeRootConjunct(nil, out))
			shaped.AddConjunct(adt.MakeRootConjunct(nil, shape))
			shaped.Finalize(p.ctx)
			if refuted(shaped) {
				return shaped, true
			}
			p.recordEvidence(vertex, shaped)
			vertex = shaped
		}
		if exact {
			source := vertex
			vertex = vertex.ToDataSingle()
			vertex.ClosedNonRecursive = true
			if !record.closed {
				p.constructors[vertex] = out
			} else {
				p.constructors[vertex] = source
			}
			p.projections[vertex] = p.projections[source]
		}
		value = vertex
	}
	for _, ref := range record.references {
		p.memberships[value] = append(p.memberships[value], &adt.LiveType{
			Ref: &adt.LiveReference{X: ref.expr}, Env: ref.env, Upper: value,
		})
	}
	return value, true
}

// Preparing a record installs lexical field bindings without checking or
// evaluating their terms. Reuse that context when locating the annotated
// implementation behind a projection, so recursive references can tie the
// same function knot as a direct function declaration.
func (p *inference) prepareRecord(bindings []proofBinding) *proofRecord {
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
	record := &proofRecord{frames: make(map[proofBinding]*adt.Environment), scope: &proofScope{
		values:   make(map[adt.Feature]adt.Value),
		fields:   make(map[adt.Feature]adt.Expr),
		bindings: make(map[adt.Feature][]proofBinding),
		presence: make(map[adt.Feature]adt.ArcType),
		active:   make(map[adt.Feature]bool),
	}}
	active := make(map[proofBinding]bool)
	for _, binding := range bindings {
		if !p.collectRecord(record, binding, active, true) {
			return nil
		}
	}
	p.records[key] = record
	return record
}

func (p *inference) functionBinding(binding proofBinding, seen map[proofBinding]bool) *adt.FuncValue {
	if seen[binding] || !p.step() {
		return nil
	}
	seen[binding] = true
	switch x := binding.expr.(type) {
	case *adt.Function:
		if x.Body != nil {
			value, _ := adt.Unwrap(p.schema(binding.env, x)).(*adt.FuncValue)
			return value
		}
	case *adt.Quantified:
		if f, ok := x.Body.(*adt.Function); ok && f.Body != nil {
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

func (p *inference) recordBinding(binding proofBinding, seen map[proofBinding]bool) *proofRecord {
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

func (p *inference) referenceBindings(env *adt.Environment, ref *adt.FieldReference) []proofBinding {
	for range ref.UpCount {
		if env == nil {
			return nil
		}
		env = env.Up
	}
	if scope := p.scopes[env]; scope != nil {
		if scope.presence[ref.Label] == adt.ArcOptional {
			return nil
		}
		if bindings := scope.bindings[ref.Label]; bindings != nil {
			return bindings
		}
		if expr := scope.fields[ref.Label]; expr != nil {
			return []proofBinding{{env, expr}}
		}
		return nil
	}
	if env == nil {
		return nil
	}
	// Collection reads source declarations, not their evaluated values.
	// Finalizing here could freeze a template's incomplete computation
	// before the enclosing meet has supplied its refinements.
	parent := env.DerefVertex(p.ctx).DerefValue()
	field := parent.LookupRaw(ref.Label)
	if graph := p.ctx.Propagation; graph != nil {
		graph.Observe(parent)
		graph.Observe(field)
	}
	if field == nil || field.ArcType == adt.ArcOptional {
		return nil
	}
	var bindings []proofBinding
	for conjunct := range field.LeafConjuncts() {
		env, expr := conjunct.EnvExpr()
		bindings = append(bindings, proofBinding{env, expr})
	}
	return bindings
}

func (p *inference) bindingMeet(bindings []proofBinding) adt.Value {
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

// collectRecord establishes one shared field scope before any body is
// checked. Each constructor retains its own lexical parent; only references
// internal to the copied record are rebound. Supplied parameter values have
// no source bindings to expand and keep their independent value proofs.
func (p *inference) collectRecord(record *proofRecord, binding proofBinding, active map[proofBinding]bool, observe bool) bool {
	if !p.step() || active[binding] {
		return false
	}
	active[binding] = true
	defer delete(active, binding)
	collect := func(env *adt.Environment, expr adt.Expr) bool {
		return p.collectRecord(record, proofBinding{env, expr}, active, observe)
	}
	switch x := binding.expr.(type) {
	case *adt.BinaryExpr:
		return x.Op == adt.AndOp && collect(binding.env, x.X) && collect(binding.env, x.Y)
	case *adt.FieldReference:
		fields := p.referenceBindings(binding.env, x)
		if len(fields) == 0 {
			return false
		}
		for _, field := range fields {
			if !p.collectRecord(record, field, active, false) {
				return false
			}
		}
		record.closed = record.closed || x.Label.IsDef()
		// A named description can acquire fields under future refinement.
		// Its current declarations do not establish an exact inventory.
		record.open = true
		record.references = append(record.references, binding)
		return true
	case *adt.SelectorExpr:
		base := p.recordBinding(proofBinding{binding.env, x.X}, make(map[proofBinding]bool))
		if base == nil || base.scope.presence[x.Sel] == adt.ArcOptional {
			return false
		}
		record.closed = record.closed || base.closed || x.Sel.IsDef()
		// Binding discovery uses canonical scopes to tie recursive methods.
		// The access path separately retains definition boundaries and the
		// openness of named descriptions crossed on the way to that scope.
		if path := p.prepareRecord([]proofBinding{{binding.env, x.X}}); path != nil {
			record.closed = record.closed || path.closed
			record.open = record.open || path.open
		}
		if observe {
			record.operations = append(record.operations, base.operations...)
		}
		fields := base.scope.bindings[x.Sel]
		if len(fields) == 0 {
			return false
		}
		for _, field := range fields {
			if !p.collectRecord(record, field, active, false) {
				return false
			}
		}
		return true
	case *adt.CallExpr:
		builtin, ok := x.Fun.(*adt.Builtin)
		if !ok || builtin.Package != adt.InvalidLabel || builtin.Name != "close" || len(x.Args) != 1 {
			return false
		}
		record.closed = true
		return collect(binding.env, x.Args[0])
	case *adt.LetReference:
		env := binding.env
		for range x.UpCount {
			if env == nil {
				return false
			}
			env = env.Up
		}
		return collect(env, x.X)
	case *adt.AliasApplication:
		args := make([]adt.Value, len(x.Args))
		for i, arg := range x.Args {
			if !p.typeOperations(binding.env, arg) {
				return false
			}
			args[i] = p.schema(binding.env, arg)
			if args[i] == nil {
				return false
			}
		}
		env, b := x.Expand(p.ctx, binding.env, args)
		return b == nil && collect(env, x.Template.Body)
	case *adt.StructLit:
		scope := record.scope
		env := p.frame(binding.env, scope.values)
		p.scopes[env] = scope
		record.frames[binding] = env
		if record.env == nil {
			record.env = env
		}
		// Install all direct declarations before expanding embeddings, which
		// can themselves refer to these declarations.
		for _, decl := range x.Decls {
			var label adt.Feature
			var expr adt.Expr
			mode := adt.ArcMember
			switch field := decl.(type) {
			case *adt.Field:
				label, expr, mode = field.Label, field.Value, field.ArcType
			case *adt.LetField:
				label, expr = field.Label, field.Value
			case adt.Expr:
				continue
			case *adt.Ellipsis:
				if field.Value != nil {
					return false
				}
				record.open = true
				continue
			default:
				return false
			}
			if old, exists := scope.presence[label]; exists {
				mode = min(old, mode)
			} else {
				record.labels = append(record.labels, label)
				scope.fields[label] = expr
			}
			scope.presence[label] = mode
			scope.bindings[label] = append(scope.bindings[label], proofBinding{env, expr})
			if observe {
				record.operations = append(record.operations, proofBinding{env, expr})
			}
		}
		for _, decl := range x.Decls {
			if embedded, ok := decl.(adt.Expr); ok && !collect(env, embedded) {
				return false
			}
		}
		return true
	}
	return false
}
