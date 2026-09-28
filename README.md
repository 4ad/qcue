# Quantified CUE

Quantified CUE (`qcue`) is a fork of [CUE](https://github.com/cue-lang/cue) that
adds universal types and polymorphic functions whose constraints propagate
alongside ordinary CUE data constraints.

Descriptions may refer to open terms and remain linked to them under later
refinement. Function bodies and calling capabilities have independent proof
goals; calls allocate fresh constrained packets and retain their validation
dependencies. Evaluation can establish facts needed by a proof, and a proof
can enable further evaluation.

The implementation follows the propagator design in version 9 of the paper.
Universal types include higher-rank and impredicative instances. Existentials,
opaque packages, and dependent value binders are no longer language features.
Quantifiers are enabled by default for every CUE language version in this fork.

Builtins, operators, and standard-library functions participate in function
checking. Native contracts preserve generic element types, structured results,
parameter labels, defaults, and validator forms. See the
[type reference](doc/types.md) for the quantified signatures, builtin
contracts, and operator overloads.

This repository contains the language implementation, the `cue` command, and
the Go API under the existing `cuelang.org/go` module path.

- [Design proposal](doc/paper.pdf) ([LaTeX source](doc/paper.tex)): the language
  design and formal semantics.
- [Implementation guide](doc/implementation.md): supported features, checking
  limits, installation, and usage.
- [Type reference](doc/types.md): quantified builtin contracts, operator
  signatures, and every standard-library function type.
- [Command guide](doc/cmd/cue.md): building and using this fork's `cue` command.
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
