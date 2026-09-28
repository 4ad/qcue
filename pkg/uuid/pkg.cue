// Copyright 2026 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file is maintained by hand: it is the authoritative description
// of the package's API for tooling such as editor completion and hover.
// Its consistency with the registered builtins is checked by the tests
// in cuelang.org/go/pkg.

@experiment(functions)
@pure()

// Package uuid defines functionality for creating UUIDs as defined in RFC 4122.
//
// Currently only Version 5 (SHA1) and Version 3 (MD5) are supported.
package uuid

// Valid ensures that s is a valid UUID which would be accepted by Parse.
Valid: validator(string) | (func(s: string) -> _)

// Parse decodes s into a UUID or returns an error. Both the standard UUID forms
// of xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx and
// urn:uuid:xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx are decoded as well as the
// Microsoft encoding {xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx} and the raw hex
// encoding: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.
Parse: func(s: string) -> string

// URN reports the canonical URN of a UUID.
URN: func(x: string) -> string

// FromInt creates a UUID from an integer.
//
// 	DNS:  uuid.FromInt(0x6ba7b810_9dad_11d1_80b4_00c04fd430c8)
FromInt: func(i: int) -> string

// ToInt represents a UUID string as a 128-bit value.
ToInt: func(x: string) -> int & >=0

// Variant reports the UUID variant.
Variant: func(x: string) -> int & >=0 & <=4

// Version reports the UUID version.
Version: func(x: string) -> int & >=0 & <=15

// SHA1 generates a version 5 UUID based on the supplied name space and data.
SHA1: func(space: string, data: bytes | string) -> string

// MD5 generates a version 3 UUID based on the supplied name space and data.
// Use SHA1 instead if you can.
MD5: func(space: string, data: bytes | string) -> string

// Predefined namespaces
ns: {
	DNS:  "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	URL:  "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
	OID:  "6ba7b812-9dad-11d1-80b4-00c04fd430c8"
	X500: "6ba7b814-9dad-11d1-80b4-00c04fd430c8"
	Nil:  "00000000-0000-0000-0000-000000000000"
}

// Invalid UUID
variants: Invalid: 0
// The variant specified in RFC4122
variants: RFC4122: 1
// Reserved, NCS backward compatibility.
variants: Reserved: 2
// Reserved, Microsoft Corporation backward compatibility.
variants: Microsoft: 3
// Reserved for future definition.
variants: Future: 4
