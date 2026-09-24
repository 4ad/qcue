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

// checkWitness is the formation boundary shared by universal elimination and
// explicit existential introduction. Value ranges require value membership;
// type bounds require universe formation and predicate inclusion. A runtime
// kind or a successful compatibility calculation cannot choose the sort.
func (p *TypeParameter) checkWitness(c *OpContext, env *Environment, value Value) *Bottom {
	if p.ValueRange != nil {
		switch capabilityMember(c, env, p.ValueRange, value) {
		case proofRefuted:
			return c.NewErrf("value argument %s is outside the range of %s", value, p.Src.Name.Name)
		case proofUnknown:
			return &Bottom{Src: p.Src, Code: IncompleteError,
				Err: c.Newf("unresolved value argument range for %s", p.Src.Name.Name)}
		}
		return nil
	}
	if b := checkTypeUniverse(c, p, value); b != nil {
		return b
	}
	if p.Bound == nil {
		return nil
	}
	bound, _ := c.Evaluate(env, p.Bound)
	switch typeArgumentFits(c, bound, value) {
	case proofRefuted:
		return c.NewErrf("type argument %s does not satisfy bound of %s", value, p.Src.Name.Name)
	case proofUnknown:
		return &Bottom{Src: p.Src, Code: IncompleteError,
			Err: c.Newf("unresolved type argument bound for %s", p.Src.Name.Name)}
	}
	return nil
}
