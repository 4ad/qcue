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

func CheckArrowRelevanceBudget(ctx *adt.OpContext, clauses []adt.FuncType, budget int) (*RelevanceError, int) {
	p := newCertifier(ctx)
	defer p.enter()()
	p.remaining = budget
	err := (&relevanceChecker{p: p}).arrows(clauses)
	return err, budget - p.remaining
}

// Test exact negative guards independently of the enclosing arrow search.
func RelevanceRegionCovered(ctx *adt.OpContext, positive []adt.Value, negative [][]adt.Value) bool {
	p := newCertifier(ctx)
	defer p.enter()()
	return (&relevanceChecker{p: p}).covered(positive, negative)
}
