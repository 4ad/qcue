# Quantified CUE

Quantified CUE (`qcue`) is a fork of [CUE](https://github.com/cue-lang/cue) that
adds quantifiers, polymorphic functions, and opaque existential packages to
CUE's constraint language.

Universal quantification describes a single value that satisfies a contract
for every admissible type, including polymorphic functions and callbacks.
Opaque existential packages hide a representation type behind an interface,
allowing abstract data types and modules to be passed as values.

The implementation supports fragments of the proposal's predicative
higher-rank profile (`S_H`) and abstraction profile (`A`). The proposal also
describes features outside these implemented fragments. Quantifiers are
enabled by default for CUE language version `v0.18.0` and later.

This repository contains the language implementation, the `cue` command, and
the Go API under the existing `cuelang.org/go` module path.

- [Design proposal](doc/paper.pdf) ([LaTeX source](doc/paper.tex)): the language
  design and formal semantics.
- [Implementation guide](doc/implementation.md): supported features, checking
  limits, installation, and usage.
- [Paper examples](cue/testdata/quantified/paper/README.md): every listing
  reproduced verbatim, with tests and documented implementation limits.
- [Quantified test index](cue/testdata/quantified/README.md): executable examples
  and regression coverage.
- [Oracle guide](doc/oracle.md): independent semantic models and fuzzing.

<!--
 Copyright 2018 The CUE Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->
