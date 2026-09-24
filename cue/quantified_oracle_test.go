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
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

// This model uses only finite sets and Boolean folds. In particular it does
// not import ADT, binding, inference, subsumption, or normalization helpers.
// Domain references are indices into an assignment, not evaluator environments.
// A domain either unions earlier values into its literal set, or excludes them.
type oracleDomain struct {
	mask, refs uint8
	exclude    bool
}

type finiteOracle struct {
	width, depth int
	universal    uint8
	domains      [3]oracleDomain
	relation     uint32
}

func (m finiteOracle) tuples() int {
	n := 1
	for range m.depth {
		n *= m.width
	}
	return n
}

func (m finiteOracle) truth() bool {
	assignment := make([]int, m.depth)
	var eval func(int) bool
	eval = func(level int) bool {
		if level == m.depth {
			index := 0
			for _, value := range assignment {
				index = index*m.width + value
			}
			return m.relation&(1<<index) != 0
		}
		d := m.domains[level]
		mask := d.mask
		for i := range level {
			if d.refs&(1<<i) == 0 {
				continue
			}
			if d.exclude {
				mask &^= 1 << assignment[i]
			} else {
				mask |= 1 << assignment[i]
			}
		}
		all := m.universal&(1<<level) != 0
		for value := range m.width {
			if mask&(1<<value) == 0 {
				continue
			}
			assignment[level] = value
			if got := eval(level + 1); got != all {
				return got
			}
		}
		return all // empty meet is true; empty join is false
	}
	return eval(0)
}

func oracleAtoms(mask uint8, width int) []string {
	var atoms []string
	for i := range width {
		if mask&(1<<i) != 0 {
			atoms = append(atoms, strconv.Itoa(i))
		}
	}
	return atoms
}

// expression translates the model to CUE. The independent truth computation
// above never calls this renderer or consults a compiled CUE value.
func (m finiteOracle) expression(renamed, reverse bool) string {
	names := []string{"x", "y", "z"}
	if renamed {
		names = []string{"outer", "middle", "inner"}
	}
	var terms []string
	for tuple := range m.tuples() {
		if m.relation&(1<<tuple) == 0 {
			continue
		}
		values := make([]int, m.depth)
		n := tuple
		for i := m.depth - 1; i >= 0; i-- {
			values[i], n = n%m.width, n/m.width
		}
		var factors []string
		for i, value := range values {
			factors = append(factors, fmt.Sprintf("%s == %d", names[i], value))
		}
		if reverse {
			slices.Reverse(factors)
		}
		terms = append(terms, "("+strings.Join(factors, " && ")+")")
	}
	if reverse {
		slices.Reverse(terms)
	}
	body := "false"
	if len(terms) != 0 {
		body = strings.Join(terms, " || ")
	}
	// Dependent value-range syntax belongs to profile D, which is not yet
	// implemented. Encode those finite domains as guarded full-universe
	// quantification. All full universes are nonempty, so a guard mentioning
	// only preceding variables commutes through subsequent quantifiers.
	dependent := false
	for _, d := range m.domains {
		dependent = dependent || d.refs != 0
	}
	if dependent {
		for i := m.depth - 1; i >= 0; i-- {
			d := m.domains[i]
			var members, excluded []string
			for _, atom := range oracleAtoms(d.mask, m.width) {
				members = append(members, names[i]+" == "+atom)
			}
			for j := range i {
				if d.refs&(1<<j) != 0 {
					if d.exclude {
						excluded = append(excluded, names[i]+" != "+names[j])
					} else {
						members = append(members, names[i]+" == "+names[j])
					}
				}
			}
			guard := "false"
			if len(members) != 0 {
				guard = "(" + strings.Join(members, " || ") + ")"
			}
			if len(excluded) != 0 {
				guard += " && " + strings.Join(excluded, " && ")
			}
			if m.universal&(1<<i) != 0 {
				body = "!(" + guard + ") || (" + body + ")"
			} else {
				body = "(" + guard + ") && (" + body + ")"
			}
		}
	}
	body = "{ok: true & (" + body + ")}"
	for i := m.depth - 1; i >= 0; i-- {
		d := m.domains[i]
		if dependent {
			d = oracleDomain{mask: uint8((1 << m.width) - 1)}
		}
		atoms := oracleAtoms(d.mask, m.width)
		var excluded []string
		for j := range i {
			if d.refs&(1<<j) != 0 {
				if d.exclude {
					excluded = append(excluded, " != "+names[j])
				} else {
					atoms = append(atoms, names[j])
				}
			}
		}
		if reverse {
			slices.Reverse(atoms)
		}
		domain := "_|_"
		if len(atoms) != 0 {
			domain = strings.Join(atoms, " | ")
		}
		if len(excluded) != 0 {
			domain = "(" + domain + ") & " + strings.Join(excluded, " & ")
		}
		kind := "exists"
		if m.universal&(1<<i) != 0 {
			kind = "forall"
		}
		body = fmt.Sprintf("%s (%s in %s) %s", kind, names[i], domain, body)
	}
	return "(" + body + ") & {ok: true}"
}

