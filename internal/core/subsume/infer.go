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
	"cuelang.org/go/internal/core/walk"
)

// A derivation evaluates descriptions under its own assumption store. The
// evaluator hooks remain installed throughout: their dispatch consults this
// store, so nested reductions use the same hypotheses and accounted work.
// Entering a store never establishes any of its pending propositions.
func (p *inference) enter() func() {
	previous := p.ctx.Inference
	p.ctx.Inference = p
	return func() { p.ctx.Inference = previous }
}

func newInference(ctx *adt.OpContext) *inference {
	return &inference{ctx: ctx, support: &assumptionStore{},
		hypotheses: make(map[*adt.FuncValue]bool), scopes: make(map[*adt.Environment]*proofScope),
		projections:  make(map[*adt.Vertex]map[adt.Feature]adt.Value),
		constructors: make(map[*adt.Vertex]adt.Expr),
		memberships:  make(map[adt.Value][]*adt.LiveType),
		records:      make(map[proofRecordKey]*proofRecord),
		completed:    make(map[proofKey]proofCertificate), refutations: make(map[proofKey]*refutation), remaining: 10000}
}

// Reuse the current proof context when validating captured composites.
// Starting another inference would forget active proof dependencies and
// permit cycles through records or lists to justify their own annotations.
func (p *inference) validateFunction(_ *adt.OpContext, f *adt.FuncValue) *adt.Bottom {
	if !p.implementation(f) {
		if p.failure != nil {
			return p.failure
		}
		for _, target := range f.Obligations() {
			if p.refutations[proofKey{f, target}] != nil {
				return &adt.Bottom{Src: f.Source(), Code: adt.BlockedError,
					Err: p.ctx.Newf("function requirement refuted by a checked counterexample")}
			}
		}
		if p.remaining == 0 {
			return &adt.Bottom{Src: f.Source(), Code: adt.BlockedError,
				Err: p.ctx.Newf("function conformance remains unproved: proof work limit reached")}
		}
		return &adt.Bottom{Src: f.Source(), Code: adt.BlockedError,
			Err: p.ctx.Newf("function conformance remains unproved")}
	}
	return nil
}

type proofScope struct {
	presence map[adt.Feature]adt.ArcType
	values   map[adt.Feature]adt.Value
	optional map[adt.Feature]adt.Value
	absent   map[adt.Feature]bool
	fields   map[adt.Feature]adt.Expr
	bindings map[adt.Feature][]proofBinding
	active   map[adt.Feature]bool
}

type proofBinding struct {
	env  *adt.Environment
	expr adt.Expr
}

type inference struct {
	support *assumptionStore
	ctx     *adt.OpContext
	active  []*adt.FuncValue
	// A recursive implementation can use its declared contracts while all
	// their bodies are being checked. These hypotheses never survive a
	// failed implementation proof or justify an unrelated stronger view.
	implementing []*adt.FuncValue
	hypotheses   map[*adt.FuncValue]bool
	scopes       map[*adt.Environment]*proofScope
	// Constructed composites retain their synthesized members, including
	// callback hypothesis identity. Re-evaluating a schema approximation
	// must not replace the evidence attached to a projected runtime value.
	projections map[*adt.Vertex]map[adt.Feature]adt.Value
	// Constructor descriptions precede the exact field inventory added to
	// their synthesized types. Source meets combine those descriptions;
	// a typing inventory is not an explicit close constraint.
	constructors map[*adt.Vertex]adt.Expr
	// Membership facts keep live coordinates alongside normalized summaries.
	// Equal current data is not enough to manufacture one of these facts.
	memberships map[adt.Value][]*adt.LiveType
	records     map[proofRecordKey]*proofRecord
	refutations map[proofKey]*refutation
	completed   map[proofKey]proofCertificate
	callProofs  map[*adt.FuncValue][]adt.FuncType
	attempts    []*proofAttempt
	remaining   int
	failure     *adt.Bottom
}

type proofKey struct {
	f      *adt.FuncValue
	target adt.FuncType
}

type proofCertificate struct {
	required []*adt.FuncValue
	// Keep the checked body's successful-result description. A weaker
	// annotation must not discard fields or refinements proved by its body.
	result adt.Value
}

// A completed proof can be reused only while its inherited hypotheses are
// still available. Hypotheses introduced by the function's own packet are
// discharged by that proof and are not prerequisites for its callers.
type proofAttempt struct {
	inherited map[*adt.FuncValue]bool
	required  map[*adt.FuncValue]bool
}

func (p *inference) useHypothesis(f *adt.FuncValue) {
	for _, a := range p.attempts {
		if a.inherited[f] {
			a.required[f] = true
		}
	}
}

func (p *inference) step() bool {
	if p.ctx.Cancelled() != nil {
		return false
	}
	if p.remaining == 0 {
		return false
	}
	if p.ctx.Propagation != nil && !p.ctx.Propagation.Charge(1) {
		p.remaining = 0
		return false
	}
	p.remaining--
	return true
}

