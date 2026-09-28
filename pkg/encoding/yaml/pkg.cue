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

package yaml

// Marshal returns the YAML encoding of v.
Marshal: func(v: null | bool | number | string | bytes | [...] | {...}) -> string

// MarshalStream returns the YAML encoding of v.
MarshalStream: func(v: [...]) -> string

// Unmarshal parses the YAML to a CUE expression.
Unmarshal: func(data: bytes | string) -> null | bool | number | string | bytes | [...] | {...}

// UnmarshalStream parses the YAML to a CUE list expression on success.
UnmarshalStream: func(data: bytes | string) -> [...]

// Validate validates YAML and confirms it is an instance of schema.
// If the YAML source is a stream, every object must match v.
Validate: (func(v: _ @schema()) -> validator(bytes | string)) | (func(b: bytes | string, v: _ @schema()) -> bool)

// ValidatePartial validates YAML and confirms it matches the constraints
// specified by v using unification. This means that b must be consistent with,
// but does not have to be an instance of v. If the YAML source is a stream,
// every object must match v.
ValidatePartial: (func(v: _ @schema()) -> validator(bytes | string)) | (func(b: bytes | string, v: _ @schema()) -> bool)
