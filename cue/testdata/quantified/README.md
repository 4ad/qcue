# Quantified CUE tests

Start here for the **S_H** and **A** implementation. Language examples and
expectations live in txtar archives, grouped by topic. The ordinary CUE evaluator
runner discovers this directory automatically; no Go table needs updating when
adding a semantic case.

- [Paper corpus](paper/README.md): an index of all **101** listings, with exact
  paper text, executable context, and explicit expectations. All 78 executable
  S_H/A examples run, including expected errors and incomplete specifications.
  The two syntax templates are parsed; D and pseudocode exclusions are labeled.
- [API fixtures](api/): programs for refinement with `Unify` and `FillPath`,
  declaration ordering, source export, and closure completeness. Their Go
  assertions remain in [quantified_test.go](../../quantified_test.go) because
  these tests exercise Go API operations and object identity.

## Semantic topics

| Directory | Coverage |
| --- | --- |
| [abstract_overloads](abstract_overloads/) | Symbolic calls and intersections of result contracts |
| [capabilities](capabilities/) | Call protocols, defaults, named arguments, and retained obligations |
| [capability_refutations](capability_refutations/) | Contradictory capability intersections |
| [capability_subsumption](capability_subsumption/) | Directional inclusion of capability contracts |
| [certification](certification/) | Universal implementation proofs and unproved obligations |
| [closure_identity](closure_identity/) | Code origins, copied closures, and partial arguments |
| [comparable_packages](comparable_packages/) | Identity and equality of sealed packages |
| [composite_instances](composite_instances/) | Type selection on records and lists |
| [covariant_existentials](covariant_existentials/) | Existential witnesses in covariant data descriptions |
| [defaults](defaults/) | Quantified syntax enabled without an attribute; ordinary indexes |
| [distinct_closures](distinct_closures/) | Distinct implementations retain distinct identity |
| [finite_witnesses](finite_witnesses/) | Finite literal value binders |
| [first_class_packages](first_class_packages/) | Existential packages as inputs and outputs |
| [implementation_concreteness](implementation_concreteness/) | Materialization versus implementation conformance |
| [instantiation](instantiation/) | Implicit and explicit type application |
| [invalid_instances](invalid_instances/) | Rejected binder domains and invalid instances |
| [opaque_boundaries](opaque_boundaries/) | Abstract values, escaping operations, and opacity errors |
| [opaque_callbacks](opaque_callbacks/) | Transport of higher-order callbacks across a seal |
| [opaque_closure_escape](opaque_closure_escape/) | Captured abstract values and delayed escape checks |
| [opaque_composite_transport](opaque_composite_transport/) | Record and list transport across opaque boundaries |
| [opaque_generic_operations](opaque_generic_operations/) | Generic operations on abstract carriers |
| [opaque_universe_boundary](opaque_universe_boundary/) | Universe restrictions at opaque boundaries |
| [parametric_aliases](parametric_aliases/) | Description aliases, lexical scope, arity, and cycles |
| [partial_capabilities](partial_capabilities/) | Partial application and residual capabilities |
| [predicative_universes](predicative_universes/) | Universe levels and higher-rank checking |
| [record_data_projections](record_data_projections/) | Correlated fields and data projections |
| [recursion_requires_descent](recursion_requires_descent/) | Rejected recursion without a decreasing argument |
| [seal_generativity](seal_generativity/) | Fresh carriers, copied packages, and incompatible seals |
| [sealed_counters](sealed_counters/) | Equivalent public behavior with different private representations |
| [slice_preserves_source](slice_preserves_source/) | Slicing a universally constrained list |
| [structural_recursion](structural_recursion/) | Execution with finite structural descent |
| [universal_refutations](universal_refutations/) | Concrete counterexamples to universal descriptions |
| [universe_literals](universe_literals/) | Formation errors for invalid universe level literals |
| [universe_occurs_check](universe_occurs_check/) | Predicative cycles and self-application |

## Other layers

Every additional corpus has `quantified` in its path or filename:

