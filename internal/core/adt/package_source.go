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

import "slices"

// PackageSource identifies a seal construction and its lexical environment.
// Source serialization may expose that construction without changing the
// observations permitted on the resulting package during evaluation.
type PackageSource struct {
	Seal *PackageSeal
	Env  *Environment
}

// PackageSource returns the construction of a closed opaque package. An
// opened view cannot be serialized independently of its lexical permission.
func (v *Vertex) PackageSource() PackageSource {
	v = v.DerefValue()
	if v.sealed == nil || v.sealedOpened {
		return PackageSource{}
	}
	return v.sealed.source
}

// PackageView returns the original public view, before subsequent
// refinements. Source export must retain those refinements separately.
func (v *Vertex) PackageView() *Vertex {
	v = v.DerefValue()
	if v.sealed == nil || v.sealedOpened {
		return nil
	}
	return v.sealed.view
}

// PackageProjection returns the public path of an exported operation. The
// exporter must replay this projection through an opening, rather than copy
// the private function and thereby discard its public capability identity.
func (s *OpaqueCall) PackageProjection() (PackageSource, []Feature) {
	if !s.outward || s.export == nil {
		return PackageSource{}, nil
	}
	var path []Feature
	for x := s.export.canonical(); x.parent != nil; x = x.parent.canonical() {
		path = append(path, x.label)
	}
	slices.Reverse(path)
	return s.owner.source, path
}
