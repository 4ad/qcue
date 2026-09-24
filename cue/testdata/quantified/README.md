# Quantified CUE tests

Start here for the supported **S_H** and **A** fragments. Language examples and
expectations live in txtar archives, grouped by topic. The ordinary CUE evaluator
runner discovers this directory automatically; no Go table needs updating when
adding a semantic case.

- [Examples](examples/): standalone programs with explicit expectations,
  including expected errors and incomplete specifications. Examples initially
  copied from the proposal evolve independently of its text and numbering.
- [API fixtures](api/): programs for refinement with `Unify` and `FillPath`,
  declaration ordering, source export, and closure completeness. Their Go
  assertions remain in [quantified_test.go](../../quantified_test.go) because
  these tests exercise Go API operations and object identity.
- [Generated semantic preservation checks](../../quantified_semantics_test.go):
  semantic regressions, independent finite membership and quantifier
  models, and transformations involving scopes, export, refinement and opacity.
- [Packet-domain model](../../../internal/core/adt/packet_domain_test.go):
  exhaustive comparison of symbolic protocol intersections against independent
  concrete packet admission in a finite vocabulary.

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
| [inference](inference/) | Variance, dependent bounds, empty arguments, and guarded speculative instances |
| [instantiation](instantiation/) | Implicit and explicit type application |
| [invalid_instances](invalid_instances/) | Rejected binder domains and invalid instances |
| [opaque_boundaries](opaque_boundaries/) | Abstract values, escaping operations, and opacity errors |
| [opaque_callbacks](opaque_callbacks/) | Transport of higher-order callbacks across a seal |
| [opaque_closure_escape](opaque_closure_escape/) | Captured abstract values and delayed escape checks |
| [opaque_composite_transport](opaque_composite_transport/) | Composite data, hidden fields, and complete overloaded interfaces across opaque boundaries |
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
| [validation](validation/) | Ordinary concrete validation of function contracts, defaults, nested values, and residual proofs |

## Other layers

Every additional corpus has `quantified` in its path or filename:

| Layer | Fixtures | Runner |
| --- | --- | --- |
| Parser | [parser/testdata/quantified](../../parser/testdata/quantified/) | `TestQuantifiedSyntax` |
| Formatter | [format/testdata/quantified.txtar](../../format/testdata/quantified.txtar) | `TestFiles/quantified` and `TestQuantifiedRoundTrip` (both formatters) |
| AST scopes and cloning | [ast/astutil/testdata/quantified.txtar](../../ast/astutil/testdata/quantified.txtar) | `TestQuantifiedScopes`, `TestAliasAndOpenScopes` |
| Source exporter | [export/testdata/quantified](../../../internal/core/export/testdata/quantified/) | `TestQuantifiedIncompleteExport` |
| CLI defaults and version | [quantified_defaults.txtar](../../../cmd/cue/cmd/testdata/script/quantified_defaults.txtar) | `TestScript/quantified_defaults` |
| CLI language versions | [quantified_versions.txtar](../../../cmd/cue/cmd/testdata/script/quantified_versions.txtar) | `TestScript/quantified_versions` |
| CLI certification | [quantified_vet.txtar](../../../cmd/cue/cmd/testdata/script/quantified_vet.txtar) | `TestScript/quantified_vet` |

The retained package path `cmd/cue/cmd` is the CLI library. Its executable is
`cmd/cue`; these CLI fixtures invoke `cue`.

## Reading and adding assertions

Most archives need no introductory description. When a note explains a
non-obvious expectation, write it as a `//` comment before the first section.
Keep runner directives on their own `#` lines, and format executable CUE inputs
with the CUE formatter.

Each evaluator archive has an `in.cue` section. File-level assertions use `at=`
to select the value without changing the program's lexical scopes:

```cue
@test(json, [3, "hello"], at="out")
@test(validate, concrete, at="id")
id(A): func(x: A) -> A: x
out: [id(3), id("hello")]
```

The directives express different observations:

| Assertion | Expectation |
| --- | --- |
| `@test(json, VALUE)` | JSON export succeeds and equals the expected concrete value. |
| `@test(validate)` | Ordinary validation succeeds; residual obligations are allowed. |
| `@test(validate, concrete)` | Values and runtime captures are concrete, and function implementations satisfy their declared contracts. |
| `@test(validate, concrete, incomplete)` | Ordinary validation permits the residual, but materialization or function conformance remains incomplete. |
| `@test(validate, conflict)` | Ordinary validation reports an established contradiction. |
| `@test(subsume, broad, narrow)` | The first path's value subsumes the second. |
| `@test(subsume, narrow, broad, fail)` | Subsumption fails in that direction. |

`validate` and `json` work either on a field or at file level with `at=`.
`subsume` takes two paths relative to the annotated value. Standard inline
`@test(eq, ...)`, error assertions, and other CUE txtar directives also work.
Formation errors prevent inline evaluation: those fixtures use the standard
`out/evalalpha` diagnostic golden and a documented `#inlinetest:exclude` marker.

