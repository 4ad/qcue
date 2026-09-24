# Quantified CUE semantic oracles

These tests compare the implementation with small independent mathematical
models, and check that specified program transformations preserve observations.
The default suite remains small. The extended suite enlarges the finite
universes and runs coverage-guided fuzzing with replayable minimized failures.

## Coverage and interpretation

| Model | Default suite | Extended suite |
| --- | --- | --- |
| Binary finite quantifiers on two values | All 1,024 cases, retained from the original suite | Same |
| Binary finite quantifiers on three values | Generated samples and a three-value distinguishing property | All 131,072 cases |
| Three finite quantifiers on two values | Generated samples | All 131,072 cases |
| Dependent finite domains | 480 cases covering every prefix/domain combination in the vocabulary | 122,880 cases, also exhausting all ternary Boolean relations on two values |
| Dependent subtype bounds | All 512 chains of subsets of `{0,1,2}` | Same |
| Record membership | 2,592 cases on two values | 25,088 cases on three values |
| Packet-domain intersection | 28,561 ordered row pairs, 12 packet shapes each | 16,875,664 ordered row pairs, 32 packet shapes each |
| Seeded random finite models | 128 | 4,096 |
| Generated preservation transformations | 48 models × 10 transformations | 512 models × 10 transformations |
| Feature combinations | 128 sampled combinations, with positive and negative controls | All 6,272 combinations in the defined vocabulary |

The packet counts correspond to 342,732 and 540,021,248 packet comparisons.
The tests precompute independent admission bitsets rather than rerunning the
oracle for every pair. They also check the parameter mapping of every admitted
symbolic family on both sides; agreement on the set of packet shapes alone
would miss a wrongly connected transport slot. The extended rows have up to
three parameters and three labels, including positional/name-only modes,
omission, defaults, reordered parameters and duplicate-binding rejection.

The finite-quantifier oracle folds Boolean relations with Go conjunction and
disjunction. It includes every quantifier order and every independent subdomain,
including empty domains. No CUE inference, normalization, binding or subsumption
helper computes the expected answer. The renderer is separate from the model.
Known cardinality and quantifier-duality properties check the model itself.
One property specifically requires three values: for every `x,y`, a value
exists that differs from both.

Dependent finite domains can contain preceding values or exclude them. The
innermost range can refer to both enclosing binders. Direct dependent
value-range syntax is outside the implemented profile, so these tests use its
finite guarded elaboration over a nonempty literal universe. This exercises
scope and witness dependence without pretending profile D is implemented.
Dependent subtype chains `B: A, C: B` are tested directly through explicit
selection and every admitted concrete argument. Admitting a type selection is
not itself proof of its entire retained infinite universal contract.

The finite Boolean fragment has exact answers: both ordinary validation and
concrete observation must match the oracle. Incompleteness fails those tests.
The membership model distinguishes conflict, compatible incomplete descriptions,
and complete membership. Required/optional presence, all four field classes,
and both conjunction orders are retained from the original model.

Feature combinations use identity implementations, whose membership in
`D -> R` is exactly finite-set inclusion `D ⊆ R`. The seven independent switches
cover hidden fields, record/list nesting, partial application, opaque packages,
declaration order and alpha renaming, consumer export/reimport, and invoking
versus ignoring the callback. Explicit selection, dependent type bounds,
copying, public alias equations, and rejection of distinct public identities
are checked along these paths. The private implementation deliberately shares
code across distinct public exports.

Opaque transport still has a documented proof limit for unions of overlapping
source kinds. Valid cases encountering that limit may remain incomplete;
they are counted separately and never counted as proved. The extended matrix
currently produces 1,984 established cases, 3,840 rejected invalid promises,
and 448 residual cases. Invalid promises may never certify or produce an
unchecked result. Every supported positive case must produce the expected
identity result; rejecting everything would fail the suite.

## Preservation and shrinking