// Ordinary validation and concrete certification are separate observations.
// Every case in this bounded literal fragment has an exact, decidable answer;
// incomplete evaluation is a failure, not an acceptable answer for either side.
func finiteOracleMismatch(v cue.Value, want bool) string {
	if !v.Exists() {
		return fmt.Sprintf("oracle program did not compile: %v", v.Err())
	}
	ordinary, concrete := v.Validate(), v.Validate(cue.Concrete(true))
	if (ordinary == nil) != want || (concrete == nil) != want {
		return fmt.Sprintf("want satisfiable=%v; ordinary=%v; concrete=%v", want, ordinary, concrete)
	}
	if want {
		if got, err := v.LookupPath(cue.ParsePath("ok")).Bool(); err != nil || !got {
			return fmt.Sprintf("want ok=true; got %v, %v", got, err)
		}
	}
	return ""
}

// Greedy semantic shrinking terminates because every accepted step removes a
// set bit. It preserves the actual mismatch (including its transformation),
// rather than merely finding a smaller false proposition. Go's fuzz minimizer
// additionally shrinks the serialized input and saves a replayable corpus file.
func shrinkFiniteOracle(m finiteOracle, fails func(finiteOracle) bool) finiteOracle {
	for {
		changed := false
		try := func(candidate finiteOracle) {
			if !changed && fails(candidate) {
				m, changed = candidate, true
			}
		}
		for bit := range m.tuples() {
			if m.relation&(1<<bit) != 0 {
				candidate := m
				candidate.relation &^= 1 << bit
				try(candidate)
			}
		}
		for i := range m.depth {
			for bit := range m.width {
				if m.domains[i].mask&(1<<bit) != 0 {
					candidate := m
					candidate.domains[i].mask &^= 1 << bit
					try(candidate)
				}
			}
			for bit := range i {
				if m.domains[i].refs&(1<<bit) != 0 {
					candidate := m
					candidate.domains[i].refs &^= 1 << bit
					try(candidate)
				}
			}
		}
		if !changed {
			return m
		}
	}
}

func checkFiniteOracle(t *testing.T, ctx *cue.Context, m finiteOracle) {
	t.Helper()
	check := func(candidate finiteOracle) string {
		v := ctx.CompileString("@experiment(quantified)\nv: " + candidate.expression(false, false))
		return finiteOracleMismatch(v.LookupPath(cue.ParsePath("v")), candidate.truth())
	}
	if message := check(m); message != "" {
		small := shrinkFiniteOracle(m, func(candidate finiteOracle) bool { return check(candidate) != "" })
		t.Fatalf("%s\nmodel=%+v\nshrunk: %s\n@experiment(quantified)\nv: %s", message, m, check(small), small.expression(false, false))
	}
}

func extendedQuantifiedOracle(t *testing.T) bool {
	t.Helper()
	switch mode := os.Getenv("CUE_QUANTIFIED_ORACLE"); mode {
	case "", "fast":
		return false
	case "extended":
		return true
	default:
		t.Fatalf("invalid CUE_QUANTIFIED_ORACLE=%q; want fast or extended", mode)
		return false
	}
}