| Layer | Fixtures | Runner |
| --- | --- | --- |
| Parser | [parser/testdata/quantified](../../parser/testdata/quantified/) | `TestQuantifiedSyntax` |
| Formatter | [format/testdata/quantified.txtar](../../format/testdata/quantified.txtar) | `TestFiles/quantified` and `TestQuantifiedRoundTrip` (both formatters) |
| AST scopes and cloning | [ast/astutil/testdata/quantified.txtar](../../ast/astutil/testdata/quantified.txtar) | `TestQuantifiedScopes`, `TestAliasAndOpenScopes` |
| Source exporter | [export/testdata/quantified](../../../internal/core/export/testdata/quantified/) | `TestQuantifiedIncompleteExport` |
| CLI defaults and version | [quantified_defaults.txtar](../../../cmd/cue/cmd/testdata/script/quantified_defaults.txtar) | `TestScript/quantified_defaults` |
| CLI certification | [quantified_vet.txtar](../../../cmd/cue/cmd/testdata/script/quantified_vet.txtar) | `TestScript/quantified_vet` |

The retained package path `cmd/cue/cmd` is the CLI library. Its executable is
`cmd/qcue`; these CLI fixtures invoke `qcue`.

## Reading and adding assertions

Each evaluator archive has an `in.cue` section. File-level assertions use `at=`
to select the value without changing the program's lexical scopes:

```cue
@test(json, [3, "hello"], at="out")
@test(validate, functions, at="id")
id(A): func(x: A) -> A: x
out: [id(3), id("hello")]
```

The directives express different observations:

| Assertion | Expectation |
| --- | --- |
| `@test(json, VALUE)` | JSON export succeeds and equals the expected concrete value. |
| `@test(validate)` | Ordinary validation succeeds; residual obligations are allowed. |
| `@test(validate, concrete)` | Values, closures, and runtime captures are concrete. |
| `@test(validate, functions)` | Function implementation contracts have universal proofs. |
| `@test(validate, concrete, incomplete)` | Ordinary validation succeeds, but materialization remains incomplete. |
| `@test(validate, functions, incomplete)` | Ordinary validation succeeds, but conformance remains unproved. |
| `@test(validate, conflict)` | Ordinary validation reports an established contradiction. |
| `@test(subsume, broad, narrow)` | The first path's value subsumes the second. |
| `@test(subsume, narrow, broad, fail)` | Subsumption fails in that direction. |

`validate` and `json` work either on a field or at file level with `at=`.
`subsume` takes two paths relative to the annotated value. Standard inline
`@test(eq, ...)`, error assertions, and other CUE txtar directives also work.
Formation errors prevent inline evaluation: those fixtures use the standard
`out/evalalpha` diagnostic golden and a documented `#inlinetest:exclude` marker.

API fixtures carry `#skip` only for the evaluator runner; their Go API tests
load them directly. Paper exclusions state their reason, and the paper index
test checks that every listing still matches the proposal exactly.

## Running the tests

From the repository root:

```sh
# Semantic fixtures, including the executable paper examples.
go test ./internal/core/adt -run TestEvalV3/quantified

# Paper integrity and Go API operations.
go test ./cue -run TestQuantified

# Syntax, both formatters, AST identities, and export errors.
go test ./cue/parser ./cue/format ./cue/ast/astutil ./internal/core/export \
  -run 'TestQuantified|TestAliasAndOpenScopes|TestFiles/quantified'

# qcue defaults, version information, opt-out, and certification.
go test ./cmd/cue/cmd -run TestScript/quantified

# Complete repository regression suite.
go test ./...
```

A narrower `-run` can select a topic or paper number, for example
`TestEvalV3/quantified/paper/035`. New semantic archives need explicit assertions.
When deliberately changing a golden expectation, use the repository's
`CUE_UPDATE=1` (fill missing output) or `CUE_UPDATE=force` (replace output), then
review the diff. JSON, validation, and subsumption assertions are authored
expectations and are never rewritten to make failures pass.
