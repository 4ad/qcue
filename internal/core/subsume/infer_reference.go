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

// fieldDescription is the common lexical-reference and projection rule. A
// required field supplies a bound on successful completions, not a runtime
// value. The source reference and closure environment retain the demand to
// supply and validate that value when the computation is observed.
func (p *inference) fieldDescription(v *adt.Vertex, label adt.Feature) *adt.Vertex {
	if v == nil || !p.step() {
		return nil
	}
	arc, err := p.ctx.FieldConstraint(v, label)
	field := arc
	if field != nil {
		// Comprehension bindings forward through temporary pending arcs.
		// Finalize their targets, as ordinary reference evaluation does;
		// the wrapper itself has no conjuncts establishing its presence.
		field = field.DerefNonRooted()
		field.Finalize(p.ctx)
	}
	// Subscribe after the read has settled, including unsuccessful reads.
	// Materializing a field may change its container before the field itself
	// exists; both coordinates are dependencies of the proof.
	if graph := p.ctx.Propagation; graph != nil {
		graph.Observe(v)
		graph.Observe(v.DerefValue())
		graph.Observe(arc)
		graph.Observe(field)
	}
	if err != nil || field == nil {
		return nil
	}
	mode := field.ArcType
	if mode != adt.ArcMember && mode != adt.ArcRequired {
		return nil
	}
	if b := field.Bottom(); b != nil && b.IsIncomplete() {
		return nil
	}
	return field
}