func (p *inference) schema(env *adt.Environment, x adt.Expr) adt.Value {
	if !p.step() {
		return nil
	}
	if x == nil {
		return &adt.Top{}
	}
	if env == nil {
		env = &adt.Environment{Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	}
	// A predicate abbreviation is substitution, not a fresh data cell. Read
	// its expression in the declaration scope before ordinary vertex
	// evaluation can turn a live membership into only its upper bound.
	if alias, ok := x.(*adt.LetReference); ok && alias.IsPredicate {
		for range alias.UpCount {
			if env == nil {
				return nil
			}
			env = env.Up
		}
		return p.schema(env, alias.X)
	}
	v, ok := p.ctx.Evaluate(env, x)
	if !ok || v == nil {
		return nil
	}
	if vertex, ok := v.(*adt.Vertex); ok {
		// The premise consumed below is the finalized value. Subscribe only
		// after obtaining it: completing this read is not new evidence for
		// the inference which is already using that completion.
		vertex.Finalize(p.ctx)
		if p.ctx.Propagation != nil {
			p.ctx.Propagation.Observe(vertex)
		}
		if _, shared := vertex.BaseValue.(*adt.Vertex); shared && vertex.ClosedNonRecursive {
			// close returns a wrapper whose closedness takes effect when
			// inserted into a conjunct. A bare Evaluate result must not
			// lose that constraint when a proof inspects the shared value.
			closed := &adt.Vertex{}
			closed.AddConjunct(adt.MakeRootConjunct(nil, vertex))
			closed.Finalize(p.ctx)
			v = closed
		}
	}
	if b, ok := adt.Unwrap(v).(*adt.Bottom); ok && b.IsIncomplete() {
		return nil
	}
	return v
}

func (p *inference) includes(want, got adt.Value) bool {
	if want == nil || got == nil {
		return false
	}
	if live, ok := adt.Unwrap(want).(*adt.LiveType); ok {
		for _, member := range p.memberships[got] {
			if live.SameReference(p.ctx, member) {
				return true
			}
		}
		got = p.constructorEvidence(got)
		if got == nil {
			return false
		}
	}
	s := &subsumer{ctx: p.ctx, inference: p}
	return s.values(want, got)
}

// Normalizing a structural predicate can forget the exact construction
// inventories of its nested fields. Recover those checked member descriptions
// for packet and singleton proofs, without closing an arbitrary input record or a local
// binding whose context deliberately permits refinement.
func (p *inference) constructorEvidence(value adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	v, ok := value.(*adt.Vertex)
	if !ok || p.projections[v] == nil || (v.Kind() == adt.StructKind && !v.IsData()) {
		return value
	}
	out := v.ToDataSingle()
	out.Arcs = nil
	for _, field := range v.Arcs {
		member := p.projections[v][field.Label]
		if member == nil {
			out.Arcs = append(out.Arcs, field)
			continue
		}
		member = p.constructorEvidence(member)
		if member == nil {
			return nil
		}
		var arc *adt.Vertex
		if vertex, ok := member.(*adt.Vertex); ok {
			// Keep the member's arcs on the field itself. A BaseValue-only
			// wrapper would hide its fields from structural inclusion.
			copy := *vertex
			copy.Label, copy.ArcType = field.Label, field.ArcType
			arc = &copy
		} else {
			arc = field.ToDataSingle()
			arc.BaseValue, arc.Arcs = member, nil
		}
		out.Arcs = append(out.Arcs, arc)
	}
	p.constructors[out] = p.constructors[v]
	p.projections[out] = p.projections[v]
	p.memberships[out] = p.memberships[v]
	return out
}

func (p *inference) frame(up *adt.Environment, values map[adt.Feature]adt.Value) *adt.Environment {
	// The proof frame is never passed to the runtime evaluator as a packet.
	// References to these fields are synthesized by expr below.
	env := &adt.Environment{Up: up, Vertex: &adt.Vertex{BaseValue: &adt.StructMarker{}}}
	p.scopes[env] = &proofScope{values: values, active: make(map[adt.Feature]bool)}
	return env
}

// Bounds on captured values need not be materialized yet. Named imports supply
// conditional hypotheses, while supplied implementations require their own
// proofs. Concrete closure validation separately discharges these links and
// requires every runtime capture to be complete.
func (p *inference) captured(v adt.Value) adt.Value {
	if !p.captureType(v, make(map[adt.Value]bool)) {
		return nil
	}
	switch x := adt.Unwrap(v).(type) {
	case *adt.FuncValue, *adt.Builtin:
		return x.(adt.Value)
	}
	if vertex, ok := v.(*adt.Vertex); ok && vertex.Kind()&(adt.StructKind|adt.ListKind) != 0 && adt.Validate(p.ctx, vertex,
		&adt.ValidateConfig{Runtime: true, Concrete: true}) == nil {
		// A complete supplied value has a concrete inventory. Its evaluated
		// predicate may remain open for later CUE refinement, but that does
		// not add fields to the runtime value supplied by this packet.
		return capturedInventory(vertex, make(map[*adt.Vertex]*adt.Vertex))
	}
	return v
}

func capturedInventory(v *adt.Vertex, seen map[*adt.Vertex]*adt.Vertex) *adt.Vertex {
	v = v.DerefValue()
	if out := seen[v]; out != nil {
		return out
	}
	out := v.ToDataSingle()
	seen[v] = out
	out.Arcs = slices.Clone(v.Arcs)
	for i, field := range out.Arcs {
		member := *capturedInventory(field, seen)
		member.Label, member.ArcType = field.Label, field.ArcType
		out.Arcs[i] = &member
	}
	return out
}

func (p *inference) captureType(v adt.Value, seen map[adt.Value]bool) bool {
	if v == nil || !p.step() {
		return false
	}
	if seen[v] {
		return true
	}
	seen[v] = true
	switch x := adt.Unwrap(v).(type) {
	case *adt.Bottom:
		// A refuted data hypothesis is not a value. Only a checked source
		// operation can introduce a failing computation into the proof.
		return false
	case *adt.Builtin:
		return ValidateBuiltin(p.ctx, x) == nil
	case *adt.FuncValue:
		if x.Fn.Body == nil {
			p.assume(x, make(map[adt.Value]bool))
			return true
		}
		return p.implementation(x)
	case *adt.Conjunction:
		for _, term := range x.Values {
			if !p.captureType(term, seen) {
				return false
			}
		}
	case *adt.Disjunction:
		for _, branch := range x.Values {
			if !p.captureType(branch, seen) {
				return false
			}
		}
	}
	if vertex, ok := v.(*adt.Vertex); ok {
		cfg := &adt.ValidateConfig{Runtime: true}

		if adt.Validate(p.ctx, vertex, cfg) != nil {
			return false
		}
		for _, field := range vertex.Arcs {
			if field.Label.IsDef() || field.Label.IsLet() || field.ArcType == adt.ArcOptional {
				continue
			}
			if !p.captureType(field, seen) {
				return false
			}
		}
	}
	return true
}

func (p *inference) implementation(f *adt.FuncValue) bool {
	clauses := f.Obligations()
	if p.recursing(f) {
		return p.recursiveContracts(f, clauses)
	}
	savedSupport := p.support
	p.support = &assumptionStore{parent: savedSupport}
	defer func() { p.support = savedSupport }()
	saved := p.hypotheses
	p.hypotheses = maps.Clone(saved)
	p.hypotheses[f] = true
	p.implementing = append(p.implementing, f)
	defer func() {
		p.hypotheses = saved
		p.implementing = p.implementing[:len(p.implementing)-1]
	}()
	for _, target := range clauses {
		if !p.function(f, target) {
			return false
		}
	}
	return true
}

func (p *inference) recursing(f *adt.FuncValue) bool {
	for _, active := range p.active {
		if active.Fn == f.Fn {
			if same, known := adt.SameFunctionInstance(p.ctx, active, f); same || !known {
				return true
			}
		}
	}
	return false
}

// The fixed-point rule is a partial-correctness rule, not a termination
// certificate. Each declaration in a recursive dependency must still pass
// its body proof. A recursive reference can use only contracts implied by
// that declaration, with independently established runtime identity.
func (p *inference) recursiveContracts(f *adt.FuncValue, targets []adt.FuncType) bool {
	for _, active := range p.implementing {
		if active.Fn != f.Fn {
			continue
		}
		if same, known := adt.SameFunctionInstance(p.ctx, active, f); !same || !known {
			continue
		}
		s := &subsumer{ctx: p.ctx}
		for _, target := range targets {
			proved := false
			for _, source := range active.Obligations() {
				if !p.step() {
					return false
				}
				// Residual protocols require their own bound-slot evidence.
				if target.Partial() == source.Partial() && s.capabilitySignature(target, source) {
					proved = true
					break
				}
			}
			if !proved {
				return false
			}
		}
		p.useHypothesis(active)
		return true
	}
	return false
}

// Saved arguments require their own implementation proofs and admission by
// one executable view. The original body proof still introduces arbitrary
// admitted arguments; these values cannot specialize away an obligation.
func (p *inference) savedPacket(f *adt.FuncValue) bool {
	if !f.IsPartial() {
		return true
	}
	values := make([]adt.Value, len(f.Fn.Params))
	for i := range f.Fn.Params {
		env, expr := f.BoundArgument(i)
		if expr == nil {
			continue
		}
		v := p.boundArgument(env, expr)
		if v == nil {
			return false
		}
		values[i] = v
	}
	for _, instance := range f.BoundArgumentInstances(p.ctx) {
		admitted := true
		for i, value := range values {
			if value == nil {
				continue
			}
			want := p.schema(instance.Env, instance.Fn.Params[i].Value)
			if want == nil || !(&subsumer{ctx: p.ctx, inference: p}).packetMember(want, value) {
				admitted = false
				break
			}
		}
		if admitted {
			return true
		}
	}
	return false
}

// A saved source constructor has an exact inventory. Evaluating it as a
// predicate would permit additional fields and lose evidence of an absent
// optional argument field. Other saved values retain their capture checks.
func (p *inference) boundArgument(env *adt.Environment, expr adt.Expr) adt.Value {
	if v, ok := expr.(*adt.Vertex); ok && p.constructors[v] != nil {
		// Symbolic partial application has already checked this constructor.
		// Preserve its source description as well as its exact inventory;
		// packet completion may add fields unless the source closed them.
		return v
	}
	switch expr.(type) {
	case *adt.StructLit, *adt.ListLit:
		return p.expr(env, expr)
	}
	return p.captured(p.schema(env, expr))
}

func (p *inference) deriveFunction(f *adt.FuncValue, target adt.FuncType) (proved bool) {
	if !p.step() || f.Fn.Body == nil || len(p.active) >= 256 {
		return false
	}
	if target.Fn.Body != nil && target.Fn != f.Fn {
		// A concrete identity conjunct carries its own source contract.
		// Conjunction can retain another view of that descriptor. Prove the
		// unchanged body only after establishing descriptor identity;
		// an attached bodyless annotation cannot supply this evidence.
		original := target.Inhabitant()
		if original == nil {
			original = &adt.FuncValue{Fn: target.Fn, Src: target.Fn.Src, Env: target.Env}
		}
		if same, known := adt.SameFunctionInstance(p.ctx, f, original); same && known {
			return p.function(original, target)
		}
	}
	if p.recursing(f) {
		return p.recursiveContracts(f, []adt.FuncType{target})
	}
	if f.Src != nil && (f.Src.Extern.IsValid() || f.Src.Effect != nil) {
		return false
	}
	key := proofKey{f, target}
	if certificate, ok := p.completed[key]; ok {
		available := true
		for _, h := range certificate.required {
			available = available && p.hypotheses[h]
		}
		if available {
			for _, h := range certificate.required {
				p.useHypothesis(h)
			}
			return true
		}
	}
	attempt := &proofAttempt{inherited: p.hypotheses, required: make(map[*adt.FuncValue]bool)}
	p.attempts = append(p.attempts, attempt)
	var result adt.Value
	defer func() {
		p.attempts = p.attempts[:len(p.attempts)-1]
		if proved {
			required := make([]*adt.FuncValue, 0, len(attempt.required))
			for h := range attempt.required {
				required = append(required, h)
			}
			p.completed[key] = proofCertificate{required: required, result: result}
		}
	}()
	savedSupport := p.support
	p.support = &assumptionStore{parent: savedSupport}
	defer func() { p.support = savedSupport }()
	savedHypotheses := p.hypotheses
	p.hypotheses = maps.Clone(p.hypotheses)
	defer func() { p.hypotheses = savedHypotheses }()
	p.active = append(p.active, f)
	defer func() { p.active = p.active[:len(p.active)-1] }()
	if !p.savedPacket(f) {
		return false
	}
	s := &subsumer{ctx: p.ctx}
	source := adt.FuncType{Fn: f.Fn, Env: f.Env}
	if len(adt.FunctionTypeParameters(target)) != 0 {
		// Selection changes the next executable view, not the source of a
		// retained universal proof. Another declaration can have its own
		// telescope, so checking only target.Fn == f.Fn below is insufficient.
		// Recover this implementation's declaration scope before opening
		// the two telescopes with shared rigid variables.
		for _, original := range f.Obligations() {
			if original.Fn == source.Fn &&
				len(adt.FunctionTypeParameters(original)) == len(adt.FunctionTypeParameters(target)) {
				source.Env = original.Env
				break
			}
		}
	}
	partial := target.Partial()
	if partial == nil && f.IsPartial() && target.Fn != f.Fn && !slices.Contains(f.Obligations(), target) {
		// A new contract, including a boundary's callback interface,
		// describes the residual packet of the supplied closure. Retained
		// original clauses still describe its full packet; their meaning
		// must not change merely because arguments have since been saved.
		partial = f
	}
	if partial != nil {
		source.Fn = partial.ResidualSignature()
	}
	if target.Fn == f.Fn {
		// Retained views of the same erased implementation include its
		// original telescope, even after selecting concrete type arguments.
		// Check each such obligation in its own type scope. Runtime capture
		// equality is checked independently by the closure identity rules.
		source.Env = target.Env
	}
	if len(adt.FunctionTypeParameters(source)) == 0 {
		// Prove a universally constrained monomorphic implementation under
		// fresh rigid inputs. This checks its body; it does not generalize
		// a monomorphic callback from an annotation alone.
		for _, param := range adt.FunctionTypeParameters(target) {
			bound := p.schema(adt.TypeParameterScope(target.Env, param), param.Bound)
			if bound == nil {
				return false
			}
			target = adt.BindFunctionTypes(target, []adt.Value{&adt.RigidType{Param: param, Bound: bound}})
		}
	}

	target, source, ok := s.capabilityScopes(target, source)
	if !ok {
		return false
	}
	target, ok = s.completeProtocol(target, source)
	if !ok {
		return false
	}
	// Prove coverage without using the implementation's result annotation.
	a, b := *target.Fn, *source.Fn
	a.Ret, b.Ret = nil, nil
	coverage := p.coverage(adt.FuncType{Fn: &a, Env: target.Env}, adt.FuncType{Fn: &b, Env: source.Env})
	if coverage.State != adt.Established {
		if proof, ok := coverage.Support.(*refutation); ok {
			p.refutations[key] = proof
		}
		return false
	}
	matches := adt.MatchFuncValueParams(target.Fn, &adt.FuncValue{Fn: source.Fn})
	values := make(map[adt.Feature]adt.Value)
	optional := make(map[adt.Feature]adt.Value)
	absent := make(map[adt.Feature]bool)
	if partial != nil {
		for i, arg := range f.Fn.Params {
			env, expr := partial.BoundArgument(i)
			if expr == nil {
				continue
			}
			v := p.boundArgument(env, expr)
			want := p.schema(source.Env, arg.Value)
			if want == nil || v == nil || !(&subsumer{ctx: p.ctx, inference: p}).packetMember(want, v) {
				return false
			}
			values[arg.Local] = p.sourceMeet(v, want)
		}
	}
	for i, j := range matches {
		arg := target.Fn.Params[i]
		v := p.schema(target.Env, arg.Value)
		if v == nil {
			return false
		}
		if arg.ArcType == adt.ArcOptional && arg.Default == nil {
			q := source.Fn.Params[j]
			if q.Default == nil {
				// Keep its declared type without assuming presence. An
				// occurrence test can introduce this hypothesis in its branch.
				optional[q.Local] = v
				p.assume(v, make(map[adt.Value]bool))
				continue
			}
			defaultValue := p.expr(source.Env, q.Default)
			if !p.includes(p.schema(source.Env, q.Value), defaultValue) {
				return false
			}
			// Omission uses the implementation's own default. A supplied
			// argument still ranges over the full advertised optional domain.
			v = proofUnion([]adt.Value{v, defaultValue})
		}
		if arg.Default != nil {
			// Omission runs the implementation's default, checked against
			// its own parameter description. A narrower attached domain
			// constrains supplied arguments, not that private default.
			q := source.Fn.Params[j]
			if q.Default == nil {
				return false
			}
			defaultValue := p.expr(source.Env, q.Default)
			if !p.includes(p.schema(source.Env, q.Value), defaultValue) {
				return false
			}
			v = proofUnion([]adt.Value{v, defaultValue})
		}
		values[source.Fn.Params[j].Local] = v
		p.assume(v, make(map[adt.Value]bool))
	}
	for _, arg := range source.Fn.Params {
		if arg.ArcType == adt.ArcOptional && arg.Default == nil && optional[arg.Local] == nil {
			v := values[arg.Local]
			if v == nil {
				v = p.schema(source.Env, arg.Value)
				absent[arg.Local] = true
			}
			if v == nil {
				return false
			}
			optional[arg.Local] = v
			p.assume(v, make(map[adt.Value]bool))
		}
		if _, ok := values[arg.Local]; ok {
			continue
		}
		if arg.Default != nil {
			v := p.expr(source.Env, arg.Default)
			if !p.includes(p.schema(source.Env, arg.Value), v) {
				return false
			}
			values[arg.Local] = v
		}
	}
	env := p.frame(source.Env, values)
	p.scopes[env].optional, p.scopes[env].absent = optional, absent
	body := p.expr(env, source.Fn.Body)
	want := p.schema(target.Env, target.Fn.Ret)
	if !p.includes(want, body) {
		if !f.IsPartial() {
			if proof := p.resultRefutation(source, target, want); proof != nil {
				p.refutations[key] = proof
			}
		}
		return false
	}
	result = body
	return true
}

// An admitted callback packet or the result of a checked call introduces a
// conformance hypothesis. Ordinary named functions must have their bodies
// checked before a call can use their result annotations. This prevents
// circular annotation proofs.
func (p *inference) assume(v adt.Value, seen map[adt.Value]bool) {
	if v == nil || seen[v] {
		return
	}
	seen[v] = true
	switch x := adt.Unwrap(v).(type) {
	case *adt.Disjunction:
		for _, branch := range x.Values {
			p.assume(branch, seen)
		}
	case *adt.Conjunction:
		for _, term := range x.Values {
			p.assume(term, seen)
		}
	}
	if f, ok := adt.Unwrap(v).(*adt.FuncValue); ok {
		if f.Fn.Body == nil {
			p.hypotheses[f] = true
		}
		return
	}
	if v, ok := v.(*adt.Vertex); ok {
		for _, a := range v.Arcs {
			// Definitions describe predicates, and optional fields need not
			// exist. Neither supplies an executable callback hypothesis.
			if !a.Label.IsDef() && !a.Label.IsLet() && a.ArcType != adt.ArcOptional {
				p.assume(a, seen)
			}
		}
	}
}

func (p *inference) expr(env *adt.Environment, expr adt.Expr) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := expr.(type) {
	case *adt.Null, *adt.Bool, *adt.Num, *adt.String, *adt.Bytes:
		return x.(adt.Value)
	case *adt.Top, *adt.BasicType, *adt.BoundValue:
		return x.(adt.Value)
	case *adt.DisjunctionExpr:
		var alternatives []adt.Value
		for _, branch := range x.Values {
			v := p.expr(env, branch.Val)
			if v == nil {
				return nil
			}
			alternatives = append(alternatives, v)
		}
		// A preference does not remove an incoming type alternative.
		return proofUnion(alternatives)
	case *adt.Bottom:
		if !x.IsIncomplete() {
			return x
		}
		return nil
	case *adt.BoundExpr:
		v := p.expr(env, x.Expr)
		if v == nil {
			return nil
		}
		if refuted(v) {
			return v
		}
		switch x.Op {
		case adt.LessThanOp, adt.LessEqualOp, adt.GreaterThanOp, adt.GreaterEqualOp:
			if v.Kind()&adt.NumberKind != v.Kind() && v.Kind() != adt.StringKind && v.Kind() != adt.BytesKind {
				return nil
			}
		case adt.MatchOp, adt.NotMatchOp:
			if v.Kind() != adt.StringKind && v.Kind() != adt.BytesKind {
				return nil
			}
		case adt.NotEqualOp:
			if v.Kind()&(adt.NumberKind|adt.StringKind|adt.BytesKind|adt.BoolKind|adt.NullKind|adt.StructKind|adt.ListKind) != v.Kind() {
				return nil
			}
		default:
			return nil
		}
		if adt.IsConcrete(v) && v.Kind()&(adt.StructKind|adt.ListKind) == 0 {
			return p.schema(nil, &adt.BoundExpr{Src: x.Src, Op: x.Op, Expr: v})
		}
		kind := (&adt.BoundValue{Op: x.Op, Value: v}).Kind()
		switch kind {
		case adt.StructKind:
			return p.schema(nil, &adt.StructLit{})
		case adt.ListKind:
			return p.schema(nil, &adt.ListLit{Elems: []adt.Elem{&adt.Ellipsis{}}})
		}
		return &adt.BasicType{K: kind}
	case *adt.Builtin:
		return x
	case *adt.LabelReference:
		// A lexical label capture is supplied by its field declaration.
		// An unmaterialized pattern label still has the string kind.
		return p.schema(env, x)
	case *adt.ImportReference:
		// An import is a namespace, not one runtime record to materialize.
		// Check the selected declaration as a capture below: unrelated
		// package schemas can contain intentionally failing computations.
		return p.schema(env, x)
	case *adt.AliasApplication:
		args := make([]adt.Value, len(x.Args))
		for i, argument := range x.Args {
			if !p.typeOperations(env, argument) {
				return nil
			}
			args[i] = p.schema(env, argument)
			if args[i] == nil {
				return nil
			}
		}
		scope, b := x.Expand(p.ctx, env, args)
		if b != nil {
			return nil
		}
		return p.expr(scope, x.Template.Body)
	case *adt.LiveReference:
		// Predicate operations read the same coordinate as ordinary source
		// expressions. Its live wrapper is a proof dependency, not a new
		// operation with a separate evaluation phase.
		return p.expr(env, x.X)
	case *adt.TypeReference:
		// Runtime erasure checking has already excluded arbitrary type
		// variables here. An alias may still substitute a fixed literal
		// used as an ordinary data index or constructor constraint.
		v := p.schema(env, x)
		if _, arbitrary := adt.Unwrap(v).(*adt.RigidType); arbitrary {
			return nil
		}
		return v
	case *adt.LetReference:
		e := env
		for range x.UpCount {
			if e == nil {
				return nil
			}
			e = e.Up
		}
		if scope := p.scopes[e]; scope != nil && !x.IsPredicate {
			return p.expr(e, &adt.FieldReference{Label: x.Label})
		}
		return p.expr(e, x.X)
	case *adt.FieldReference:
		e := env
		for range x.UpCount {
			if e == nil {
				return nil
			}
			e = e.Up
		}
		if scope := p.scopes[e]; scope != nil {
			if scope.presence[x.Label] == adt.ArcOptional {
				return nil
			}
			if v := scope.values[x.Label]; v != nil {
				if scope.fields[x.Label] != nil {
					return p.bindingDescription(v)
				}
				return v
			}
			if scope.active[x.Label] {
				if ref, ok := scope.fields[x.Label].(*adt.FieldReference); ok &&
					ref.UpCount == 0 && ref.Label == x.Label {
					// A reflexive data equation adds no constraint. Its
					// description is top, not an assumed concrete value or
					// a hypothesis for arbitrary recursive computations.
					return &adt.Top{}
				}
				return nil
			}
			if field := scope.fields[x.Label]; field != nil {
				scope.active[x.Label] = true
				var v adt.Value
				var function *adt.FuncValue
				bindings := scope.bindings[x.Label]
				if len(bindings) == 1 {
					e = bindings[0].env
				}
				if len(bindings) <= 1 {
					function = p.functionBinding(proofBinding{e, field}, make(map[proofBinding]bool))
				}
				if len(bindings) > 1 {
					v = p.bindingMeet(bindings)
				} else if function != nil {
					// Tie only annotated function bindings before checking
					// their bodies. Ordinary cyclic data is never a typing
					// hypothesis. Failed bodies remove the temporary binding.
					scope.values[x.Label] = function
					if p.implementation(function) {
						// A projected implementation still has to check all the
						// source terms in its containing record expression.
						switch field.(type) {
						case *adt.Function, *adt.Quantified:
							v = function
						default:
							v = p.expr(e, field)
						}
					}
				} else {
					v = p.expr(e, field)
				}
				delete(scope.active, x.Label)
				scope.values[x.Label] = v
				return p.bindingDescription(v)
			}
			return nil
		}
		if e == nil {
			return nil
		}
		field := p.fieldDescription(e.DerefVertex(p.ctx), x.Label)
		if field == nil {
			return nil
		}
		value := p.captured(field)
		if value != nil && !adt.IsConcrete(value) && !packetHasFunction(value, make(map[adt.Value]bool)) {
			return &adt.LiveType{Ref: &adt.LiveReference{X: x}, Env: env, Upper: value}
		}
		if value != nil && value.Kind()&(adt.StructKind|adt.ListKind) != 0 && !packetHasFunction(value, make(map[adt.Value]bool)) {
			p.memberships[value] = append(p.memberships[value], &adt.LiveType{Ref: &adt.LiveReference{X: x}, Env: env, Upper: value})
		}
		return value
	case *adt.SelectorExpr:
		v := p.project(p.expr(env, x.X), x.Sel)
		if _, imported := x.X.(*adt.ImportReference); imported {
			return p.captured(v)
		}
		return v
	case *adt.IndexExpr:
		v := p.expr(env, x.X)
		composite, _ := v.(*adt.Vertex)
		if v != nil && x.Quantified && (v.Kind() == adt.FuncKind || composite != nil && composite.HasSubjectSchemes()) {
			index := x.TypeIndex
			if index == nil {
				index = x.Index
			}
			if !p.typeOperations(env, index) {
				return nil
			}
			argument := p.schema(env, index)
			if argument == nil {
				return nil
			}
			if v.Kind() == adt.FuncKind {
				return p.selectType(v, argument)
			}
			selection := *x
			selection.X, selection.Index, selection.TypeIndex = v, argument, argument
			return p.schema(nil, &selection)
		}
		if x.ErasedRuntimeIndex(env) {
			p.failure = &adt.Bottom{Src: x.Source(), Code: adt.BlockedError,
				Err: p.ctx.NewPosf(adt.Pos(x), "erased type parameter cannot be used as a runtime index")}
			return nil
		}
		return p.index(v, p.expr(env, x.Index))
	case *adt.Function:
		// Use the source descriptor, including its explicit interface root.
		// A local constructor still needs an implementation. Conditional
		// import hypotheses enter through their named capture bindings.
		f, ok := adt.Unwrap(p.schema(env, x)).(*adt.FuncValue)
		if !ok || !p.implementation(f) {
			return nil
		}
		return f
	case *adt.Quantified:
		if _, function := x.Body.(*adt.Function); function {
			// Creating the lexical telescope does not execute the body.
			v := p.schema(env, x)
			f, ok := adt.Unwrap(v).(*adt.FuncValue)
			if !ok || !p.implementation(f) {
				return nil
			}
			return f
		}

		scope := x.CheckingScope(p.ctx, env)
		for _, param := range x.Params {

			bound := p.schema(scope, param.Bound)
			if bound == nil {
				return nil
			}
			scope = adt.BindFunctionTypes(adt.FuncType{Env: scope},
				[]adt.Value{&adt.RigidType{Param: param, Bound: bound}}).Env
		}
		if p.expr(scope, x.Body) == nil {
			return nil
		}
		// Retain the source telescope after proving its arbitrary instance.
		// The rigid variables belong only to this introduction proof. The
		// ordinary quantifier normalization distributes over fixed record
		// projections and retains non-distributive Boolean predicates.
		return p.schema(env, x)
	case *adt.SliceExpr:
		return p.slice(env, x)

	case *adt.StructLit:
		if value, handled := p.recordMeet([]proofBinding{{env, x}}); handled {
			return value
		}
		e := p.frame(env, make(map[adt.Feature]adt.Value))
		if len(x.Decls) == 1 {
			if embedded, ok := x.Decls[0].(adt.Expr); ok {
				return p.expr(e, embedded)
			}
		}
		scope := p.scopes[e]
		scope.fields = make(map[adt.Feature]adt.Expr)
		optional := make(map[adt.Feature]adt.Expr)
		var conditions []*adt.Comprehension
		for _, decl := range x.Decls {
			if _, ok := decl.(*adt.BulkOptionalField); ok {
				continue
			}
			if comp, ok := decl.(*adt.Comprehension); ok {
				conditions = append(conditions, comp)
				continue
			}
			if let, ok := decl.(*adt.LetField); ok {
				scope.fields[let.Label] = let.Value
				continue
			}
			f, ok := decl.(*adt.Field)
			if !ok || (f.ArcType != adt.ArcMember && f.ArcType != adt.ArcOptional) {
				return nil
			}
			if scope.fields[f.Label] != nil || optional[f.Label] != nil {
				return nil
			}
			if f.ArcType == adt.ArcOptional {
				optional[f.Label] = f.Value
			} else {
				scope.fields[f.Label] = f.Value
			}
		}
		out := &adt.StructLit{}
		for _, decl := range x.Decls {
			if pattern, ok := decl.(*adt.BulkOptionalField); ok {
				filter := p.expr(e, pattern.Filter)
				value := p.expr(e, pattern.Value)
				if filter == nil || filter.Kind() != adt.StringKind || value == nil {
					return nil
				}
				var constraint adt.Expr = value
				if label, ok := pattern.Value.(*adt.LabelReference); ok && label.UpCount == 0 {
					// Preserve a constraint equating the field with its label.
					// The pattern introduces the same lexical label on replay.
					constraint = label
				}
				out.Decls = append(out.Decls, &adt.BulkOptionalField{
					Src: pattern.Src, Filter: filter, Value: constraint, Label: pattern.Label,
				})
				continue
			}
			if _, ok := decl.(*adt.Comprehension); ok {
				continue
			}
			if let, ok := decl.(*adt.LetField); ok {
				// Every present source term is checked, even when a let is
				// unused by the returned fields.
				if p.expr(e, &adt.FieldReference{Label: let.Label}) == nil {
					return nil
				}
				continue
			}
			f := decl.(*adt.Field)
			var v adt.Value
			if f.ArcType == adt.ArcOptional {
				// An optional constraint is checked without introducing a
				// present value that an ordinary field reference could read.
				v = p.expr(e, f.Value)
			} else {
				v = p.expr(e, &adt.FieldReference{Label: f.Label})
			}
			if v == nil {
				return nil
			}
			out.Decls = append(out.Decls, &adt.Field{Label: f.Label, ArcType: f.ArcType, Value: v})
		}
		if len(conditions) != 0 {
			return p.conditionalRecord(e, out, conditions)
		}
		v := p.schema(nil, out)
		if v, ok := v.(*adt.Vertex); ok {
			if len(optional) != 0 {
				// Keep optional constraints and their possible presence. They
				// do not establish an exact singleton constructor inventory.
				p.projections[v] = scope.values
				return v
			}
			// A constructed runtime record has exactly these fields, even
			// though their symbolic values describe many possible packets.
			summary := v.ToDataSingle()
			summary.ClosedNonRecursive = true
			p.projections[summary] = scope.values
			p.constructors[summary] = out
			return summary
		}
		return v
	case *adt.ListLit:
		env = p.frame(env, make(map[adt.Feature]adt.Value))
		out := &adt.ListLit{}
		var elements []adt.Value
		variable := false
		for _, elem := range x.Elems {
			if comp, ok := elem.(*adt.Comprehension); ok {
				variable = true
				v, empty := p.comprehension(env, comp)
				if empty {
					continue
				}
				if v == nil {
					return nil
				}
				elements = append(elements, v)
				continue
			}
			e, ok := elem.(adt.Expr)
			if !ok {
				return nil
			}
			v := p.expr(env, e)
			if v == nil {
				return nil
			}
			out.Elems = append(out.Elems, v)
			elements = append(elements, v)
		}
		if variable {
			out.Elems = nil
			if len(elements) != 0 {
				out.Elems = append(out.Elems, &adt.Ellipsis{Value: proofUnion(elements)})
			}
		}
		result := p.schema(nil, out)
		if v, ok := result.(*adt.Vertex); ok {
			p.constructors[v] = out
		}
		if v, ok := result.(*adt.Vertex); ok && !variable {
			fields := make(map[adt.Feature]adt.Value)
			for i, value := range elements {
				fields[adt.MakeIntLabel(adt.IntLabel, int64(i))] = value
			}
			p.projections[v] = fields
		}
		return result
	case *adt.Interpolation:
		for _, part := range x.Parts {
			v := p.expr(env, part)
			if v == nil || v.Kind()&(adt.NumberKind|adt.StringKind|adt.BytesKind|adt.BoolKind) != v.Kind() {
				return nil
			}
		}
		return &adt.BasicType{K: x.K}
	case *adt.UnaryExpr:
		v := p.expr(env, x.X)
		if v == nil {
			return nil
		}
		switch x.Op {
		case adt.NotOp:
			if v.Kind() == adt.BoolKind {
				if b, ok := adt.Unwrap(v).(*adt.Bool); ok {
					return &adt.Bool{B: !b.B}
				}
				return &adt.BasicType{K: adt.BoolKind}
			}
		case adt.AddOp, adt.SubtractOp:
			if v.Kind()&adt.NumberKind == v.Kind() {
				if x.Op == adt.AddOp {
					return v
				}
				if result := p.negateNumber(v); result != nil {
					return p.schema(nil, result)
				}
			}
		}
		return nil
	case *adt.BinaryExpr:
		if x.Op == adt.AndOp {
			if value, handled := p.recordMeet([]proofBinding{{env, x}}); handled {
				return value
			}
			if a, annotation := p.functionAnnotation(env, x.X); annotation {
				return p.assertFunction(p.expr(env, x.Y), a)
			}
			if b, annotation := p.functionAnnotation(env, x.Y); annotation {
				return p.assertFunction(p.expr(env, x.X), b)
			}
		}
		a, b := p.expr(env, x.X), p.expr(env, x.Y)
		if a == nil || b == nil {
			return nil
		}
		return p.binary(x, a, b)

	case *adt.CallExpr:
		return p.call(env, x)
	case adt.Value:
		// Internal calls can carry an already evaluated argument across a
		// representation boundary. Retain its own capture and conformance
		// checks instead of treating it as a new source constructor.
		return p.captured(x)
	}
	return nil
}