Function contract checks use the same `concrete` demand as all other values.
There is no separate proof option. The [certification](certification/) cases
exercise the supported proof rules through ordinary concrete validation.

Semantic regressions cover retained universal obligations after selection and
source export, captured callbacks, partial closures, completed open protocols,
and opaque operation proofs. The opaque boundary cases exercise union transport
and omission defaults; the escape cases also refine optional fields, patterns,
and list tails after opening. Existential instances retain their captured
predicates, effect intersections retain shared admitted outcomes, and recursive
closure equality stays incomplete without overflowing the evaluator stack.
The [quantified fragment fixture](certification/quantified_fragment.txtar) records
both supported universal checks and the general Boolean predicates that remain
residual. Successful concrete calls alone are not certification assertions.

These regressions check membership, bounds, and lexical preservation:

| Obligation | Regression |
| --- | --- |
| Guard membership preserves the supplied packet's shape | [record_membership](capabilities/record_membership.txtar) |
| Type arguments prove inclusion in structural bounds | [structural_bounds](invalid_instances/structural_bounds.txtar) |
| An unknown callback row cannot establish complete call coverage | [open_callback_row](certification/open_callback_row.txtar) |
| Required results impose field presence, including top-valued fields | [required_results](certification/required_results.txtar) |
| Opened abstract types retain their representation universe | [opened_level](opaque_universe_boundary/opened_level.txtar) |
| Finite alias arguments satisfy their declared range | [value_ranges](parametric_aliases/value_ranges.txtar) |
| Export preserves shared code origins and distinct captures | [closure_export](api/closure_export.txtar) |
| Residual quantifier export retains lexical substitutions | [quantifier_export](api/quantifier_export.txtar) |
| Subtype binders admit unary bodies | [bound_unary_body](finite_witnesses/bound_unary_body.txtar), plus parser and formatter fixtures |

These regressions check sealing, certification, runtime identity, and export:

| Obligation | Regression |
| --- | --- |
| Existential membership preserves shape, presence, and closedness | [membership_shape](covariant_existentials/membership_shape.txtar), [API refinement](api/membership_refinement.txtar) |
| Sealing retains every interface conjunct, independent of order | [interface_conjunctions](seal_generativity/interface_conjunctions.txtar) |
| Builtin contracts require independent universal proofs | [builtin_contracts](certification/builtin_contracts.txtar) |
| Builtin capability guards preserve calls, labels, and defaults | [builtin_contracts](certification/builtin_contracts.txtar) |
| Opaque representations retain private function proof obligations | [opaque_representations](certification/opaque_representations.txtar) |
| Composite witnesses denote exact eventual singletons | [composite_singletons](certification/composite_singletons.txtar), [API refinement](api/membership_refinement.txtar) |
| Capture identity is independent of attached contracts | [captured_contracts](closure_identity/captured_contracts.txtar) |
| Distinct closures at one code origin can form a finite call chain | [finite_chain](closure_identity/finite_chain.txtar), [generative recursion](recursion_requires_descent/generative_chain.txtar) |
| Evaluated source export preserves attached builtin contracts | [builtin_export](api/builtin_export.txtar) |
| All export modes preserve opaque boundaries | [opaque_export](api/opaque_export.txtar), [exporter fixtures](../../../internal/core/export/testdata/quantified/) |
| Explicit type selection uses retained universal clauses | [attached_clauses](instantiation/attached_clauses.txtar), [selected_export](api/selected_export.txtar) |

Further regressions cover [ordinary open-record transport](opaque_composite_transport/open_records.txtar),
[erased type-parameter formation](api/erasure.txtar), and
[source export of callable capture graphs](api/closure_export.txtar).
The [profile boundaries](certification/profile_boundaries.txtar) fixture records
the remaining unsupported package eliminations and effect/foreign proofs;
these limitations must not become successful certification.

Further regressions cover call protocols, identity, and bounded evaluation:

| Obligation | Regression |
| --- | --- |
| Callable guard membership requires independent proof | [higher_order_guard_proof](capabilities/higher_order_guard_proof.txtar) |
| Certified calls respect partial and builtin protocols | [certification_protocol_test.go](../../certification_protocol_test.go) |
| Seal identity survives singleton and capture equality | [seal_identity](opaque/seal_identity.txtar) |
| Every retained telescope participates in universe checks | [attached_clauses](universes/attached_clauses.txtar) |
| Composite selection considers every admissible clause | [composite_clause_order](instantiation/composite_clause_order.txtar) |
| Aliases retain contextual witness decoding and code identity | [alias_witness_test.go](../../alias_witness_test.go) |
| Export preserves lexical predicates and contract environments | [lexical_export_test.go](../../lexical_export_test.go) |
| Finite enumeration retains exact residuals when bounded | [finite_expansion_test.go](../../finite_expansion_test.go) |

The small Go API tables exercise refinements and export/reimport in fresh
scopes. Resource tests generate large binder products and check residual
semantics, without relying on timing thresholds.

The following regressions check interactions between these features:

