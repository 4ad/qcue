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

// An occurrence test may inspect an optional slot without reading its absent
// value. Its identity includes the lexical binding: two optional parameters
// with the same type need not be present together.
func (p *certifier) presenceCondition(env *adt.Environment, expr adt.Expr) (checkedCondition, bool) {
	binary, ok := expr.(*adt.BinaryExpr)
	if !ok || (binary.Op != adt.EqualOp && binary.Op != adt.NotEqualOp) {
		return checkedCondition{}, false
	}
	operand, failure := binary.X, binary.Y
	if _, ok := operand.(*adt.Bottom); ok {
		operand, failure = failure, operand
	}
	bottom, ok := failure.(*adt.Bottom)
	if !ok || bottom.IsIncomplete() {
		return checkedCondition{}, false
	}
	ref, ok := operand.(*adt.FieldReference)
	if !ok {
		return checkedCondition{}, false
	}
	for range ref.UpCount {
		if env == nil {
			return checkedCondition{}, false
		}
		env = env.Up
	}
	scope := p.scopes[env]
	if scope == nil || scope.optional[ref.Label] == nil {
		return checkedCondition{}, false
	}
	var domain adt.Value = &adt.BasicType{K: adt.BoolKind}
	if scope.values[ref.Label] != nil {
		domain = &adt.Bool{B: true}
	} else if scope.absent[ref.Label] {
		domain = &adt.Bool{B: false}
	}
	c := checkedCondition{
		subject: conditionSubject{env: env, label: ref.Label, presence: true},
		domain:  domain, yes: &adt.Bool{B: true}, no: &adt.Bool{B: false},
		optional: scope.optional[ref.Label], presentOnTrue: binary.Op == adt.NotEqualOp,
	}
	if !c.presentOnTrue {
		c.yes, c.no = c.no, c.yes
	}
	return c, true
}

// Give a branch its own proof environments. Mutating a shared scope would
// leak presence to a sibling branch or a cached closure proof. New scopes also
// keep refinements alive in closures constructed inside the checked branch.
func (p *certifier) presenceBranch(env *adt.Environment, condition checkedCondition, yes bool) *adt.Environment {
	if condition.optional == nil {
		return env
	}
	replacements := make(map[*adt.Environment]*adt.Environment)
	var clone func(*adt.Environment) *adt.Environment
	clone = func(old *adt.Environment) *adt.Environment {
		if old == nil || !p.step() {
			return nil
		}
		copy := *old
		replacements[old] = &copy
		if old != condition.subject.env {
			copy.Up = clone(old.Up)
		}
		if scope := p.scopes[old]; scope != nil {
			s := *scope
			s.values, s.active = maps.Clone(scope.values), maps.Clone(scope.active)
			s.absent = maps.Clone(scope.absent)
			if old == condition.subject.env {
				label := condition.subject.label
				if yes == condition.presentOnTrue {
					s.values[label] = condition.optional
					delete(s.absent, label)
				} else {
					delete(s.values, label)
					if s.absent == nil {
						s.absent = make(map[adt.Feature]bool)
					}
					s.absent[label] = true
				}
			}
			p.scopes[&copy] = &s
		}
		return &copy
	}
	branch := clone(env)
	if replacements[condition.subject.env] == nil {
		return nil
	}
	for old, current := range replacements {
		if scope := p.scopes[old]; scope != nil && scope.bindings != nil {
			bindings := make(map[adt.Feature][]proofBinding, len(scope.bindings))
			for label, values := range scope.bindings {
				values = slices.Clone(values)
				for i := range values {
					if env := replacements[values[i].env]; env != nil {
						values[i].env = env
					}
				}
				bindings[label] = values
			}
			p.scopes[current].bindings = bindings
		}
	}
	return branch
}