// Type arguments can contain computations as well as finite type syntax.
// Check those operations before normalization: an ill-typed operation must
// not become an empty predicate that an unused alias argument can conceal.
// Literal function bodies have their own parameter scopes and are checked by
// the implementation rule even if the abbreviation never uses its argument.
func (p *inference) typeOperations(env *adt.Environment, expr adt.Expr) bool {
	ok := true
	seen := make(map[adt.Node]bool)
	var visitor walk.Visitor
	visitor.Before = func(node adt.Node) bool {
		if node == nil || seen[node] || !ok {
			return false
		}
		seen[node] = true
		if !p.step() {
			ok = false
			return false
		}
		switch x := node.(type) {
		case *adt.AliasApplication:
			args := make([]adt.Value, len(x.Args))
			for i, argument := range x.Args {
				if !p.typeOperations(env, argument) {
					ok = false
					return false
				}
				args[i] = p.schema(env, argument)
				if args[i] == nil {
					ok = false
					return false
				}
			}
			scope, b := x.Expand(p.ctx, env, args)
			ok = b == nil && p.typeOperations(scope, x.Template.Body)
			return false
		case *adt.Quantified:
			scope := x.CheckingScope(p.ctx, env)
			for _, param := range x.Params {

				if param.Bound != nil && !p.typeOperations(scope, param.Bound) {
					ok = false
					return false
				}
				bound := p.schema(scope, param.Bound)
				if bound == nil {
					ok = false
					return false
				}
				scope = adt.BindFunctionTypes(adt.FuncType{Env: scope},
					[]adt.Value{&adt.RigidType{Param: param, Bound: bound}}).Env
			}
			ok = p.typeOperations(scope, x.Body)
			return false
		case *adt.StructLit:
			scope := p.frame(env, make(map[adt.Feature]adt.Value))
			p.scopes[scope].fields = make(map[adt.Feature]adt.Expr)
			for _, decl := range x.Decls {
				switch field := decl.(type) {
				case *adt.Field:
					p.scopes[scope].fields[field.Label] = field.Value
				case *adt.LetField:
					p.scopes[scope].fields[field.Label] = field.Value
				}
			}
			for _, decl := range x.Decls {
				switch field := decl.(type) {
				case *adt.Field:
					ok = p.typeOperations(scope, field.Value)
				case *adt.LetField:
					ok = p.typeOperations(scope, field.Value)
				case adt.Expr:
					ok = p.typeOperations(scope, field)
				default:
					ok = false
				}
				if !ok {
					break
				}
			}
			return false
		case *adt.ListLit:
			scope := p.frame(env, make(map[adt.Feature]adt.Value))
			for _, elem := range x.Elems {
				switch item := elem.(type) {
				case *adt.Ellipsis:
					ok = item.Value == nil || p.typeOperations(scope, item.Value)
				case adt.Expr:
					ok = p.typeOperations(scope, item)
				default:
					ok = false
				}
				if !ok {
					break
				}
			}
			return false
		case *adt.Function:
			for _, param := range x.Params {
				visitor.Elem(param.Value)
				visitor.Elem(param.Default)
			}
			visitor.Elem(x.Ret)
			if ok && x.Body != nil {
				ok = p.implementation(&adt.FuncValue{Fn: x, Src: x.Src, Env: env})
			}
			return false
		case *adt.LetReference:
			e := env
			for range x.UpCount {
				if e == nil {
					ok = false
					return false
				}
				e = e.Up
			}
			ok = p.typeOperations(e, x.X)
			return false
		case *adt.BinaryExpr:
			if x.Op == adt.AndOp {
				return true
			}
			ok = p.expr(env, x) != nil
			return false
		case *adt.UnaryExpr, *adt.BoundExpr, *adt.CallExpr, *adt.IndexExpr, *adt.SliceExpr, *adt.SelectorExpr:
			ok = p.expr(env, x.(adt.Expr)) != nil
			return false
		}
		return true
	}
	visitor.Elem(expr)
	return ok
}

