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

package adt

import "maps"

func sameTypeEnvironment(c *OpContext, a, b *Environment) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || !sameTypeEnvironment(c, a.Up, b.Up) {
		return false
	}
	if a.types == nil || b.types == nil {
		return a.types == b.types && a.DerefVertex(c) == b.DerefVertex(c)
	}
	if a.types.quantifier != b.types.quantifier || len(a.types.arguments) != len(b.types.arguments) {
		return false
	}
	if !maps.Equal(a.types.erasedIndices, b.types.erasedIndices) {
		return false
	}
	for param, value := range a.types.arguments {
		other, ok := b.types.arguments[param]
		// Data equality omits patterns, optional fields and preferences.
		// Only shared predicates or this exact scalar vocabulary justify
		// the shortcut; structural arguments go through membership below.
		if !ok || value != other && (!fixedCapabilityExpr(value) || !fixedCapabilityExpr(other) ||
			!Equal(c, value, other, CheckStructural)) {
			return false
		}
	}
	return true
}

func sameQuantifiedEnvironment(c *OpContext, q *Quantified, env, other *Environment) bool {
	if sameTypeEnvironment(c, env, other) {
		return true
	}
	for _, ref := range q.References {
		x, xok := c.Evaluate(env, ref)
		y, yok := c.Evaluate(other, ref)
		if !xok || !yok || x == nil || y == nil {
			return false
		}
		if a, ok := x.(*Vertex); ok {
			if b, ok := y.(*Vertex); ok && a.DerefValue() == b.DerefValue() {
				continue
			}
		}
		predicate := false
		switch r := ref.(type) {
		case *TypeReference:
			predicate = true
		case *FieldReference:
			predicate = r.Label.IsDef()
		case *LetReference:
			predicate = r.IsPredicate
		}
		if predicate {
			if !c.provesInclusion(x, y) || !c.provesInclusion(y, x) {
				return false
			}
		} else if !concreteCapture(c, x) || !concreteCapture(c, y) || !Equal(c, x, y, CheckStructural) {
			return false
		}
	}
	return true
}
