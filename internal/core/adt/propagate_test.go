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

import "testing"

func TestPropagationRefinement(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		p := NewPropagation(100)
		cell := &Vertex{}
		known := false
		visits := 0
		bound := func(p *Propagation) Evidence {
			p.Observe(cell)
			if known {
				return Evidence{State: Established}
			}
			return Evidence{Wait: MissingData}
		}
		body := func(p *Propagation) Evidence {
			visits++
			if p.Require("bound", bound).State == Established {
				return Evidence{State: Established}
			}
			return Evidence{}
		}
		if reverse {
			p.Require("bound", bound)
		}
		goal := p.Require("body", body)
		if goal.State != Pending || len(p.Residual()) != 2 {
			t.Fatal("missing data supplied an unfounded theorem")
		}
		before := visits
		p.Drain()
		if visits != before {
			t.Fatal("a suspended goal was polled without new evidence")
		}
		known = true
		p.Notify(cell)
		p.Drain()
		if goal.State != Established || len(p.Residual()) != 0 {
			t.Fatal("new data did not propagate through the proof dependency")
		}
		before = visits
		p.Notify(cell)
		p.Drain()
		if visits != before {
			t.Fatal("a persistent theorem was needlessly rechecked")
		}
	}
}

func TestPropagationCycleRequiresIndependentEvidence(t *testing.T) {
	p := NewPropagation(100)
	cell := &Vertex{}
	grounded := false
	var left, right func(*Propagation) Evidence
	left = func(p *Propagation) Evidence {
		p.Observe(cell)
		if grounded || p.Require("right", right).State == Established {
			return Evidence{State: Established}
		}
		return Evidence{}
	}
	right = func(p *Propagation) Evidence {
		if p.Require("left", left).State == Established {
			return Evidence{State: Established}
		}
		return Evidence{}
	}
	g := p.Require("left", left)
	if g.State != Pending || g.Wait != DependencyCycle {
		t.Fatal("cyclic dependency was not retained")
	}
	grounded = true
	p.Notify(cell)
	p.Drain()
	if g.State != Established || len(p.Residual()) != 0 {
		t.Fatal("independent premise did not ground the cycle")
	}
}

func TestPropagationBudgetRetainsWork(t *testing.T) {
	p := NewPropagation(1)
	leaf := func(*Propagation) Evidence { return Evidence{State: Established} }
	g := p.Require("root", func(p *Propagation) Evidence {
		if p.Require("leaf", leaf).State == Established {
			return Evidence{State: Established}
		}
		return Evidence{}
	})
	if g.State != Pending || len(p.Residual()) != 2 {
		t.Fatal("exhaustion discarded an obligation")
	}
	p.AddBudget(10)
	p.Drain()
	if g.State != Established || len(p.Residual()) != 0 {
		t.Fatal("resuming the retained work failed")
	}
}