// Run the shared eager service on an intersection independently of whether
// its operands are concrete. A proof may retain an unresolved conjunction,
// but it must preserve an established empty completion predicate. Each source
// operand must have its own derivation before this operation is used.
func (p *inference) eagerMeet(a, b adt.Value) adt.Value {
	if !p.step() || a == nil || b == nil {
		return nil
	}
	v := &adt.Vertex{}
	v.AddConjunct(adt.MakeRootConjunct(nil, a))
	v.AddConjunct(adt.MakeRootConjunct(nil, b))
	v.Finalize(p.ctx)
	return v
}

// Type elimination distributes through alternative subjects. For a
// conjunction it uses the clauses admitting the type argument, retaining
// all of their consequences rather than arbitrarily selecting one clause.
func (p *inference) selectType(value, argument adt.Value) adt.Value {
	if !p.step() || argument == nil {
		return nil
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.Disjunction:
		var results []adt.Value
		for _, branch := range x.Values {
			result := p.selectType(branch, argument)
			if result == nil {
				return nil
			}
			results = append(results, result)
		}
		return proofUnion(results)
	case *adt.Conjunction:
		var results []adt.Value
		for _, term := range x.Values {
			if result := p.selectType(term, argument); result != nil {
				results = append(results, result)
			}
		}
		return p.meetResults(results)
	case *adt.FuncValue:
		inst, b := adt.SelectFunctionType(p.ctx, x, argument)
		if b != nil || inst == nil {
			return nil
		}
		if p.hypotheses[x] {
			p.useHypothesis(x)
			p.hypotheses[inst] = true
		}
		return inst
	}
	return nil
}

