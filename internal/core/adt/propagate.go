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

// EvidenceState distinguishes positive evidence from unfinished work. A failed
// search is Pending, never Rejected. Rejected requires a checked refutation in
// the same assumption store as the goal.
type EvidenceState uint8

const (
	Pending EvidenceState = iota
	Established
	Rejected
	// Quiet records completed mandatory observation work. It is not a
	// theorem and must be revisited when any supported premise changes.
	Quiet
)

// Suspension describes the premise still needed by a residual obligation.
type Suspension uint8

const (
	MissingEvidence Suspension = iota
	MissingData
	DependencyCycle
	WorkLimit
)

// Evidence is a consequence of one accounted propagation step. Value is a
// summary, not a replacement for the source expression or its validation
// demands. Err describes a refutation or an unresolved goal; it does not turn
// pending work into semantic bottom.
type Evidence struct {
	State EvidenceState
	Wait  Suspension
	Value Value
	Err   *Bottom
	// Support is the kernel-checked derivation and its undischarged hypotheses.
	Support any
}

// Goal is a scoped proposition and its accumulated evidence. Its key includes
// the target's lexical environment and all local proof hypotheses. A goal's
// dependencies are retained even when its current result is an atom.
type Goal struct {
	Evidence
	key          any
	step         func(*Propagation) Evidence
	active       bool
	queued       bool
	dependencies map[*Goal]bool
	dependents   map[*Goal]bool
}

// Propagation owns the worklist shared by evaluation and proof production.
// Adding constraints and evidence wakes their dependents. Scheduling has no
// semantic force: the source graph remains in vertices and conjuncts, and this
// graph records only justified consequences and outstanding obligations.
//
// Established and rejected goals are persistent theorems about fixed targets.
// Refining a source creates additional goals; it does not change an old goal's
// proposition. Snapshot-dependent searches and observation passes stay Pending
// until their premises supply evidence. Quiet relevance passes must therefore
// not be recorded as Established propositions.
type Propagation struct {
	goals     map[any]*Goal
	watchers  map[*Vertex]map[*Goal]bool
	queue     []*Goal
	current   *Goal
	remaining int
}

func NewPropagation(budget int) *Propagation {
	if budget < 0 {
		budget = 0
	}
	return &Propagation{
		goals:     make(map[any]*Goal),
		watchers:  make(map[*Vertex]map[*Goal]bool),
		remaining: budget,
	}
}

// Require installs a proposition or consults its evidence, recording the edge
// from the current goal. A reentrant demand remains residual. In particular,
// merely finding a cycle never supplies a hypothesis-discharge inference.
func (p *Propagation) Require(key any, step func(*Propagation) Evidence) *Goal {
	g := p.goals[key]
	if g == nil {
		g = &Goal{key: key, step: step}
		p.goals[key] = g
	}
	if parent := p.current; parent != nil {
		if parent.dependencies == nil {
			parent.dependencies = make(map[*Goal]bool)
		}
		if g.dependents == nil {
			g.dependents = make(map[*Goal]bool)
		}
		parent.dependencies[g], g.dependents[parent] = true, true
	}
	if g.active {
		// Do not overwrite evidence produced by another derivation. An
		// active goal is necessarily pending here.
		g.Wait = DependencyCycle
		return g
	}
	if g.State == Pending {
		p.run(g)
	}
	if p.current == nil {
		p.Drain()
	}
	return g
}

func (p *Propagation) run(g *Goal) {
	if g.active || g.State != Pending {
		return
	}
	if !p.Charge(1) {
		g.Wait = WorkLimit
		p.enqueue(g)
		return
	}
	previous := p.current
	p.current, g.active = g, true
	result := g.step(p)
	p.current, g.active = previous, false
	if result.State == Pending && g.Wait == DependencyCycle {
		result.Wait = DependencyCycle
	}
	g.Evidence = result
	if result.State != Pending {
		for dependent := range g.dependents {
			// An active requester consumes this result on return from
			// Require. Re-enqueuing it would repeat completed premises and
			// manufacture work without any new information.
			if !dependent.active {
				p.enqueue(dependent)
			}
		}
	}
}

func (p *Propagation) enqueue(g *Goal) {
	if !g.queued && g.State == Pending {
		g.queued = true
		p.queue = append(p.queue, g)
	}
}

// Observe records a live data dependency of the active inference. Callers
// notify the graph when evaluating that vertex supplies stronger information.
func (p *Propagation) Observe(v *Vertex) {
	if p.current == nil || v == nil {
		return
	}
	if p.watchers[v] == nil {
		p.watchers[v] = make(map[*Goal]bool)
	}
	p.watchers[v][p.current] = true
}

func (p *Propagation) Notify(v *Vertex) {
	for g := range p.watchers[v] {
		if g.State == Quiet {
			g.State = Pending
		}
		p.enqueue(g)
	}
}

// Charge accounts for proof search, substitution and expansion as well as
// worklist dispatch. Exhaustion preserves the entire remaining graph.
func (p *Propagation) Charge(n int) bool {
	if n < 0 || n > p.remaining {
		p.remaining = 0
		return false
	}
	p.remaining -= n
	return true
}

// Drain performs enabled work without polling pending goals whose premises
// have not changed. It may be resumed after adding an explicit work allowance.
func (p *Propagation) Drain() {
	for len(p.queue) != 0 && p.remaining > 0 {
		g := p.queue[0]
		p.queue[0] = nil
		p.queue = p.queue[1:]
		g.queued = false
		p.run(g)
	}
}

func (p *Propagation) AddBudget(n int) {
	if n > 0 {
		p.remaining += n
	}
}

// Residual returns all unfinished goals, including cycles and queued work.
// The order is intentionally unspecified; scheduling is not an observation.
func (p *Propagation) Residual() []*Goal {
	var out []*Goal
	for _, g := range p.goals {
		if g.State == Pending {
			out = append(out, g)
		}
	}
	return out
}

// Diagnostic exposes unfinished or rejected checking without treating it as
// semantic emptiness. The source graph and the goal remain available to a
// later refinement or an additional work allowance.
func (g *Goal) Diagnostic(c *OpContext, source Node) *Bottom {
	if g.Err != nil {
		return g.Err
	}
	if g.State != Established && g.State != Quiet {
		return &Bottom{Src: source.Source(), Code: BlockedError,
			Err: c.Newf("function invocation awaits evidence")}
	}
	return nil
}

func (c *OpContext) invocationEvidence(f Value, call *CallExpr) *Goal {
	if c.CallEvidence != nil {
		return c.CallEvidence(c, c.Env(0), f, call)
	}
	return &Goal{Evidence: Evidence{Err: &Bottom{Src: call.Source(), Code: BlockedError,
		Err: c.Newf("function propagation is not configured")}}}
}
