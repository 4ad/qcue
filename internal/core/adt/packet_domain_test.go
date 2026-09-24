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

import (
	"fmt"
	"math/bits"
	"os"
	"slices"
	"testing"
)

type oraclePacket struct{ positional, labels int }

// Return the supplied packet entry feeding each parameter, or -1 for omission.
// This is a direct implementation of concrete packet admission. It does not
// call evaluator binding, parameter matching, or domain intersection helpers.
func oraclePacketBinding(row *Function, packet oraclePacket, labels []Feature) ([]int, bool) {
	bound := make([]int, len(row.Params))
	for i := range bound {
		bound[i] = -1
	}
	remaining := packet.positional
	for i, p := range row.Params {
		if p.Positional && remaining > 0 {
			bound[i] = packet.positional - remaining
			remaining--
		}
	}
	if remaining != 0 {
		return nil, false
	}
	for bit, label := range labels {
		if packet.labels&(1<<bit) == 0 {
			continue
		}
		found := false
		for i, p := range row.Params {
			if p.Label == label {
				if bound[i] >= 0 {
					return nil, false
				}
				bound[i], found = packet.positional+bit, true
			}
		}
		if !found {
			return nil, false
		}
	}
	for i, p := range row.Params {
		if bound[i] < 0 && p.ArcType != ArcOptional && p.Default == nil {
			return nil, false
		}
	}
	return bound, true
}

func oraclePacketMask(row *Function, labels []Feature, size int) uint64 {
	var mask uint64
	for n := range size + 1 {
		for fields := range 1 << len(labels) {
			if _, ok := oraclePacketBinding(row, oraclePacket{n, fields}, labels); ok {
				mask |= 1 << (n*(1<<len(labels)) + fields)
			}
		}
	}
	return mask
}

// Materialize the implementation's symbolic families. In addition to domain
// equality, check that each slot feeds the correct parameter on BOTH sides;
// a correct set of packet shapes alone would not detect a transport miswiring.
func oracleIntersectionMask(a, b *Function, labels []Feature, size int) (uint64, string) {
	shapes, complete := intersectPacketDomains(a, b)
	if !complete {
		return 0, "closed rows were not covered"
	}
	var result uint64
	for _, shape := range shapes {
		if shape.positional < 0 || shape.positional > size || len(shape.slots) < shape.positional {
			return 0, "invalid positional prefix"
		}
		allowed, required := 0, 0
		seenA, seenB := 0, 0
		for i, slot := range shape.slots {
			if slot.a < 0 || slot.a >= len(a.Params) || slot.b < 0 || slot.b >= len(b.Params) ||
				seenA&(1<<slot.a) != 0 || seenB&(1<<slot.b) != 0 {
				return 0, "invalid or duplicated parameter mapping"
			}
			seenA |= 1 << slot.a
			seenB |= 1 << slot.b
			if i < shape.positional {
				if slot.optional || !a.Params[slot.a].Positional || !b.Params[slot.b].Positional {
					return 0, "invalid positional mapping"
				}
				continue
			}
			label := a.Params[slot.a].Label
			bit := slices.Index(labels, label)
			if bit < 0 || b.Params[slot.b].Label != label || allowed&(1<<bit) != 0 {
				return 0, "invalid labeled mapping"
			}
			allowed |= 1 << bit
			if !slot.optional {
				required |= 1 << bit
			}
		}
		for fields := range 1 << len(labels) {
			if fields & ^allowed != 0 || fields&required != required {
				continue
			}
			packet := oraclePacket{shape.positional, fields}
			ab, aok := oraclePacketBinding(a, packet, labels)
			bb, bok := oraclePacketBinding(b, packet, labels)
			if !aok || !bok {
				return 0, fmt.Sprintf("family admits rejected packet %+v", packet)
			}
			for i, slot := range shape.slots {
				entry := i
				if i >= shape.positional {
					bit := slices.Index(labels, a.Params[slot.a].Label)
					if fields&(1<<bit) == 0 {
						continue
					}
					entry = shape.positional + bit
				}
				if ab[slot.a] != entry || bb[slot.b] != entry {
					return 0, fmt.Sprintf("wrong parameter mapping for packet %+v", packet)
				}
			}
			result |= 1 << (shape.positional*(1<<len(labels)) + fields)
		}
	}
	return result, ""
}

