# Quantified CUE semantic oracles

These tests compare the implementation with independent finite mathematical
models and check preservation under source transformations. The default suite
is bounded; the extended suite increases finite universes and runs native Go
fuzzing. The models do not call implementation inference or subsumption helpers
to compute their expected results.

## Models

The finite Boolean oracle folds relations with Go conjunction and disjunction,
including empty domains, every quantifier order, and domains depending on prior
finite assignments. A separate renderer elaborates the finite assignments into
ordinary Boolean CUE formulas. This preserves the original mathematical tests
without reintroducing removed value-binder syntax. Cardinality and duality
properties check the model itself; a three-value property distinguishes its
larger universe from the two-value one.

The default suite retains all 1,024 original two-variable cases. Extended runs
exhaust the 131,072 cases in each larger independent-domain vocabulary and
122,880 dependent-domain cases. Direct subtype-bound tests separately exercise
all 512 subset chains over `{0,1,2}` using universal type parameters.

Record models cover required and optional fields, regular and qualified labels,
conjunction order, compatible incomplete constraints, and later refinement.
The call-packet model independently computes completion of supplied fields under
parameter constraints. Direct calls, attached contracts, and saved partial
packets must produce that completed packet. In contrast, a return annotation
must prove the independently constructed body; it cannot add a missing field.
Contradictory packets fail, while ambiguous demanded packets remain incomplete.

The packet-domain model compares 28,561 ordered protocol pairs over 12 packet
shapes in the default suite. The extended vocabulary compares 16,875,664 pairs
over 32 shapes. Independent admission bitsets and slot mappings check positional
and named bindings, omissions, defaults, reordered slots, and duplicate-binding
rejection. This model concerns universal calling capability, independently of
call-local data completion.

Feature combinations use identity implementations, whose valid promises are
finite-set inclusions. The switches cover hidden fields, record/list nesting,
partial application, declaration order and alpha renaming, export/reimport,
and invoking versus ignoring a callback. Positive and negative controls remain;
the removed opaque-package dimension is no longer generated.

Boundary regressions retain predicate constraints, scope, runtime closure
identity, selected domains, and complete callback intersections. Live-input
refinement and explicit assertions also have direct API and paper-example
coverage. Removed existential package and transport operations are covered by
parser rejection tests rather than a simulated compatibility layer.

## Preservation and shrinking

Transformations include Boolean regrouping and reordering, redundant meets,
record and list copies, `Unify`, `FillPath`, and ordinary and final source export
followed by compilation. Each transformed result is compared with the independent
oracle, not merely with the original evaluator result.

Failures report the generated source and model parameters. Semantic shrinking
removes relation tuples, domain members, dependencies, feature switches, or
packet parameters while preserving the mismatch. These are local reductions,
not claims of globally minimal programs. Native Go fuzzing supplies additional
coverage-guided generation and corpus minimization:

- `FuzzQuantifiedFiniteOracle`
- `FuzzQuantifiedPreservation`
- `FuzzQuantifiedFeatureCombinations`
- `FuzzQuantifiedPacketDomain` (in `internal/core/adt`)

Ordinary tests use a fixed seed, printed with `-v`.
`CUE_QUANTIFIED_ORACLE_SEED` selects another reproducible sample. Native fuzzing
uses Go's generator; a saved corpus input is its reproducer and is replayed by
ordinary `go test`. Keep minimized failures and add readable regressions when
repairing them.

## Running

```sh
# Default finite models and preservation checks.
tools/test-quantified-oracles.sh fast

# Extended exhaustive models followed by four 30-second fuzz runs.
tools/test-quantified-oracles.sh extended

# A different deterministic seed and longer fuzzing.
CUE_QUANTIFIED_ORACLE_SEED=0x1234 CUE_QUANTIFIED_FUZZ_TIME=5m \
  tools/test-quantified-oracles.sh extended

# One extended model.
CUE_QUANTIFIED_ORACLE=extended go test ./cue \
  -run '^TestQuantifiedOracleExhaustive$' -count=1 -v

# Replay a saved fuzz case.
go test ./cue -run 'FuzzQuantifiedFiniteOracle/HASH' -count=1 -v

go test ./...
```

`CUE_QUANTIFIED_FUZZ_WORKERS` controls native fuzz parallelism (default 2).
The runner has finite timeouts and stops on failure. The
[workflow](../.github/workflows/quantified-oracles.yml) runs the fast suite on
pushes and pull requests, schedules the extended suite weekly, and supports
manual dispatch with a seed. Logs and minimized failures are retained as
artifacts when that workflow is installed on the default branch.

These suites establish results for their finite vocabularies and observations.
They do not establish completeness of arbitrary proof search or enumerate every
interaction of open constraints and higher-rank types.
