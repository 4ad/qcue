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

// CheckDeclarationRelevance opens the universal clauses of one source root
// rigidly and adds instances justified by independently stated sibling domains.
// It never chooses an instance merely to make a generic result empty.
func CheckDeclarationRelevance(ctx *adt.OpContext, clauses []adt.FuncType) *RelevanceError {
	p := newCertifier(ctx)
	defer p.enter()()
	r := &relevanceChecker{p: p}
	prepared, err := r.declarations(clauses)
	if err != nil {
		return err
	}
	return r.arrows(prepared)
}

type relevanceDeclaration struct {
	source adt.FuncType
	opened adt.FuncType
	params []*adt.TypeParameter
	rigid  []*adt.RigidType
	domain []adt.Value
	used   map[*adt.TypeParameter]bool
	anchor bool
}

func (r *relevanceChecker) declarations(clauses []adt.FuncType) ([]adt.FuncType, *RelevanceError) {
	var declarations []relevanceDeclaration
	var out []adt.FuncType
	for _, source := range clauses {
		d := relevanceDeclaration{source: source, opened: source,
			params: adt.FunctionTypeParameters(source), anchor: true}
		owned := make(map[*adt.TypeParameter]bool)
		for _, param := range d.params {
			owned[param] = true
			bound := r.p.schema(adt.TypeParameterScope(d.opened.Env, param), param.Bound)
			if bound == nil {
				return nil, r.blocked("universal bound remains unresolved")
			}
			if refuted(bound) && !r.p.typeOperations(adt.TypeParameterScope(d.opened.Env, param), param.Bound) {
				return nil, r.blocked("ill-typed operation in universal bound")
			}
			rigid := &adt.RigidType{Param: param, Bound: bound}
			d.rigid = append(d.rigid, rigid)
			d.opened = adt.BindFunctionTypes(d.opened, []adt.Value{rigid})
		}
		d.used = make(map[*adt.TypeParameter]bool)
		for _, param := range source.Fn.Params {
			value := r.p.schema(d.opened.Env, param.Value)
			if value == nil {
				return nil, r.blocked("packet predicate remains unresolved")
			}
			if refuted(value) && !r.p.typeOperations(d.opened.Env, param.Value) {
				return nil, r.blocked("ill-typed operation in packet predicate")
			}
			if !r.dependencies(value, owned, d.used, make(map[adt.Value]bool)) {
				return nil, r.blocked("packet dependencies remain unresolved")
			}
			d.domain = append(d.domain, value)
		}
		d.anchor = len(d.used) == 0
		result := r.p.schema(d.opened.Env, source.Fn.Ret)
		if refuted(result) && !r.p.typeOperations(d.opened.Env, source.Fn.Ret) {
			return nil, r.blocked("ill-typed operation in result predicate")
		}
		if !r.dependencies(result, owned, d.used, make(map[adt.Value]bool)) {
			return nil, r.blocked("result dependencies remain unresolved")
		}
		declarations = append(declarations, d)
		out = append(out, d.opened)
	}
	for i, d := range declarations {
		if len(d.used) == 0 {
			continue
		}
		for j, anchor := range declarations {
			if i == j || !anchor.anchor {
				continue
			}
			if !r.p.step() {
				return nil, r.blocked("")
			}
			bindings := make(map[*adt.RigidType]adt.Value)
			owned := make(map[*adt.RigidType]bool)
			for _, v := range d.rigid {
				owned[v] = true
			}
			matches := adt.MatchFuncValueParams(anchor.opened.Fn, &adt.FuncValue{Fn: d.opened.Fn})
			matched := len(matches) == len(d.domain)
			for ai, di := range matches {
				if di < 0 || !r.matchDomain(d.domain[di], anchor.domain[ai], owned, bindings) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			var args []adt.Value
			for k, param := range d.params {
				value := bindings[d.rigid[k]]
				if value == nil {
					if d.used[param] {
						matched = false
						break
					}
					value = d.rigid[k] // A vacuous binder requires no guessed instance.
				}
				args = append(args, value)
			}
			if !matched {
				continue
			}
			instance := adt.BindFunctionTypes(d.source, args)
			for k, param := range d.params {
				if !r.p.step() || !r.p.includes(r.p.schema(instance.Env, param.Bound), args[k]) {
					matched = false
					break
				}
			}
			if matched {
				// A structural substitution must actually admit the anchor's
				// full packet domain, including labels and presence modes.
				fn := *instance.Fn
				fn.Ret = nil
				packet := *anchor.opened.Fn
				packet.Ret = nil
				s := &subsumer{ctx: r.p.ctx}
				matched = s.capabilitySignature(adt.FuncType{Fn: &packet, Env: anchor.opened.Env},
					adt.FuncType{Fn: &fn, Env: instance.Env})
			}
			if matched {
				out = append(out, instance)
			}
		}
	}
	if !r.p.step() {
		return nil, r.blocked("")
	}
	return out, nil
}

// Collect dependencies after alias expansion, so a vacuous alias argument
// cannot make an independent sibling domain appear generic. Nested universal
// binders are opened rigidly; only this declaration's owned variables count.
func (r *relevanceChecker) dependencies(value adt.Value, owned, used map[*adt.TypeParameter]bool, seen map[adt.Value]bool) bool {
	if value == nil || !r.p.step() {
		return false
	}
	if seen[value] {
		return true
	}
	seen[value] = true
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType:
		if owned[x.Param] {
			used[x.Param] = true
		}
		if x.Bound != nil {
			return r.dependencies(x.Bound, owned, used, seen)
		}
	case *adt.Vertex:
		for _, field := range x.Arcs {
			if !r.dependencies(field, owned, used, seen) {
				return false
			}
		}
		if x.PatternConstraints != nil {
			for _, pair := range x.PatternConstraints.Pairs {
				pair.Constraint.Finalize(r.p.ctx)
				if !r.dependencies(pair.Pattern, owned, used, seen) || !r.dependencies(pair.Constraint, owned, used, seen) {
					return false
				}
			}
		}
	case *adt.Conjunction:
		for _, term := range x.Values {
			if !r.dependencies(term, owned, used, seen) {
				return false
			}
		}
	case *adt.Disjunction:
		for _, branch := range x.Values {
			if !r.dependencies(branch, owned, used, seen) {
				return false
			}
		}
	case *adt.FuncValue:
		for _, clause := range x.Obligations() {
			for _, param := range adt.FunctionTypeParameters(clause) {
				bound := r.p.schema(clause.Env, param.Bound)
				if bound == nil {
					return false
				}
				clause = adt.BindFunctionTypes(clause, []adt.Value{&adt.RigidType{Param: param, Bound: bound}})
			}
			for _, param := range clause.Fn.Params {
				if !r.dependencies(r.p.schema(clause.Env, param.Value), owned, used, seen) {
					return false
				}
			}
			if !r.dependencies(r.p.schema(clause.Env, clause.Fn.Ret), owned, used, seen) {
				return false
			}
		}
	case *adt.LiveType:
		return r.dependencies(x.Upper, owned, used, seen)
	case *adt.Universal:
		// These retained quantifiers are outside structural matching, but
		// their free source dependencies still prevent false anchors.
		var visitor walk.Visitor
		visited := make(map[adt.Node]bool)
		ok := true
		visitor.Before = func(n adt.Node) bool {
			if n == nil || visited[n] || !ok {
				return false
			}
			visited[n] = true
			if !r.p.step() {
				ok = false
				return false
			}
			switch y := n.(type) {
			case *adt.TypeReference:
				if owned[y.Param] {
					used[y.Param] = true
				}
			case *adt.LetReference:
				visitor.Elem(y.X)
				return false
			case *adt.Universal:
				visitor.Elem(y.Template)
				return false

			}
			return true
		}
		visitor.Elem(x)
		return ok
	}
	return true
}