func oracleSeed(t *testing.T) uint64 {
	t.Helper()
	seed := uint64(0x4355452026)
	if s := os.Getenv("CUE_QUANTIFIED_ORACLE_SEED"); s != "" {
		var err error
		seed, err = strconv.ParseUint(s, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func TestQuantifiedOracleExhaustive(t *testing.T) {
	if !extendedQuantifiedOracle(t) {
		t.Skip("set CUE_QUANTIFIED_ORACLE=extended for the larger exhaustive universes")
	}
	for _, dimensions := range [][2]int{{3, 2}, {2, 3}} {
		t.Run(fmt.Sprintf("values=%d/binders=%d", dimensions[0], dimensions[1]), func(t *testing.T) {
			m := finiteOracle{width: dimensions[0], depth: dimensions[1]}
			sets := 1 << m.width
			domainAssignments := 1 << (m.width * m.depth)
			ctx := cuecontext.New()
			cases := 0
			for relation := range 1 << m.tuples() {
				m.relation = uint32(relation)
				for domains := range domainAssignments {
					n := domains
					for i := range m.depth {
						m.domains[i] = oracleDomain{mask: uint8(n % sets)}
						n /= sets
					}
					for prefix := range 1 << m.depth {
						m.universal = uint8(prefix)
						if cases%256 == 0 {
							ctx = cuecontext.New() // bound retained compilation graphs
						}
						checkFiniteOracle(t, ctx, m)
						cases++
					}
				}
			}
			if cases != 131072 {
				t.Fatalf("exhaustive enumeration changed: %d cases", cases)
			}
			t.Logf("checked %d exact denotations and concrete observations", cases)
		})
	}
}

// All two-valued ternary relations and all eight quantifier orders, under a
// vocabulary of dependent domains. The innermost range can refer to both
// enclosing binders. Extended mode exhausts this product; fast mode includes
// every prefix/domain combination and cycles through the truth tables.
func TestQuantifiedOracleDependencies(t *testing.T) {
	extended := extendedQuantifiedOracle(t)
	m := finiteOracle{width: 2, depth: 3}
	outer := []oracleDomain{{}, {mask: 1}, {mask: 2}, {mask: 3}}
	middle := []oracleDomain{{mask: 3}, {refs: 1}, {mask: 3, refs: 1, exclude: true}}
	inner := []oracleDomain{{mask: 3}, {refs: 1}, {refs: 2}, {refs: 3}, {mask: 3, refs: 3, exclude: true}}
	ctx := cuecontext.New()
	cases := 0
	for _, a := range outer {
		for _, b := range middle {
			for _, c := range inner {
				m.domains = [3]oracleDomain{a, b, c}
				for prefix := range 8 {
					m.universal = uint8(prefix)
					count := 1
					if extended {
						count = 256
					}
					for relation := range count {
						m.relation = uint32(relation)
						if !extended {
							m.relation = uint32((cases*73 + 105) % 256)
						}
						if cases%256 == 0 {
							ctx = cuecontext.New()
						}
						checkFiniteOracle(t, ctx, m)
						cases++
					}
				}
			}
		}
	}
	t.Logf("checked %d dependent-domain cases", cases)
}

func decodeFiniteOracle(data []byte) finiteOracle {
	next := func(i int) byte {
		if i < len(data) {
			return data[i]
		}
		return 0
	}
	m := finiteOracle{width: 2 + int(next(0)%2), depth: 1 + int(next(1)%3)}
	m.universal = next(2) & uint8((1<<m.depth)-1)
	for i := range m.depth {
		m.domains[i] = oracleDomain{
			mask:    next(3+3*i) & uint8((1<<m.width)-1),
			refs:    next(4+3*i) & uint8((1<<i)-1),
			exclude: next(5+3*i)%2 != 0,
		}
	}
	for i := range 4 {
		m.relation |= uint32(next(12+i)) << (8 * i)
	}
	m.relation &= uint32((1 << m.tuples()) - 1)
	return m
}

func TestQuantifiedOracleRandom(t *testing.T) {
	seed := oracleSeed(t)
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	count := 128
	if extendedQuantifiedOracle(t) {
		count = 4096
	}
	t.Logf("seed=%#x cases=%d", seed, count)
	for range count {
		data := make([]byte, 16)
		for j := range data {
			data[j] = byte(r.Uint32())
		}
		checkFiniteOracle(t, cuecontext.New(), decodeFiniteOracle(data))
	}
}

// These three-valued, three-binder inputs previously formed exponential
// products of equivalent pending alternatives. Keep them in the ordinary suite
// and the fuzz corpus, without machine-dependent timing assertions.
var finiteNormalizationSeeds = [][]byte{
	{0x79, 0x74, 0x2b, 0xf0, 0x1f, 0x13, 0x82, 0xc8, 0xee, 0xef, 0xb5, 0xde, 0x64, 0xa3, 0x58, 0x72},
	{0xd3, 0x62, 0x8b, 0x06, 0xac, 0x0b, 0xbe, 0x43, 0x6c, 0xa7, 0x94, 0xa8, 0x1c, 0x33, 0xd7, 0x81},
}

func TestQuantifiedOracleNormalization(t *testing.T) {
	for _, data := range finiteNormalizationSeeds {
		checkFiniteOracle(t, cuecontext.New(), decodeFiniteOracle(data))
	}
	// Equal current data does not imply equal predicates. Later refinement
	// must retain every optional constraint, pattern, and open-list tail.
	for _, tt := range []struct {
		body, single, mixed string
	}{
		{"{x?: n, y?: n}", "{x: %d}", "{x: 0, y: 1}"},
		{"{[string]: n}", "{x: %d}", "{x: 0, y: 1}"},
		{"[...n]", "[%d]", "[0, 1]"},
	} {
		ctx := cuecontext.New()
		v := ctx.CompileString("@experiment(quantified)\nv: exists (n in 0|1|2) " + tt.body).LookupPath(cue.ParsePath("v"))
		for n := range 3 {
			if err := v.Unify(ctx.CompileString(fmt.Sprintf(tt.single, n))).Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("%s lost assignment n=%d: %v", tt.body, n, err)
			}
		}
		if err := v.Unify(ctx.CompileString(tt.mixed)).Validate(); err == nil {
			t.Fatalf("%s lost correlation between fields or elements", tt.body)
		}
	}
}

func FuzzQuantifiedFiniteOracle(f *testing.F) {
	for _, data := range finiteNormalizationSeeds {
		f.Add(data)
	}
	for _, data := range [][]byte{
		{},
		{0, 2, 7, 3, 0, 0, 3, 1, 1, 3, 3, 1, 105},
		{1, 2, 5, 7, 0, 0, 7, 1, 1, 7, 3, 1, 0x49, 0x92, 0x24, 0x01},
		{1, 1, 1, 7, 0, 0, 7, 0, 0, 0, 0, 0, 0xef, 1},
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32 {
			t.Skip()
		}
		checkFiniteOracle(t, cuecontext.New(), decodeFiniteOracle(data))
	})
}

// Verify the independent model and shrinker on known mathematical properties.
// The deliberately wrong observation below models a checker that always
// accepts; shrinking must retain a false denotation while reducing its input.
func TestQuantifiedOracleModel(t *testing.T) {
	m := finiteOracle{width: 2, depth: 2, universal: 1, relation: 0b1001,
		domains: [3]oracleDomain{{mask: 3}, {mask: 3}}}
	if !m.truth() {
		t.Fatal("forall x exists y: x=y must hold")
	}
	m.universal = 2
	if m.truth() {
		t.Fatal("exists x forall y: x=y must fail")
	}
	small := shrinkFiniteOracle(m, func(candidate finiteOracle) bool { return !candidate.truth() })
	weight := func(m finiteOracle) int {
		n := bits.OnesCount32(m.relation)
		for _, d := range m.domains {
			n += bits.OnesCount8(d.mask) + bits.OnesCount8(d.refs)
		}
		return n
	}
	if small.truth() || weight(small) >= weight(m) {
		t.Fatalf("shrinker lost the failure or did not simplify: %+v", small)
	}
	for relation := range 16 {
		for prefix := range 4 {
			m.relation, m.universal = uint32(relation), uint8(prefix)
			dual := m
			dual.relation ^= 15
			dual.universal ^= 3
			if m.truth() == dual.truth() {
				t.Fatal("finite quantifier duality failed")
			}
		}
	}
}

var finiteTransformations = []string{
	"identity", "alpha", "reorder", "meet", "record-copy", "list-copy",
	"unify", "fill", "source", "final-source",
}

func transformedFiniteOracle(m finiteOracle, transformation string) (cue.Value, error) {
	ctx := cuecontext.New()
	expr := m.expression(transformation == "alpha", transformation == "reorder")
	source := "v: " + expr
	switch transformation {
	case "alpha":
		// Hostile outer names must not capture the renamed bound variables.
		source = "outer: 91\nmiddle: 92\ninner: 93\n" + source
	case "meet":
		source = "v: {ok: bool} & (" + expr + ") & {ok: true}"
	case "record-copy":
		source = "original: " + expr + "\nv: ({copy: original}).copy"
	case "list-copy":
		source = "v: [" + expr + "][0]"
	}
	v := ctx.CompileString("@experiment(quantified)\n" + source).LookupPath(cue.ParsePath("v"))
	switch transformation {
	case "unify":
		v = ctx.CompileString("{ok: bool}").Unify(v)
	case "fill":
		v = ctx.CompileString("v: {ok: bool}").FillPath(cue.ParsePath("v"), v).LookupPath(cue.ParsePath("v"))
	case "source", "final-source":
		var options []cue.Option
		if transformation == "final-source" {
			options = append(options, cue.Final())
		}
		source, err := format.Node(v.Syntax(options...))
		if err != nil {
			return cue.Value{}, err
		}
		v = cuecontext.New().CompileString("@experiment(quantified)\nv: " + string(source)).LookupPath(cue.ParsePath("v"))
	}
	return v, nil
}

func checkFiniteTransformation(t *testing.T, m finiteOracle, transformation string) {
	t.Helper()
	check := func(candidate finiteOracle) string {
		v, err := transformedFiniteOracle(candidate, transformation)
		if err != nil {
			return err.Error()
		}
		return finiteOracleMismatch(v, candidate.truth())
	}
	if message := check(m); message != "" {
		small := shrinkFiniteOracle(m, func(candidate finiteOracle) bool { return check(candidate) != "" })
		t.Fatalf("transformation=%s: %s\nmodel=%+v\nshrunk: %s\n@experiment(quantified)\nv: %s", transformation, message, m, check(small), small.expression(false, false))
	}
}

func TestQuantifiedOracleTransformations(t *testing.T) {
	seed := oracleSeed(t)
	r := rand.New(rand.NewPCG(seed, 17))
	count := 48
	if extendedQuantifiedOracle(t) {
		count = 512
	}
	t.Logf("seed=%#x models=%d transformations=%d", seed, count, len(finiteTransformations))
	for range count {
		data := make([]byte, 16)
		for i := range data {
			data[i] = byte(r.Uint32())
		}
		m := decodeFiniteOracle(data)
		for _, transformation := range finiteTransformations {
			checkFiniteTransformation(t, m, transformation)
		}
	}
}

func FuzzQuantifiedPreservation(f *testing.F) {
	for i := range finiteTransformations {
		f.Add([]byte{1, 2, 5, 7, 0, 0, 7, 1, 1, 7, 3, 1, 0x49, 0x92, 0x24, 1}, uint8(i))
	}
	f.Fuzz(func(t *testing.T, data []byte, transform uint8) {
		if len(data) > 32 {
			t.Skip()
		}
		checkFiniteTransformation(t, decodeFiniteOracle(data), finiteTransformations[int(transform)%len(finiteTransformations)])
	})
}

// Subtype bounds use the supported dependent type telescope directly. The
// independent oracle is finite-set inclusion, including the empty predicate.
func TestQuantifiedOracleSubtypeBounds(t *testing.T) {
	domain := func(mask int) string {
		atoms := oracleAtoms(uint8(mask), 3)
		if len(atoms) == 0 {
			return "_|_"
		}
		return strings.Join(atoms, " | ")
	}
	cases := 0
	for a := range 8 {
		for b := range 8 {
			for c := range 8 {
				want := b & ^a == 0 && c & ^b == 0
				source := fmt.Sprintf("f(A, B: A, C: B): func(x: C) -> C: x\nselected: f[%s][%s][%s]", domain(a), domain(b), domain(c))
				v := semanticValue(t, source).LookupPath(cue.ParsePath("selected"))
				if !v.Exists() || (v.Validate() == nil) != want {
					t.Fatalf("subset chain=%v: %s\nordinary=%v; concrete=%v", want, source, v.Validate(), v.Validate(cue.Concrete(true)))
				}
				if want {
					if err := v.Err(); err != nil {
						t.Fatalf("admissible selection did not resolve: %s\n%v", source, err)
					}
					// Check every admitted concrete use. Selection admissibility
					// does not by itself certify the retained infinite universal.
					for value := range 3 {
						if c&(1<<value) != 0 {
							called := semanticValue(t, source+fmt.Sprintf("\nout: selected(%d)", value))
							semanticJSON(t, called, "out", strconv.Itoa(value))
						}
					}
				}
				cases++
			}
		}
	}
	t.Logf("checked %d dependent subtype chains", cases)
}

func TestQuantifiedOracleThreeValueDistinction(t *testing.T) {
	for _, width := range []int{2, 3} {
		// For every pair x,y there is a third value distinct from both
		// exactly when the universe has at least three values.
		m := finiteOracle{width: width, depth: 3, universal: 3}
		full := uint8((1 << width) - 1)
		m.domains = [3]oracleDomain{{mask: full}, {mask: full}, {mask: full, refs: 3, exclude: true}}
		m.relation = (1 << m.tuples()) - 1
		if got := m.truth(); got != (width == 3) {
			t.Fatalf("third-value property: width=%d got=%v", width, got)
		}
		checkFiniteOracle(t, cuecontext.New(), m)
		for _, transformation := range finiteTransformations {
			checkFiniteTransformation(t, m, transformation)
		}
	}
}