// A projection must be justified in every incoming shape. Width subtyping
// does not make an undeclared field accessible. List indexes instead denote
// partial computations: an out-of-range index fails, and a homogeneous tail
// constrains any successful selection without promising that it is present.
func (p *inference) project(value adt.Value, label adt.Feature) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.Bottom:
		// A checked computation with no successful result needs no field
		// witness. This rule does not add fields to a surviving shape;
		// every source operand was checked before producing this bottom.
		if !x.IsIncomplete() {
			return x
		}
		return nil
	case *adt.RigidType:
		// A bound supplies a field inventory and the types of those fields,
		// without identifying the arbitrary record with its upper bound.
		return p.project(x.Bound, label)
	case *adt.LiveType:
		return p.project(x.Upper, label)
	}
	if union, ok := adt.Unwrap(value).(*adt.Disjunction); ok {
		var alternatives []adt.Value
		for _, branch := range union.Values {
			v := p.project(branch, label)
			if v == nil {
				return nil
			}
			alternatives = append(alternatives, v)
		}
		return proofUnion(alternatives)
	}
	v, ok := value.(*adt.Vertex)
	if !ok {
		return nil
	}
	if fields := p.projections[v]; fields != nil {
		if field := fields[label]; field != nil {
			return field
		}
	}
	// A transported field may share its structural value through BaseValue.
	// Its inventory belongs to that value, not to the forwarding field.
	v = v.DerefValue()
	if field := p.fieldDescription(v, label); field != nil {
		return field
	}
	if !v.IsList() || !label.IsInt() {
		return nil
	}
	if v.IsClosedList() {
		return &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("list index out of range")}
	}
	tail := &adt.Vertex{Label: label}
	v.MatchAndInsert(p.ctx, tail)
	tail.Finalize(p.ctx)
	if b := tail.Bottom(); b != nil && b.IsIncomplete() {
		return nil
	}
	// A successful selection from a checked list has its declared element
	// interface, including callable evidence for a homogeneous tail.
	p.assume(tail, make(map[adt.Value]bool))
	return tail
}