| Obligation | Regression |
| --- | --- |
| Parametric aliases preserve witness decoding, erasure, and per-use index guards | [alias_witness_test.go](../../alias_witness_test.go), [erasure](api/erasure.txtar) |
| Completed calls retain actual callback conformance obligations | [certification_protocol_test.go](../../certification_protocol_test.go) |
| Nested packages preserve seals and cannot hide free outer abstract dependencies | [nested_packages](opaque_composite_transport/nested_packages.txtar), [opaque_transport_test.go](../../opaque_transport_test.go) |
| Callback round trips preserve identity through direct, composite, and partial transport | [round_trip_identity](opaque_callbacks/round_trip_identity.txtar), [round_trip_containers](opaque_callbacks/round_trip_containers.txtar) |
| Residual predicate export preserves schemas and runtime captures preserve hidden fields | [lexical_export_test.go](../../lexical_export_test.go), [foreign_hidden_capture](../../../internal/core/export/testdata/quantified/foreign_hidden_capture.txtar) |
| Composite introductions, selected views, and shared copies survive source export | [composite_export_test.go](../../composite_export_test.go), [lexical_selection](composite_instances/lexical_selection.txtar), [mixed_composite](../../../internal/core/export/testdata/quantified/mixed_composite.txtar) |
| Shared proof graphs reuse completed evidence under compatible hypotheses and bound work | [certify_work_test.go](../../../internal/core/subsume/certify_work_test.go) |

The proof-work tests count deterministic steps, require incomplete results on
budget exhaustion, and retry independently. Composite export tests make new
selections after recompilation and check unsupported mixed graphs through the
typed incomplete-export API. Negative opacity cases distinguish a free outer
dependency from a legal private witness bound by an independent inner seal.

Additional regressions cover inference, transport, and retained obligations:

| Obligation | Regression |
| --- | --- |
| Saved callbacks satisfy the required domain as well as their own contracts | [certification_partial_test.go](../../certification_partial_test.go) |
| Inference tracks variance and dependent bounds; failed guesses cannot discard guards | [variance_bounds](inference/variance_bounds.txtar), [guarded_instances](inference/guarded_instances.txtar) |
| Opaque adapters retain every clause, result guard, and coherent transport | [overloaded_operations](opaque_composite_transport/overloaded_operations.txtar), [overloaded_abstract](opaque_composite_transport/overloaded_abstract.txtar) |
| Complete interfaces survive callbacks, generic selection, partial calls, labels, and defaults | [overloaded_callbacks](opaque_composite_transport/overloaded_callbacks.txtar), [overloaded_generic](opaque_composite_transport/overloaded_generic.txtar), [overloaded_protocols](opaque_composite_transport/overloaded_protocols.txtar) |
| Runtime identity ignores contracts while validation retains their obligations | [singleton_contracts](closure_identity/singleton_contracts.txtar), [recursive_singletons](closure_identity/recursive_singletons.txtar), [runtime_identity](opaque/runtime_identity.txtar) |
| Ordinary record transport preserves hidden and definition labels | [hidden_records](opaque_composite_transport/hidden_records.txtar) |
| Escape checks follow retained predicates, and export retains erased opened names | [retained_predicates](opaque_closure_escape/retained_predicates.txtar), `TestQuantifiedOpaquePredicateExport` in [quantified_test.go](../../quantified_test.go) |
| Unary proofs cover full Boolean and numeric domains without accepting invalid promises | [certification_unary_test.go](../../certification_unary_test.go) |

API fixtures carry `#skip` only for the evaluator runner; their Go API tests
load them directly. Syntax-only cases and malformed syntax belong in the parser
corpus. No test compares fixtures with the proposal document.

## Larger oracles and fuzzing

The [oracle guide](../../../doc/oracle.md) describes independent
models, their exact coverage, preservation transformations, residual judgments,
seeded generation, and failure shrinking. The default suite stays small;
`tools/test-quantified-oracles.sh extended` runs larger exhaustive universes
and five native fuzz targets. A weekly workflow runs the same command.

## Running the tests

From the repository root:

```sh
# Semantic fixtures, including the standalone examples.
go test ./internal/core/adt -run TestEvalV3/quantified

# Go API operations.
go test ./cue -run TestQuantified

# Syntax, both formatters, AST identities, and export errors.
go test ./cue/parser ./cue/format ./cue/ast/astutil ./internal/core/export \
  -run 'TestQuantified|TestAliasAndOpenScopes|TestFiles/quantified'

# cue defaults, version information, opt-out, and certification.
go test ./cmd/cue/cmd -run TestScript/quantified

# Complete repository regression suite.
go test ./...
```

A narrower `-run` can select a topic or example name, for example
`TestEvalV3/quantified/examples/a-polymorphic-callback`.
New semantic archives need explicit assertions.
When deliberately changing a golden expectation, use the repository's
`CUE_UPDATE=1` (fill missing output) or `CUE_UPDATE=force` (replace output), then
review the diff. JSON, validation, and subsumption assertions are authored
expectations and are never rewritten to make failures pass.