Finite-model transformations include alpha renaming under hostile outer names,
reordering Boolean factors and domain alternatives, redundant/reordered meets,
record and list copies, `Unify`, `FillPath`, and both ordinary and final source
export followed by compilation in a fresh context. Each transformed result is
checked against the independent oracle, not just against the original CUE
result. Two identically wrong evaluations therefore do not make a test pass.

Failures include CUE input and model parameters; preservation failures also
identify the transformation needed to reproduce the observation. Greedy
semantic shrinking removes relation tuples, domain members and dependencies
while preserving the actual mismatch. Feature failures additionally remove
feature switches; packet failures remove parameters. These are local reductions,
not claims of globally minimal programs. Four native Go fuzz targets add
coverage-guided generation and automatic corpus minimization:

- `FuzzQuantifiedFiniteOracle`
- `FuzzQuantifiedPreservation`
- `FuzzQuantifiedFeatureCombinations`
- `FuzzQuantifiedPacketDomain` (in `internal/core/adt`)

Normal tests use a fixed seed, printed with `-v`. Set
`CUE_QUANTIFIED_ORACLE_SEED` to a decimal or hexadecimal integer for another
reproducible run. Native fuzz exploration uses Go's own generator; its saved
corpus input is the reproducer. A failing corpus file under `testdata/fuzz` is
replayed by ordinary `go test`. Keep the minimized input and add a readable
semantic regression when repairing a failure.

## Running and scheduling

From the repository root:

```sh
# Fast oracle suite, including all existing semantic preservation checks.
tools/test-quantified-oracles.sh fast

# Extended exhaustive models followed by four 30-second fuzz runs.
tools/test-quantified-oracles.sh extended

# Longer fuzzing and a different deterministic model sample.
CUE_QUANTIFIED_ORACLE_SEED=0x1234 CUE_QUANTIFIED_FUZZ_TIME=5m \
  tools/test-quantified-oracles.sh extended

# A particular extended model without fuzzing.
CUE_QUANTIFIED_ORACLE=extended go test ./cue \
  -run '^TestQuantifiedOracleExhaustive$' -count=1 -v

# Replay a native fuzz failure (replace HASH with the saved corpus filename).
go test ./cue -run 'FuzzQuantifiedFiniteOracle/HASH' -count=1 -v

# Full repository regression suite; includes the fast tests and fuzz seeds.
go test ./...
```

`CUE_QUANTIFIED_FUZZ_WORKERS` controls native fuzz parallelism (default 2).
The script has finite test timeouts and exits on the first failed command.
Invalid mode names fail instead of silently skipping the extended suite.

The [GitHub workflow](../.github/workflows/quantified-oracles.yml) runs the fast
suite on pushes and pull requests. It schedules the extended runner weekly,
Mondays at 03:17 UTC, and supports manual dispatch with an optional seed.
Scheduled runs use their run number as the deterministic
sample seed. Logs and minimized failing corpus files are retained as artifacts.
This configuration takes effect when it reaches the repository's default branch
and Actions is enabled; adding the file locally does not activate a remote run.
It uses the documented interfaces of [checkout](https://github.com/actions/checkout),
[setup-go](https://github.com/actions/setup-go), and
[upload-artifact](https://github.com/actions/upload-artifact).

## Finding from the larger oracle

Three-valued, three-binder randomized cases exposed a finite-normalization
blow-up. Distinct scoped pending alternatives formed an exponential Cartesian
product even when their evaluated results were equal. The finite evaluator now
finalizes each assignment and deduplicates proved-equal ground data only when
its captures are immutable finite binder assignments. It preserves ordinary
captured witnesses, callable contracts, optional constraints, patterns, open
list tails, and retained introductions. The slow generated inputs are permanent
regressions and fuzz seeds; refinement tests check that normalization retains
optional-field, pattern and list-element correlations.

These suites establish results for the finite vocabularies and supported
observations described above. They do not exhaust all three-valued ternary
relations, unrestricted dependent types, arbitrary program syntax, or every
possible interaction of the extension.