// Negation reverses ordered interval endpoints, while preserving numeric
// kinds, exclusions, and unions. This is a proof for the whole input domain.
func (p *inference) negateNumber(v adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(v).(type) {
	case *adt.Num:
		return p.schema(nil, &adt.UnaryExpr{Op: adt.SubtractOp, X: x})
	case *adt.BoundValue:
		op := x.Op
		switch op {
		case adt.LessThanOp:
			op = adt.GreaterThanOp
		case adt.LessEqualOp:
			op = adt.GreaterEqualOp
		case adt.GreaterThanOp:
			op = adt.LessThanOp
		case adt.GreaterEqualOp:
			op = adt.LessEqualOp
		case adt.NotEqualOp:
		default:
			return &adt.BasicType{K: v.Kind()}
		}
		if n, ok := x.Value.(*adt.Num); ok {
			value := p.negateNumber(n)
			if value == nil {
				return nil
			}
			return &adt.BoundValue{Op: op, Value: value}
		}
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			v := p.negateNumber(term)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return out
	case *adt.Disjunction:
		out := &adt.Disjunction{}
		for _, term := range x.Values {
			v := p.negateNumber(term)
			if v == nil {
				return nil
			}
			out.Values = append(out.Values, v)
		}
		return out
	}
	return &adt.BasicType{K: v.Kind()}
}

