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
	"fmt"
	"slices"
	"strings"

	"cuelang.org/go/internal/core/adt"
)

// RelevanceError blocks admission of an explicit interface. It deliberately
// does not implement adt.Value: a failed relevance check is not a proof that
// the function type denotes bottom. Limit distinguishes incomplete checking
// from a refuted result on a region the finite service could not refute.
type RelevanceError struct {
	Limit  bool
	Reason string
}

func (e *RelevanceError) Error() string { return "interface relevance blocked: " + e.Reason }

// CheckArrowRelevance checks one finite conjunction of explicit arrow clauses
// in their supplied scopes. The caller must open universal declarations with
// rigid variables and supply any independently anchored instances. Selected
// use-site instances and synthesized computation types are not source roots.
// Nested interfaces are separate roots checked by the source traversal.
func CheckArrowRelevance(ctx *adt.OpContext, clauses []adt.FuncType) *RelevanceError {
	p := newCertifier(ctx)
	defer p.enter()()
	return (&relevanceChecker{p: p}).arrows(clauses)
}

type relevanceChecker struct{ p *certifier }

type observation struct {
	domain []adt.Value
	result adt.Value
}

type packetObservations struct {
	clauses []observation
}

func (r *relevanceChecker) blocked(reason string) *RelevanceError {
	if r.p.remaining == 0 {
		return &RelevanceError{Limit: true, Reason: "proof work limit reached"}
	}
	if r.p.ctx.Cancelled() != nil {
		return &RelevanceError{Limit: true, Reason: "checking cancelled"}
	}
	return &RelevanceError{Reason: reason}
}

func (r *relevanceChecker) arrows(clauses []adt.FuncType) *RelevanceError {
	// Eager meet preserves established refutations. If even the conjunction
	// of all results survives, no subset can refute a result. Avoid expanding
	// optional packet shapes for this common, immediately checkable case.
	results := make(map[adt.FuncType]adt.Value)
	var joint adt.Value = &adt.Top{}
	for _, clause := range clauses {
		if len(adt.FunctionTypeParameters(clause)) != 0 {
			return r.blocked("universal declaration requires a rigid checking scope")
		}
		result := r.p.schema(clause.Env, clause.Fn.Ret)
		if result == nil {
			return r.blocked("result predicate remains unresolved")
		}
		results[clause] = result
		joint = r.meet(joint, result)
	}
	if !r.p.step() {
		return r.blocked("")
	}
	if !refuted(joint) {
		return nil
	}
	groups := make(map[string]*packetObservations)
	for _, clause := range clauses {
		if !r.p.step() {
			return r.blocked("")
		}
		result := results[clause]
		params := make([]adt.Value, len(clause.Fn.Params))
		for i, param := range clause.Fn.Params {
			params[i] = r.p.schema(clause.Env, param.Value)
			if params[i] == nil {
				return r.blocked("packet predicate remains unresolved")
			}
		}
		ok := adt.WalkPacketShapes(clause.Fn, r.p.step, func(positional int, slots []int) bool {
			// Label order is not part of a call packet's identity.
			ordered := slices.Clone(slots)
			slices.SortFunc(ordered[positional:], func(i, j int) int {
				return strings.Compare(clause.Fn.Params[i].Label.SelectorString(r.p.ctx),
					clause.Fn.Params[j].Label.SelectorString(r.p.ctx))
			})
			key := fmt.Sprint(positional)
			domain := make([]adt.Value, len(ordered))
			for i, j := range ordered {
				domain[i] = params[j]
				if i >= positional {
					key += fmt.Sprintf("/%d", clause.Fn.Params[j].Label)
				}
			}
			group := groups[key]
			if group == nil {
				group = &packetObservations{}
				groups[key] = group
			}
			group.clauses = append(group.clauses, observation{domain, result})
			return true
		})
		if !ok {
			return r.blocked("open packet row requires a completed protocol")
		}
	}
	var keys []string
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		group := groups[key]
		// Aggregate equal domains, making joint result conflicts visible
		// without enumerating equivalent presence regions.
		var merged []observation
		for _, clause := range group.clauses {
			found := false
			for i, prior := range merged {
				if r.sameDomain(prior.domain, clause.domain) {
					merged[i].result = r.meet(prior.result, clause.result)
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, clause)
			}
		}
		positive := make([]adt.Value, len(merged[0].domain))
		for i := range positive {
			positive[i] = &adt.Top{}
		}
		if err := r.regions(merged, 0, positive, nil, nil); err != nil {
			return err
		}
	}
	if !r.p.step() {
		return r.blocked("")
	}
	return nil
}

func (r *relevanceChecker) sameDomain(a, b []adt.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !r.p.step() || !r.p.includes(a[i], b[i]) || !r.p.includes(b[i], a[i]) {
			return false
		}
	}
	return true
}

// meet runs ordinary eager normalization in a fresh vertex. Evaluating a raw
// Conjunction alone can retain validators without checking their joint
// emptiness; source relevance needs the complete shared eager pass.
func (r *relevanceChecker) meet(a, b adt.Value) adt.Value {
	return r.p.eagerMeet(a, b)
}

func refuted(v adt.Value) bool {
	b, ok := adt.Unwrap(v).(*adt.Bottom)
	return ok && !b.IsIncomplete()
}

