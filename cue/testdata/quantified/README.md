# Quantified constraint tests

The corpus exercises universal descriptions, functions, call packets, live
references, proof obligations, and later refinement. The ordinary evaluator
runner discovers executable txtar archives automatically.

- [Paper examples](paper/) preserve every current listing verbatim.
  `TestQuantifiedPaperIndex` compares the catalogue with `doc/paper.tex`.
- [Historical examples](paper_history/) retain earlier examples that still
  belong to the language, including examples no longer in the paper.
- [Additional examples](examples/) exercise individual language rules.
- [API fixtures](api/) support Go tests for `Unify`, `FillPath`, declaration
  order, closure identity, and source export.
- [Propagation tests](../../propagation_test.go) exercise live descriptions,
  residual goals, strict calls, and refinement after source export.
- [Finite oracles](../../quantified_oracle_test.go) compare ordinary Boolean
  constraints with independent finite-set models. Their source renderer uses
  explicit finite expansions; value quantification is not language syntax.
- [Feature combinations](../../quantified_oracle_features_test.go) combine
  callbacks, partial calls, universals, qualified labels, and source export.

The parser, AST, formatter, compiler, and exporter have separate tests. Removed
existentials, sealing, opening, and value-binder syntax have parser rejection
cases. The former implementation and tests specifically requiring those
features have been removed. Mixed cases retain their universal and ordinary
constraint coverage. Universe-level binder spellings have been removed;
universal type arguments remain impredicative.

Archives keep expectations beside the source:

```cue
@test(json, 3, at="answer")
@test(validate, concrete, at="identity")
@test(validate, concrete, incomplete, at="pending")
@test(validate, conflict, at="contradiction")
```

`blocked` currently checks an unresolved proof demand without a permanent data
contradiction. An unsuccessful proof search must not turn into semantic bottom.
Paper archives use `#noformat` to preserve the published listing. API fixtures
use `#skip` only because a Go test runs their multi-step operations.

```sh
go test ./internal/core/adt -run TestEvalV3/quantified
go test ./cue -run 'TestQuantified|TestLiveDescription|TestPropagation'
go test ./cue/parser ./cue/format ./cue/ast/... ./internal/core/export
```

The [oracle guide](../../../doc/oracle.md) describes the independent models and
larger generated runs.
