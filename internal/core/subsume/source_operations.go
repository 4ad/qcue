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
	"cuelang.org/go/internal/core/walk"
)

// ValidateSourceOperations preserves static call obligations when a data
// projection discards part of its source constructor. Ordinary CUE data
// constraints keep their existing semantics; quantified calls require the
// same certificates as observed applications.
func ValidateSourceOperations(ctx *adt.OpContext, env *adt.Environment, expr adt.Expr) *adt.Bottom {
	p := newCertifier(ctx)
	defer p.enter()()
	if p.sourceOperations(env, expr) {
		return nil
	}
	if p.failure != nil {
		return p.failure
	}
	return &adt.Bottom{Src: expr.Source(), Code: adt.BlockedError,
		Err: ctx.NewPosf(adt.Pos(expr), "source operation remains unproved")}
}

func (p *certifier) sourceOperations(env *adt.Environment, expr adt.Expr) bool {
	ok := true
	seen := make(map[adt.Node]bool)
	visitor := walk.Visitor{}
	visitor.Before = func(node adt.Node) bool {
		if node == nil || seen[node] || !ok {
			return false
		}
		seen[node] = true
		if !p.step() {
			ok = false
			return false
		}
		switch x := node.(type) {
		case *adt.BinaryExpr:
			if x.Op == adt.AndOp {
				if record := p.prepareRecord([]proofBinding{{env, x}}); record != nil {
					ok = p.sourceRecordOperations(record)
					return false
				}
			}
		case *adt.StructLit:
			if record := p.prepareRecord([]proofBinding{{env, x}}); record != nil {
				ok = p.sourceRecordOperations(record)
				return false
			}
			scope := p.frame(env, make(map[adt.Feature]adt.Value))
			p.scopes[scope].fields = make(map[adt.Feature]adt.Expr)
			for _, decl := range x.Decls {
				switch f := decl.(type) {
				case *adt.Field:
					p.scopes[scope].fields[f.Label] = f.Value
				case *adt.LetField:
					p.scopes[scope].fields[f.Label] = f.Value
				}
			}
			for _, decl := range x.Decls {
				var value adt.Expr
				switch f := decl.(type) {
				case *adt.Field:
					value = f.Value
				case *adt.LetField:
					value = f.Value
				case adt.Expr:
					value = f
				case *adt.BulkOptionalField:
					ok = p.sourceOperations(scope, f.Filter)
					value = f.Value
				case *adt.DynamicField:
					ok = p.sourceOperations(scope, f.Key)
					value = f.Value
				case *adt.Ellipsis:
					value = f.Value
				case *adt.Comprehension:
					ok = p.sourceComprehension(scope, f)
				}
				if !ok || value != nil && !p.sourceOperations(scope, value) {
					ok = false
					break
				}
			}
			return false
		case *adt.ListLit:
			scope := p.frame(env, make(map[adt.Feature]adt.Value))
			for _, elem := range x.Elems {
				if comp, found := elem.(*adt.Comprehension); found {
					ok = p.sourceComprehension(scope, comp)
				} else if e, found := elem.(adt.Expr); found {
					ok = p.sourceOperations(scope, e)
				} else if e, found := elem.(*adt.Ellipsis); found && e.Value != nil {
					ok = p.sourceOperations(scope, e.Value)
				}
				if !ok {
					break
				}
			}
			return false
		case *adt.Function:
			// Bodies have their own parameter and capture scopes. Definition
			// checking certifies them after their enclosing record refinements
			// have been accumulated, even when no call observes them.
			return false
		case *adt.Quantified:
			// Finite quantified data uses the finite expansion checker, not
			// an arbitrary rigid type variable as a value witness.
			for _, param := range x.Params {
				if param.ValueRange != nil {
					return false
				}
			}
			if _, function := x.Body.(*adt.Function); function {
				return false
			}
			ok = p.typeOperations(env, x)
			return false
		case *adt.AliasApplication:
			if record := p.prepareRecord([]proofBinding{{env, x}}); record != nil {
				ok = p.sourceRecordOperations(record)
				return false
			}
			ok = p.typeOperations(env, x)
			return false
		case *adt.CallExpr:
			callee := p.expr(env, x.Fun)
			if callee == nil {
				ok = false
				return false
			}
			switch f := adt.Unwrap(callee).(type) {
			case *adt.FuncValue:
				if f.Fn.Quantified {
					ok = p.apply(env, f, x) != nil
					return false
				}
			case *adt.Builtin:
				for _, clause := range f.AdditionalTypes() {
					if clause.Fn.Quantified {
						ok = p.apply(env, f, x) != nil
						return false
					}
				}
			}
		}
		return true
	}
	visitor.Elem(expr)
	return ok
}

// Calls see all refinements of their record's fields, including contracts
// and implementations supplied in separate record conjuncts.
func (p *certifier) sourceRecordOperations(record *proofRecord) bool {
	for _, label := range record.labels {
		bindings := record.scope.bindings[label]
		for _, binding := range bindings {
			if !p.sourceOperations(binding.env, binding.expr) {
				return false
			}
		}
	}
	return true
}

// Data-only comprehensions retain ordinary CUE evaluation. A comprehension
// containing a user call needs its iteration scopes checked before projection.
func (p *certifier) sourceComprehension(env *adt.Environment, comp *adt.Comprehension) bool {
	calls := false
	visitor := walk.Visitor{Before: func(node adt.Node) bool {
		if calls || node == nil {
			return false
		}
		if !p.step() {
			calls = true
			return false
		}
		switch x := node.(type) {
		case *adt.Function:
			return false
		case *adt.CallExpr:
			switch fun := x.Fun.(type) {
			case *adt.Builtin:
				return true
			case *adt.SelectorExpr:
				if _, imported := fun.X.(*adt.ImportReference); imported {
					if b, ok := adt.Unwrap(p.schema(env, fun)).(*adt.Builtin); ok && len(b.AdditionalTypes()) == 0 {
						return true
					}
				}
			}
			calls = true
			return false
		}
		return true
	}}
	visitor.Elem(comp)
	if !calls {
		return true
	}
	for _, clause := range comp.Clauses {
		if condition, ok := clause.(*adt.IfClause); ok {
			if !p.sourceOperations(env, condition.Condition) {
				return false
			}
			continue
		}
		// Iteration introduces bindings. Reuse its strict constructor rule
		// rather than checking the body in an unrelated lexical scope. An
		// empty iteration cannot substitute for a derivation of its body.
		value, _ := p.comprehension(env, comp)
		return value != nil
	}
	return p.sourceOperations(env, comp.Value) &&
		(comp.Fallback == nil || p.sourceOperations(env, comp.Fallback))
}
