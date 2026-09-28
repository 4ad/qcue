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

// resultInterfaces propagates proved callable interfaces to the closure
// produced by execution. In particular, returning a monomorphic closure can
// establish a universal interface without changing its code or captures.
// First-order result annotations never filter or enrich the returned data.
func (f *FuncValue) resultInterfaces(c *OpContext, packet callPacket, result Value) Value {
	closure, ok := Unwrap(result).(*FuncValue)
	if !ok {
		return result
	}
	copy := *closure
	for _, clause := range f.CallClauses(c) {
		if clause.Fn != f.Fn || clause.Env != f.Env {
			args, status := packet.project(clause, f.Fn)
			if status != proofEstablished {
				continue
			}
			admission, status := args.admit(c, clause, false)
			if status != proofEstablished {
				continue
			}
			clause = admission.clause
		}
		if clause.Fn.Ret == nil {
			continue
		}
		value, complete := c.Evaluate(clause.Env, clause.Fn.Ret)
		if !complete {
			continue
		}
		contract, ok := Unwrap(value).(*FuncValue)
		if !ok || contract.Fn.Body != nil || len(typeParameters(contract.Env)) == 0 {
			continue
		}
		copy.Types = mergeFuncTypes(copy.Types, contract.Obligations())
	}
	if len(copy.Types) == len(closure.Types) {
		return result
	}
	return &copy
}
