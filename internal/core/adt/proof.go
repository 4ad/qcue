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

// proofResult is the result of a local proof procedure for a proposition.
// Unknown is a suspended obligation. Refutation requires positive evidence
// of the proposition's negation, never merely the failure to establish it.
type proofResult uint8

const (
	proofUnknown proofResult = iota
	proofEstablished
	proofRefuted
)

type inclusionCheck struct{ bound, argument Value }

// provesInclusion calls the proof layer without making failed proof-search
// premises into program constraints. Recursive proof dependencies remain
// pending instead of using the proposition as its own evidence.
func (c *OpContext) provesInclusion(bound, argument Value) bool {
	if c.ProveInclusion == nil {
		return false
	}
	key := inclusionCheck{bound, argument}
	if c.inclusionChecks[key] {
		return false
	}
	if c.inclusionChecks == nil {
		c.inclusionChecks = make(map[inclusionCheck]bool)
	}
	c.inclusionChecks[key] = true
	defer delete(c.inclusionChecks, key)
	saved := c.PushState(c.Env(0), bound.Source())
	proved := c.ProveInclusion(c, bound, argument)
	return c.PopState(saved) == nil && proved
}
