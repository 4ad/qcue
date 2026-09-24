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

package cue_test

import (
	"fmt"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

// Each generated implementation is identity. Its membership in D -> R is
// therefore exactly D subset R, computed using finite sets. The generator
// varies the path by which that same obligation reaches a runtime boundary.
type featureOracle struct {
	domain, result uint8 // nonempty subsets of {0,1,2}
	flags          uint8
}

const (
	featureHidden = 1 << iota
	featureNested
	featurePartial
	featureOpaque
	featureReorder
	featureRoundTrip
	featureInvoke
)

func (m featureOracle) source(consumer string) string {
	label := "cb"
	if m.flags&featureHidden != 0 {
		label = "_cb"
	}
	wrap := func(value string) string {
		x := "{" + label + ": " + value + "}"
		if m.flags&featureNested != 0 {
			x = "{nest: [" + x + "]}"
		}
		return x
	}
	access := "packet."
	if m.flags&featureNested != 0 {
		access += "nest[0]."
	}
	ret, body := "#D", "x"
	if m.flags&featureInvoke != 0 {
		ret, body = "#R", access+label+"(x)"
	}
	first, second := "T", "U"
	if m.flags&featureReorder != 0 {
		first, second = "Outer", "Inner"
	}
	if consumer == "" {
		consumer = fmt.Sprintf("forall (%s, %s: %s) func(packet: %s, x: #D) -> %s: %s", first, second, first, wrap("func(#D) -> #R"), ret, body)
	}
	value := bits.TrailingZeros8(m.domain)
	invoke := fmt.Sprintf("consume[0|1|2][#D](%s, %d)", wrap("selected"), value)
	if m.flags&featurePartial != 0 {
		invoke = fmt.Sprintf("consume[0|1|2][#D](%s, ...)(%d)", wrap("selected"), value)
	}
	var provider, out, distinct string
	if m.flags&featureOpaque != 0 {
		provider = `#M: exists State {
 seed: State
 f: forall A func(A) -> A
 alias: f
 other: forall A func(A) -> A
}
p: seal #M with (State = int) {
 seed: 2
 f: forall A func(x: A) -> A: x
 alias: f
 other: f
}`
		out = "out: (open p as (S, P) {\nlet copy = P\nlet selected = P.f[#D] & copy.alias[#D]\nv: " + invoke + "\n}).v"
		distinct = "different: (open p as (S, P) {v: P.f & P.other}).v"
	} else {
		provider = "id(A): func(x: A) -> A: x\nalias: id"
		out = "let selected = id[#D] & alias[#D]\nout: " + invoke
	}
	parts := []string{
		"#D: " + strings.Join(oracleAtoms(m.domain, 3), " | "),
		"#R: " + strings.Join(oracleAtoms(m.result, 3), " | "),
		"consume: " + consumer, provider, out, distinct,
	}
	if m.flags&featureReorder != 0 {
		slices.Reverse(parts)
	}
	return "@experiment(quantified)\n" + strings.Join(parts, "\n")
}

// These finite scalar unions all use identity transport. Every independently
// valid case must now certify and execute, including opaque combinations.
// Unknown is not accepted as evidence for this supported vocabulary.
func featureOracleObservation(m featureOracle) (outcome, mismatch string) {
	ctx := cuecontext.New()
	v := ctx.CompileString(m.source(""))
	if m.flags&featureRoundTrip != 0 {
		consumer := v.LookupPath(cue.ParsePath("consume"))
		source, err := format.Node(consumer.Syntax(cue.Final()))
		if err != nil {
			return "", fmt.Sprintf("consumer export: %v", err)
		}
		v = cuecontext.New().CompileString(m.source(string(source)))
	}
	out := v.LookupPath(cue.ParsePath("out"))
	if !out.Exists() {
		return "", fmt.Sprintf("generated program did not compile: %v", v.Err())
	}
	if m.flags&featureOpaque != 0 {
		if err := v.LookupPath(cue.ParsePath("different")).Validate(); err == nil {
			return "", "private sharing identified distinct public exports"
		}
	}
	want := m.domain & ^m.result == 0
	ordinary, concrete := out.Validate(), out.Validate(cue.Concrete(true))
	value, jsonErr := out.MarshalJSON()
	if !want {
		if concrete == nil || jsonErr == nil {
			return "", fmt.Sprintf("invalid callback certified: ordinary=%v concrete=%v JSON=%s, %v", ordinary, concrete, value, jsonErr)
		}
		return "rejected", ""
	}
	if ordinary != nil {
		return "", fmt.Sprintf("valid callback refuted: %v", ordinary)
	}
	if concrete != nil || jsonErr != nil {
		return "", fmt.Sprintf("supported valid callback unresolved: concrete=%v JSON=%s, %v", concrete, value, jsonErr)
	}
	wantJSON := strconv.Itoa(bits.TrailingZeros8(m.domain))
	if string(value) != wantJSON {
		return "", fmt.Sprintf("identity returned %s, want %s", value, wantJSON)
	}
	return "established", ""
}

func shrinkFeatureOracle(m featureOracle, fails func(featureOracle) bool) featureOracle {
	for {
		changed := false
		for part := range 3 {
			for bit := range 8 {
				candidate := m
				switch part {
				case 0:
					candidate.domain &^= 1 << bit
				case 1:
					candidate.result &^= 1 << bit
				case 2:
					candidate.flags &^= 1 << bit
				}
				if candidate != m && candidate.domain != 0 && candidate.result != 0 && fails(candidate) {
					m, changed = candidate, true
					break
				}
			}
			if changed {
				break
			}
		}
		if !changed {
			return m
		}
	}
}

func checkFeatureOracle(t *testing.T, m featureOracle) string {
	t.Helper()
	outcome, message := featureOracleObservation(m)
	if message != "" {
		small := shrinkFeatureOracle(m, func(candidate featureOracle) bool {
			_, mismatch := featureOracleObservation(candidate)
			return mismatch != ""
		})
		_, shrunk := featureOracleObservation(small)
		t.Fatalf("%s\nmodel=%+v\nshrunk model=%+v: %s\n%s", message, m, small, shrunk, small.source(""))
	}
	return outcome
}

func TestQuantifiedOracleFeatureCombinations(t *testing.T) {
	extended := extendedQuantifiedOracle(t)
	outcomes := make(map[string]int)
	opaqueUnionControls := 0
	check := func(m featureOracle) {
		if m.domain & ^m.result == 0 && m.flags&featureOpaque != 0 && bits.OnesCount8(m.domain) > 1 {
			opaqueUnionControls++
		}
		outcomes[checkFeatureOracle(t, m)]++
	}
	for flags := range 128 {
		if !extended {
			m := featureOracle{domain: uint8(1 + flags*5%7), result: uint8(1 + flags*3%7), flags: uint8(flags)}
			if flags%3 == 0 {
				m.result = m.domain
			}
			check(m)
			continue
		}
		for domain := uint8(1); domain < 8; domain++ {
			for result := uint8(1); result < 8; result++ {
				m := featureOracle{domain, result, uint8(flags)}
				check(m)
			}
		}
	}
	// Keep positive, negative, and overlapping ordinary-union controls.
	// Regressing a proved identity transport to unknown must fail this suite.
	if outcomes["established"] == 0 || outcomes["rejected"] == 0 || opaqueUnionControls == 0 {
		t.Fatalf("missing positive, negative or opaque-union controls: %v", outcomes)
	}
	t.Logf("feature combinations: established=%d rejected=%d residual=%d", outcomes["established"], outcomes["rejected"], outcomes["residual"])
}

func FuzzQuantifiedFeatureCombinations(f *testing.F) {
	for _, data := range [][]byte{{0, 0, 0}, {2, 0, 15}, {6, 6, 127}, {0, 6, 127}} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 16 {
			t.Skip()
		}
		checkFeatureOracle(t, featureOracle{1 + data[0]%7, 1 + data[1]%7, data[2] & 127})
	})
}