// Translation by a constant preserves numeric interval predicates. This
// proves operations such as a nonnegative counter's successor without
// testing concrete examples or discarding its lower bound.
func (p *inference) translateNumber(v adt.Value, n *adt.Num, op adt.Op) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(v).(type) {
	case *adt.Num:
		return adt.BinOp(p.ctx, nil, op, x, n)
	case *adt.BoundValue:
		switch x.Op {
		case adt.LessThanOp, adt.LessEqualOp, adt.GreaterThanOp, adt.GreaterEqualOp, adt.NotEqualOp:
			if b, ok := x.Value.(*adt.Num); ok {
				return &adt.BoundValue{Op: x.Op, Value: adt.BinOp(p.ctx, nil, op, b, n)}
			}
		}
	case *adt.Conjunction:
		out := &adt.Conjunction{}
		for _, term := range x.Values {
			value := p.translateNumber(term, n, op)
			if value == nil {
				return nil
			}
			out.Values = append(out.Values, value)
		}
		return out
	}
	return &adt.BasicType{K: v.Kind() | n.Kind()}
}

func (p *inference) call(env *adt.Environment, call *adt.CallExpr) adt.Value {
	callee := adt.Unwrap(p.expr(env, call.Fun))
	return p.apply(env, callee, call)
}

func (p *inference) apply(env *adt.Environment, callee adt.Value, call *adt.CallExpr) adt.Value {
	if callee == nil {
		return nil
	}
	if err := (&relevanceChecker{p: p}).description(callee, make(map[adt.Value]bool)); err != nil {
		p.failure = &adt.Bottom{Src: call.Source(), Code: adt.BlockedError,
			Err: p.ctx.NewPosf(adt.Pos(call), "%s", err)}
		return nil
	}
	packet := &adt.Function{}
	args := make([]adt.Value, len(call.Args))
	for i, arg := range call.Args {
		args[i] = p.constructorEvidence(p.expr(env, arg))
		if args[i] == nil {
			return nil
		}
		if err := (&relevanceChecker{p: p}).description(args[i], make(map[adt.Value]bool)); err != nil {
			p.failure = &adt.Bottom{Src: arg.Source(), Code: adt.BlockedError,
				Err: p.ctx.NewPosf(adt.Pos(arg), "%s", err)}
			return nil
		}
		label := adt.InvalidLabel
		if i < len(call.ArgLabels) {
			label = call.ArgLabels[i]
		}
		packet.Params = append(packet.Params, adt.FuncParam{Value: args[i], Label: label, Positional: label == adt.InvalidLabel})
	}
	target := adt.FuncType{Fn: packet}
	var result adt.Value
	if call.Partial {
		result = p.partialCall(callee, target)
	} else {
		result = p.callValue(callee, target)
	}
	if result != nil {
		for _, arg := range args {
			if refuted(arg) {
				return arg // The checked packet cannot produce a value.
			}
		}
	}

	// The call rule establishes the successful result's interface. Retain
	// that evidence for higher-order elimination, including a quantified
	// callback returned by an explicitly impredicative instance.
	p.assume(result, make(map[adt.Value]bool))
	return result
}

func (p *inference) partialCall(callee adt.Value, packet adt.FuncType) adt.Value {
	if !p.step() {
		return nil
	}
	switch f := callee.(type) {
	case *adt.Disjunction:
		var results []adt.Value
		for _, branch := range f.Values {
			v := p.partialCall(adt.Unwrap(branch), packet)
			if v == nil {
				return nil
			}
			results = append(results, v)
		}
		return proofUnion(results)
	case *adt.Conjunction:
		var results []adt.Value
		for _, term := range f.Values {
			if v := p.partialCall(adt.Unwrap(term), packet); v != nil {
				results = append(results, v)
			}
		}
		return p.meetResults(results)
	case *adt.FuncValue:
		if f.Src != nil && f.Src.Effect != nil {
			return nil
		}
		if p.hypotheses[f] {
			p.useHypothesis(f)
		} else if !p.implementation(f) {
			return nil
		}
		bound, b := adt.BindFunctionPacket(p.ctx, f, packet)
		if b != nil || !p.savedPacket(bound) {
			return nil
		}
		p.assume(bound, make(map[adt.Value]bool))
		return bound
	}
	return nil
}

func (p *inference) callValue(callee adt.Value, target adt.FuncType) adt.Value {
	if validator, ok := callee.(*adt.BuiltinValidator); ok && validator.Src == nil {
		for _, param := range target.Fn.Params {
			if param.Label != adt.InvalidLabel {
				return nil
			}
		}
		callee = validator.Builtin
	}
	if !p.step() {
		return nil
	}
	if union, ok := callee.(*adt.Disjunction); ok {
		var alternatives []adt.Value
		for _, branch := range union.Values {
			result := p.callValue(adt.Unwrap(branch), target)
			if result == nil {
				return nil
			}
			alternatives = append(alternatives, result)
		}
		return proofUnion(alternatives)
	}
	if conjunction, ok := callee.(*adt.Conjunction); ok {
		var consequences []adt.Value
		for _, term := range conjunction.Values {
			if result := p.callValue(adt.Unwrap(term), target); result != nil {
				consequences = append(consequences, result)
			}
		}
		return p.meetResults(consequences)
	}
	var source adt.FuncType
	switch f := callee.(type) {
	case *adt.FuncValue:
		if f.Src != nil && f.Src.Effect != nil {
			return nil
		}
		source = adt.FuncType{Fn: f.ResidualSignature(), Env: f.Env}
	case *adt.Builtin:
		return p.builtinCall(f, target)
	default:
		return nil
	}
	sources := []adt.FuncType{source}
	results := sources
	if f, ok := callee.(*adt.FuncValue); ok {
		if p.hypotheses[f] {
			p.useHypothesis(f)
		} else if !p.implementation(f) {
			return nil
		}
	}
	function, _ := callee.(*adt.FuncValue)
	return p.callPackets(target, sources, results, function, 0)
}