func oracleRows(labels []Feature, size int) []*Function {
	var params []FuncParam
	for _, label := range append([]Feature{InvalidLabel}, labels...) {
		for _, positional := range []bool{false, true} {
			if label == InvalidLabel && !positional {
				continue
			}
			for omission := range 3 {
				p := FuncParam{Label: label, Positional: positional}
				switch omission {
				case 1:
					p.ArcType = ArcOptional
				case 2:
					p.Default = &Null{}
				}
				params = append(params, p)
			}
		}
	}
	var rows []*Function
	var extend func([]FuncParam)
	extend = func(row []FuncParam) {
		rows = append(rows, &Function{Params: slices.Clone(row)})
		if len(row) == size {
			return
		}
		for _, p := range params {
			if p.Label != InvalidLabel && slices.ContainsFunc(row, func(q FuncParam) bool { return q.Label == p.Label }) {
				continue
			}
			extend(append(slices.Clone(row), p))
		}
	}
	extend(nil)
	return rows
}

func shrinkOracleRows(a, b *Function, fails func(*Function, *Function) bool) (*Function, *Function) {
	for {
		changed := false
		for side, row := range []*Function{a, b} {
			for i := range row.Params {
				copy := *row
				copy.Params = slices.Delete(slices.Clone(row.Params), i, i+1)
				x, y := a, b
				if side == 0 {
					x = &copy
				} else {
					y = &copy
				}
				if fails(x, y) {
					a, b, changed = x, y, true
					break
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			return a, b
		}
	}
}

func checkOracleRows(t *testing.T, a, b *Function, labels []Feature, size int, want uint64) {
	t.Helper()
	got, err := oracleIntersectionMask(a, b, labels, size)
	if err == "" && got == want {
		return
	}
	smallA, smallB := shrinkOracleRows(a, b, func(x, y *Function) bool {
		got, err := oracleIntersectionMask(x, y, labels, size)
		return err != "" || got != oraclePacketMask(x, labels, size)&oraclePacketMask(y, labels, size)
	})
	t.Fatalf("intersection=%#x oracle=%#x first differing packet bit=%d: %s\na=%+v\nb=%+v\nshrunk a=%+v\nshrunk b=%+v", got, want, bits.TrailingZeros64(got^want), err, a.Params, b.Params, smallA.Params, smallB.Params)
}

func TestQuantifiedPacketDomainModel(t *testing.T) {
	size := 2
	switch mode := os.Getenv("CUE_QUANTIFIED_ORACLE"); mode {
	case "", "fast":
	case "extended":
		size = 3
	default:
		t.Fatalf("invalid CUE_QUANTIFIED_ORACLE=%q", mode)
	}
	labels := []Feature{makeLabel(1, StringLabel), makeLabel(2, StringLabel), makeLabel(3, StringLabel)}[:size]
	rows := oracleRows(labels, size)
	masks := make([]uint64, len(rows))
	for i, row := range rows {
		masks[i] = oraclePacketMask(row, labels, size)
	}
	for i, a := range rows {
		for j, b := range rows {
			checkOracleRows(t, a, b, labels, size, masks[i]&masks[j])
		}
	}
	wantRows := 169
	if size == 3 {
		wantRows = 4108
	}
	if len(rows) != wantRows {
		t.Fatalf("row enumeration changed: got %d, want %d", len(rows), wantRows)
	}
	packets := (size + 1) * (1 << len(labels))
	t.Logf("checked %d ordered row pairs against %d packet shapes each (%d comparisons), including slot mappings", len(rows)*len(rows), packets, len(rows)*len(rows)*packets)
}

func FuzzQuantifiedPacketDomain(f *testing.F) {
	f.Add([]byte{3, 3, 1, 2, 3, 4, 5, 6})
	f.Add([]byte{2, 1, 9, 18, 0, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32 {
			t.Skip()
		}
		next := func(i int) byte {
			if i < len(data) {
				return data[i]
			}
			return 0
		}
		labels := []Feature{makeLabel(1, StringLabel), makeLabel(2, StringLabel), makeLabel(3, StringLabel)}
		rows := [2]*Function{{}, {}}
		for side, row := range rows {
			for i := range int(next(side) % 4) {
				x := next(2 + side*3 + i)
				p := FuncParam{Positional: x&1 != 0}
				label := int(x/2) % 4
				if label == 0 {
					p.Positional = true
				} else {
					p.Label = labels[label-1]
					if slices.ContainsFunc(row.Params, func(q FuncParam) bool { return p.Label == q.Label }) {
						continue
					}
				}
				switch x / 8 % 3 {
				case 1:
					p.ArcType = ArcOptional
				case 2:
					p.Default = &Null{}
				}
				row.Params = append(row.Params, p)
			}
		}
		checkOracleRows(t, rows[0], rows[1], labels, 3, oraclePacketMask(rows[0], labels, 3)&oraclePacketMask(rows[1], labels, 3))
	})
}
