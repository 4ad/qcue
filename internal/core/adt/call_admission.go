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

// callPacket retains the supplied expressions in their original scopes,
// normalized into a particular protocol's slots. It contains neither defaults
// nor parameter predicates. An activation is a different object: it supports
// relational refinement during execution and cannot serve as admission
// evidence for the packet that created it.
type callPacket struct {
	args []funcArg
}

// packetAdmission is evidence for one scoped clause and the original packet.
// Only callPacket.admit constructs it. Consumers may propagate this clause's
// result, but cannot infer that an implementation has executed.
type packetAdmission struct {
	clause FuncType
	packet callPacket
}

func (packet callPacket) admit(c *OpContext, clause FuncType, partial bool) (*packetAdmission, proofResult) {
	if len(packet.args) != len(clause.Fn.Params) {
		return nil, proofUnknown
	}
	if len(typeParameters(clause.Env)) != 0 {
		view, b := (&FuncValue{Fn: clause.Fn, Env: clause.Env}).inferInstance(c, packet.args)
		if b != nil {
			if b.IsIncomplete() {
				return nil, proofUnknown
			}
			return nil, proofRefuted
		}
		clause.Env = view.Env
	}
	result := packet.membership(c, clause, partial)
	if result != proofEstablished {
		return nil, result
	}
	return &packetAdmission{clause: clause, packet: packet}, proofEstablished
}

// membership is shared by all concrete packet admission paths. A missing
// optional slot stays absent; a default grants omission without becoming an
// argument. Partial application checks supplied slots only.
func (packet callPacket) membership(c *OpContext, clause FuncType, partial bool) proofResult {
	result := proofEstablished
	for i, param := range clause.Fn.Params {
		arg := packet.args[i]
		if arg.expr == nil {
			if !partial && param.ArcType != ArcOptional && param.Default == nil {
				return proofRefuted
			}
			continue
		}
		value, _ := c.Evaluate(arg.env, arg.expr)
		switch capabilityMember(c, clause.Env, param.Value, value) {
		case proofRefuted:
			return proofRefuted
		case proofUnknown:
			result = proofUnknown
		}
	}
	return result
}

// project aligns an implementation packet with an attached contract, including
// the fixed coordinate system of contracts attached after partial application.
// Protocol exclusion is independent of value membership and type inference.
func (packet callPacket) project(clause FuncType, implementation *Function) (callPacket, proofResult) {
	matches := capabilityMatches(clause, implementation)
	projected := callPacket{args: make([]funcArg, len(clause.Fn.Params))}
	used := make([]bool, len(packet.args))
	for i, j := range matches {
		if j >= 0 && j < len(packet.args) {
			projected.args[i] = packet.args[j]
			used[j] = true
		}
	}
	if !clause.Fn.Open {
		for j, arg := range packet.args {
			if clause.partial != nil && j < len(clause.partial.args) && clause.partial.args[j].expr != nil {
				continue
			}
			if arg.expr != nil && !used[j] {
				return projected, proofRefuted
			}
		}
	}
	return projected, proofEstablished
}