// Structural matching is finite and invariant in each repeated variable.
// Width at a record pattern is permitted, but no missing field is invented.
// A failed match contributes no instance; it is not a refutation of either
// the universal declaration or the sibling's domain.
func (r *relevanceChecker) matchDomain(pattern, anchor adt.Value, owned map[*adt.RigidType]bool, bindings map[*adt.RigidType]adt.Value) bool {
	if !r.p.step() || pattern == nil || anchor == nil {
		return false
	}
	pattern, anchor = adt.Unwrap(pattern), adt.Unwrap(anchor)
	if variable, ok := pattern.(*adt.RigidType); ok && owned[variable] {
		if old := bindings[variable]; old != nil {
			return r.p.includes(old, anchor) && r.p.includes(anchor, old)
		}
		bindings[variable] = anchor
		return true
	}
	switch pattern := pattern.(type) {
	case *adt.Vertex:
		other, ok := anchor.(*adt.Vertex)
		if !ok || pattern.Kind() != other.Kind() {
			return false
		}
		if pattern.IsList() {
			if pattern.IsClosedList() != other.IsClosedList() || len(pattern.Arcs) != len(other.Arcs) {
				return false
			}
		}
		for _, field := range pattern.Arcs {
			if field.Label.IsLet() {
				continue
			}
			actual := other.LookupRaw(field.Label)
			if actual == nil || field.ArcType != actual.ArcType || !r.matchDomain(field, actual, owned, bindings) {
				return false
			}
		}
		if pattern.IsList() && !pattern.IsClosedList() {
			label := adt.MakeIntLabel(adt.IntLabel, int64(len(pattern.Arcs)))
			a, b := &adt.Vertex{Label: label}, &adt.Vertex{Label: label}
			pattern.MatchAndInsert(r.p.ctx, a)
			other.MatchAndInsert(r.p.ctx, b)
			a.Finalize(r.p.ctx)
			b.Finalize(r.p.ctx)
			return r.matchDomain(a, b, owned, bindings)
		}
		return true
	case *adt.Conjunction:
		for _, term := range pattern.Values {
			if !r.matchDomain(term, anchor, owned, bindings) {
				return false
			}
		}
		return true
	case *adt.FuncValue:
		other, ok := anchor.(*adt.FuncValue)
		if !ok || len(pattern.Fn.Params) != len(other.Fn.Params) || len(pattern.Types) != 0 || len(other.Types) != 0 {
			return false
		}
		pt := adt.FuncType{Fn: pattern.Fn, Env: pattern.Env}
		at := adt.FuncType{Fn: other.Fn, Env: other.Env}
		if len(adt.FunctionTypeParameters(pt)) != len(adt.FunctionTypeParameters(at)) {
			return false
		}
		s := &subsumer{ctx: r.p.ctx}
		pt, at, ok = s.capabilityScopes(pt, at)
		if !ok {
			return false
		}
		for i, param := range pt.Fn.Params {
			actual := at.Fn.Params[i]
			if param.Positional != actual.Positional || param.Label != actual.Label || param.ArcType != actual.ArcType ||
				!r.matchDomain(r.p.schema(pt.Env, param.Value), r.p.schema(at.Env, actual.Value), owned, bindings) {
				return false
			}
		}
		return r.matchDomain(r.p.schema(pt.Env, pt.Fn.Ret), r.p.schema(at.Env, at.Fn.Ret), owned, bindings)
	}
	return r.p.includes(pattern, anchor)
}
