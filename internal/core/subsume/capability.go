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

// Capability inclusion is contravariant in packets and covariant in results.
// Each target clause needs a proof. One stronger source clause is sufficient;
// failure to find one leaves inclusion unproved, never refutes the meet.
func (s *subsumer) capabilityValues(a, b *adt.FuncValue) bool {
	if !adt.IsFuncType(a) {
		if adt.IsFuncType(b) || a.Fn != b.Fn || !a.EqualArgs(b) || !a.Env.Equal(s.ctx, b.Env) {
			return false
		}
		for _, t := range a.Types {
			if !s.hasFunc(b, t) {
				return false
			}
		}
		return true
	}
	targets := append([]adt.FuncType{{Fn: a.Fn, Env: a.Env}}, a.Types...)
	sources := append([]adt.FuncType{{Fn: b.Fn, Env: b.Env}}, b.Types...)
	if b.IsPartial() {
		// The remaining implementation protocol is known. Its old clauses
		// describe full packets and cannot be read as residual signatures.
		sources = []adt.FuncType{{Fn: b.ResidualSignature(), Env: b.Env}}
	}
	for _, target := range targets {
		proved := false
		for _, source := range sources {
			if s.capabilitySignature(target, source) {
				proved = true
				break
			}
		}
		if !proved {
			return false
		}
	}
	return true
}

func (s *subsumer) capabilitySignature(target, source adt.FuncType) bool {
	if target == source {
		return true
	}
	a, b := target.Fn, source.Fn
	if a.Open {
		// The shared protocol row is unresolved. Do not replace it with a
		// promise that arbitrary additional packets succeed.
		return false
	}
	matches := adt.MatchFuncValueParams(a, &adt.FuncValue{Fn: b})
	used := make([]bool, len(b.Params))
	for i, p := range a.Params {
		j := matches[i]
		if j < 0 {
			// Even an optional parameter can be supplied in a target packet.
			return false
		}
		used[j] = true
		q := b.Params[j]
		if p.Label != adt.InvalidLabel && p.Label != q.Label {
			return false
		}
		if (p.ArcType == adt.ArcOptional || p.Default != nil) &&
			q.ArcType != adt.ArcOptional && q.Default == nil {
			return false
		}
		if !s.funcConstraint(source.Env, q.Value, target.Env, p.Value) {
			return false
		}
	}
	for j, p := range b.Params {
		if !used[j] && p.ArcType != adt.ArcOptional && p.Default == nil {
			return false
		}
	}
	return s.funcConstraint(target.Env, a.Ret, source.Env, b.Ret)
}