func (r *relevanceChecker) regions(clauses []observation, i int, positive []adt.Value, negative [][]adt.Value, result adt.Value) *RelevanceError {
	if !r.p.step() {
		return r.blocked("")
	}
	if i == len(clauses) {
		if result != nil && refuted(result) && !r.covered(positive, negative) {
			return r.blocked("empty successful result on a non-refuted input region")
		}
		if r.p.remaining == 0 {
			return r.blocked("")
		}
		return nil
	}
	clause := clauses[i]
	included := make([]adt.Value, len(positive))
	empty := false
	for j, a := range positive {
		included[j] = r.meet(a, clause.domain[j])
		empty = empty || refuted(included[j])
	}
	if !empty {
		next := clause.result
		if result != nil {
			next = r.meet(result, next)
		}
		if err := r.regions(clauses, i+1, included, negative, next); err != nil {
			return err
		}
	}
	return r.regions(clauses, i+1, positive, append(slices.Clone(negative), clause.domain), result)
}

// covered proves that a positive product is contained in the union of the
// excluded domains. Difference retains a sound overapproximation where the
// finite predicate rules do not decide a complement; unknown never proves
// emptiness. Thus the checker retains exact negative guards rather than
// equating an unresolved overlap with a surviving exact region.
func (r *relevanceChecker) covered(positive []adt.Value, negative [][]adt.Value) bool {
	boxes := [][]adt.Value{positive}
	for _, excluded := range negative {
		var next [][]adt.Value
		for _, box := range boxes {
			if !r.p.step() {
				return false
			}
			prefix := slices.Clone(box)
			for i, value := range box {
				for _, remaining := range r.difference(value, excluded[i]) {
					part := slices.Clone(prefix)
					part[i] = remaining
					next = append(next, part)
				}
				prefix[i] = r.meet(value, excluded[i])
				if refuted(prefix[i]) {
					break
				}
			}
		}
		boxes = next
		if !r.p.step() {
			return false
		}
		if len(boxes) == 0 {
			return true
		}
	}
	return len(boxes) == 0
}

func (r *relevanceChecker) difference(a, b adt.Value) []adt.Value {
	if !r.p.step() || a == nil || b == nil {
		return []adt.Value{a}
	}
	if refuted(a) || r.p.includes(b, a) {
		return nil
	}
	if refuted(r.meet(a, b)) {
		return []adt.Value{a}
	}
	if union, ok := adt.Unwrap(a).(*adt.Disjunction); ok {
		var out []adt.Value
		for _, branch := range union.Values {
			out = append(out, r.difference(branch, b)...)
		}
		return out
	}
	switch x := adt.Unwrap(b).(type) {
	case *adt.Disjunction:
		out := []adt.Value{a}
		for _, branch := range x.Values {
			var next []adt.Value
			for _, part := range out {
				next = append(next, r.difference(part, branch)...)
			}
			out = next
		}
		return out
	case *adt.Conjunction:
		var out []adt.Value
		for _, term := range x.Values {
			out = append(out, r.difference(a, term)...)
		}
		return out
	case *adt.BasicType:
		return r.withComplement(a, x.K, nil)
	case *adt.Null:
		return r.withComplement(a, adt.NullKind, nil)
	case *adt.Bool:
		return r.withComplement(a, adt.BoolKind, &adt.Bool{B: !x.B})
	case *adt.Num:
		outside := &adt.Disjunction{Values: []adt.Value{
			&adt.BoundValue{Op: adt.LessThanOp, Value: x},
			&adt.BoundValue{Op: adt.GreaterThanOp, Value: x},
		}}
		return r.withComplement(a, x.Kind(), outside)
	case *adt.String, *adt.Bytes:
		return r.withComplement(a, b.Kind(), &adt.BoundValue{Op: adt.NotEqualOp, Value: adt.Unwrap(b)})
	case *adt.BoundValue:
		op := adt.NoOp
		switch x.Op {
		case adt.LessThanOp:
			op = adt.GreaterEqualOp
		case adt.LessEqualOp:
			op = adt.GreaterThanOp
		case adt.GreaterThanOp:
			op = adt.LessEqualOp
		case adt.GreaterEqualOp:
			op = adt.LessThanOp
		case adt.NotEqualOp:
			if x.Value.Kind()&adt.NumberKind != 0 {
				// Numeric comparison ignores the integer/decimal spelling.
				// Equality to the endpoint therefore retains both kinds.
				equal := r.meet(&adt.BoundValue{Op: adt.GreaterEqualOp, Value: x.Value},
					&adt.BoundValue{Op: adt.LessEqualOp, Value: x.Value})
				return r.withComplement(a, x.Kind(), equal)
			}
			return r.withComplement(a, x.Kind(), x.Value)
		case adt.MatchOp:
			op = adt.NotMatchOp
		case adt.NotMatchOp:
			op = adt.MatchOp
		}
		if op != adt.NoOp {
			return r.withComplement(a, x.Kind(), &adt.BoundValue{Op: op, Value: x.Value})
		}
	}
	return []adt.Value{a}
}

func (r *relevanceChecker) withComplement(a adt.Value, kind adt.Kind, sameKind adt.Value) []adt.Value {
	var out []adt.Value
	if other := a.Kind() &^ kind; other != adt.BottomKind {
		if v := r.meet(a, &adt.BasicType{K: other}); !refuted(v) {
			out = append(out, v)
		}
	}
	if sameKind != nil {
		if v := r.meet(a, sameKind); !refuted(v) {
			out = append(out, v)
		}
	}
	return out
}
