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
- [Type-reference examples](../../../doc/types.md) are executed directly from
  the document by `TestReferenceExamples`; `TestReferenceCurrent` checks its
  generated package catalogue.
- [Native inventory](../../../pkg/builtin_checking_test.go) checks every
  registered standard-library function's argument and result contracts,
  ordinary and partial calls, labels, and validator forms.
- [Operator matrix](../../operator_checking_test.go) covers unary and binary
  operand domains, selection, slicing, interpolation, and successful results.
- Standard-library regressions cover
  [generic and refined results](../../stdlib_precision_test.go),
  [list shapes](../../stdlib_list_shape_test.go),
  [finite pure calls](../../stdlib_ground_test.go),
  [selected functions](../../stdlib_function_test.go),
  [partials and export](../../stdlib_partial_test.go),
  [sorting templates](../../stdlib_comparer_test.go),
  [predicates](../../stdlib_predicate_test.go),
  [adapters](../../stdlib_adapter_test.go),
  [codecs](../../stdlib_codec_test.go),
  [saved schemas](../../stdlib_schema_test.go),
  [validator labels](../../stdlib_validator_labels_test.go), and
  [error-only results](../../stdlib_error_result_test.go).

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
go test ./internal/cmd/gentypes ./pkg
go test ./cue -run 'TestBuiltin|TestOperator|TestStdlib'
```

The [oracle guide](../../../doc/oracle.md) describes the independent models and
larger generated runs.
