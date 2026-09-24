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

// packetSlot identifies the two parameters fed by one supplied packet entry.
// Optional entries can be omitted by both rows. Parameter defaults affect
// admissible omission, but never add an entry to the supplied packet.
type packetSlot struct {
	a, b     int
	optional bool
}

// packetIntersection describes a family of common packet shapes with one
// positional prefix. The remaining slots are labeled; independently choosing
// presence for each optional slot describes the whole family without an
// exponential enumeration. Constraints on entry values are checked separately.
type packetIntersection struct {
	positional int
	slots      []packetSlot
}

// intersectPacketDomains enumerates all intersections of two closed protocol
// rows. This is a symmetric domain judgment, not directional parameter
// alignment. Every complete packet has a positional prefix followed by unique
// labels. A positional parameter may instead be supplied by its label, so a
// mismatch of positional modes is never, by itself, evidence of disjointness.
// false means that this procedure does not cover the rows (currently open
// rows), not that their domains are disjoint.
func intersectPacketDomains(a, b *Function) ([]packetIntersection, bool) {
	if a.Open || b.Open {
		return nil, false
	}
	positions := func(f *Function) []int {
		var out []int
		for i, p := range f.Params {
			if p.Positional {
				out = append(out, i)
			}
		}
		return out
	}
	ap, bp := positions(a), positions(b)
	omittable := func(p FuncParam) bool { return p.ArcType == ArcOptional || p.Default != nil }
	var result []packetIntersection
prefix:
	for n := 0; n <= min(len(ap), len(bp)); n++ {
		shape := packetIntersection{positional: n}
		boundA, boundB := make([]bool, len(a.Params)), make([]bool, len(b.Params))
		for i := range n {
			shape.slots = append(shape.slots, packetSlot{a: ap[i], b: bp[i]})
			boundA[ap[i]], boundB[bp[i]] = true, true
		}
		// Required slots must be supplied by a label that addresses a free
		// slot in both rows. Optional unmatched slots are simply absent.
		for i, p := range a.Params {
			if boundA[i] {
				continue
			}
			j := -1
			if p.Label != InvalidLabel {
				for k, q := range b.Params {
					if q.Label == p.Label && !boundB[k] {
						j = k
						break
					}
				}
			}
			if j < 0 {
				if !omittable(p) {
					continue prefix
				}
				continue
			}
			shape.slots = append(shape.slots, packetSlot{a: i, b: j,
				optional: omittable(p) && omittable(b.Params[j])})
			boundB[j] = true
		}
		for j, p := range b.Params {
			if !boundB[j] && !omittable(p) {
				continue prefix
			}
		}
		result = append(result, shape)
	}
	return result, true
}