// Finite disjunctions describe alternative packets, not an argument that must
// belong to one clause uniformly. Split them under the proof's work budget,
// prove every packet family, and join the guaranteed results. This makes
// overload coverage independent of intersection order.
func (p *inference) callPackets(target adt.FuncType, sources, results []adt.FuncType, function *adt.FuncValue, start int) adt.Value {
	if !p.step() {
		return nil
	}
	for i := start; i < len(target.Fn.Params); i++ {
		union, ok := adt.Unwrap(target.Fn.Params[i].Value.(adt.Value)).(*adt.Disjunction)
		if !ok {
			continue
		}
		var alternatives []adt.Value
		for _, branch := range union.Values {
			packet := *target.Fn
			packet.Params = slices.Clone(packet.Params)
			packet.Params[i].Value = branch
			result := p.callPackets(adt.FuncType{Fn: &packet, Env: target.Env}, sources, results, function, i+1)
			if result == nil {
				return nil
			}
			alternatives = append(alternatives, result)
		}
		return proofUnion(alternatives)
	}
	if function != nil {
		sources = function.CallClausesFor(p.ctx, target)
		results = function.ResultClausesFor(p.ctx, target)
	}
	s := &subsumer{ctx: p.ctx, inference: p}
	admit := func(source adt.FuncType, activation bool) (adt.FuncType, bool) {
		if len(adt.FunctionTypeParameters(source)) != 0 {
			var b *adt.Bottom
			source, b = adt.InstantiateFunctionType(p.ctx, source, target)
			if b != nil {
				return source, false
			}
		}
		noResult := *source.Fn
		noResult.Ret = nil
		if activation {
			return source, s.activationSignature(target, adt.FuncType{Fn: &noResult, Env: source.Env})
		}
		return source, s.capabilitySignature(target, adt.FuncType{Fn: &noResult, Env: source.Env})
	}
	admitted := false
	var consequences []adt.Value
	addResult := func(source adt.FuncType) {
		if failure := p.packetFailure(target, source); failure != nil {
			consequences = append(consequences, failure)
			return
		}
		if result := p.schema(source.Env, source.Fn.Ret); result != nil {
			consequences = append(consequences, result)
		}
		// Only a visible implementation supplies body evidence. Recursive
		// calls use their declared induction hypothesis. A linked hypothesis
		// supplies an interface without manufacturing a body summary.
		if function != nil && function.Fn.Body != nil && !p.recursing(function) {
			proof := target
			noResult := *source.Fn
			noResult.Ret = nil
			if s.capabilitySignature(adt.FuncType{Fn: &noResult, Env: source.Env}, target) {
				// Equivalent packet types reuse the declaration's body proof.
				proof = source
			} else {
				for _, previous := range p.callProofs[function] {
					if !p.step() {
						return
					}
					if s.capabilitySignature(previous, target) && s.capabilitySignature(target, previous) {
						proof = previous
						break
					}
				}
				if proof == target {
					if p.callProofs == nil {
						p.callProofs = make(map[*adt.FuncValue][]adt.FuncType)
					}
					p.callProofs[function] = append(p.callProofs[function], target)
				}
			}
			if p.function(function, proof) {
				if result := p.completed[proofKey{function, proof}].result; result != nil {
					consequences = append(consequences, result)
				}
			}
		}
	}
	seen := make(map[adt.FuncType]bool)
	for _, source := range sources {
		seen[source] = true
		activation := function != nil && function.Fn.Body != nil && source.Fn == function.Fn
		if source, ok := admit(source, activation); ok {
			admitted = true
			addResult(source)
		}
	}
	if !admitted {
		// A bodyless hypothesis can constrain an open packet to its domain.
		// Different possible domains describe alternative packets. Their
		// results are joined; compatible guards are not simultaneous facts.
		var alternatives []adt.Value
		for _, source := range sources {
			if source, ok := admit(source, true); ok {
				if failure := p.packetFailure(target, source); failure != nil {
					alternatives = append(alternatives, failure)
					continue
				}
				if result := p.schema(source.Env, source.Fn.Ret); result != nil {
					alternatives = append(alternatives, result)
				}
			}
		}
		if len(alternatives) == 0 {
			return nil
		}
		return proofUnion(alternatives)
	}
	for _, source := range results {
		if seen[source] {
			continue
		}
		seen[source] = true
		if source, ok := admit(source, false); ok {
			addResult(source)
		}
	}
	return p.meetResults(consequences)
}

func (p *inference) meetResults(consequences []adt.Value) adt.Value {
	if len(consequences) == 0 {
		return nil
	}
	result := consequences[0]
	for _, constraint := range consequences[1:] {
		result = p.eagerMeet(result, constraint)
	}
	return result
}

func proofUnion(values []adt.Value) adt.Value {
	if len(values) == 1 {
		return values[0]
	}
	return &adt.Disjunction{Values: values}
}

// Every operational list is finite. Iterating its element predicate proves
// a finite comprehension uniformly without testing a representative list.
// Conditions may suppress elements; they must themselves be total booleans.
func (p *inference) comprehension(env *adt.Environment, comp *adt.Comprehension) (adt.Value, bool) {
	if comp.Fallback != nil {
		return nil, false
	}
	for _, clause := range comp.Clauses {
		switch x := clause.(type) {
		case *adt.ForClause:
			v, ok := p.expr(env, x.Src).(*adt.Vertex)
			if !ok || !v.IsList() {
				return nil, false
			}
			var elems []adt.Value
			for a := range v.Elems() {
				elems = append(elems, a)
			}
			if !v.IsClosedList() {
				tail := &adt.Vertex{Label: adt.MakeIntLabel(adt.IntLabel, int64(len(elems)))}
				v.MatchAndInsert(p.ctx, tail)
				tail.Finalize(p.ctx)
				if tail.Bottom() != nil {
					return nil, false
				}
				elems = append(elems, tail)
			}
			if len(elems) == 0 {
				return nil, true
			}
			values := map[adt.Feature]adt.Value{x.Value: proofUnion(elems)}
			if x.Key != adt.InvalidLabel {
				values[x.Key] = &adt.BasicType{K: adt.IntKind}
			}
			env = p.frame(env, values)
		case *adt.IfClause:
			v := p.expr(env, x.Condition)
			if v == nil || v.Kind() != adt.BoolKind {
				return nil, false
			}
		case *adt.LetClause:
			v := p.expr(env, x.Expr)
			if v == nil {
				return nil, false
			}
			env = p.frame(env, map[adt.Feature]adt.Value{x.Label: v})
		default:
			return nil, false
		}
	}
	return p.expr(env, comp.Value), false
}

// packetFailure derives an ordinary contradiction of fresh activation cells.
// This is independent of a body or coverage theorem. In particular a bodyless
// declaration still rejects a contradictory packet before materialization.
func (p *inference) packetFailure(packet, source adt.FuncType) *adt.Bottom {
	matches := adt.MatchFuncValueParams(packet.Fn, &adt.FuncValue{Fn: source.Fn})
	for i, j := range matches {
		if j < 0 {
			continue // Protocol evidence is produced by the separate rule.
		}
		got, ok := packet.Fn.Params[i].Value.(adt.Value)
		if !ok {
			got = p.schema(packet.Env, packet.Fn.Params[i].Value)
		}
		want, ok := source.Fn.Params[j].Value.(adt.Value)
		if !ok {
			want = p.schema(source.Env, source.Fn.Params[j].Value)
		}
		if kind, ok := want.(*adt.BasicType); ok && got != nil &&
			got.Kind() != adt.BottomKind && got.Kind()&^kind.K == 0 {
			if got.Kind()&(adt.StructKind|adt.ListKind) == 0 {
				continue // The kind theorem already excludes a packet conflict.
			}
		}
		got = p.bindingDescription(got)
		if got == nil || want == nil || packetHasFunction(want, make(map[adt.Value]bool)) {
			continue
		}
		meet := p.sourceMeet(got, want)
		if refuted(meet) {
			return adt.Unwrap(meet).(*adt.Bottom)
		}
		if v, ok := meet.(*adt.Vertex); ok {
			if b := adt.Validate(p.ctx, v, &adt.ValidateConfig{}); b != nil && !b.IsIncomplete() {
				return b
			}
		}
	}
	return nil
}
