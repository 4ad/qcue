# Quantified CUE: implementation and use

This fork implements the constraint-propagation design in version 9 of the
[paper](paper.pdf) ([source](paper.tex)). Universal types and functions are
available by default at every CUE language version, including standalone files
and Go API calls. The module path remains `cuelang.org/go`.

```sh
go install ./cmd/cue
cue version
```

The executable is `cue`. Its version and experiment help identify universal
types and live constraint propagation. `@experiment(quantified)` remains an
optional explicit enablement. `@experiment(quantified=false)` selects the
underlying function experiment; `@experiment(functions=false,quantified=false)`
disables both extensions.

Existentials, opaque packages, `seal`, `open` package elimination, value-range
binders, and universe-level syntax have been removed. A type parameter may
still have a subtype bound referring to an earlier type parameter.

## Universal subjects and abbreviations

```cue
id(A): func(x: A) -> A: x
out: [id(3), id[string]("hello")]
Box(A) = {value: A}
item: Box(int) & {value: 3}
```

`id(A): ...`, `id: forall A ...`, and `id: func<A>(...) -> ...` introduce
universal binders. Binder identity is lexical; shadowing does not identify two
parameters with the same spelling. `A: number` supplies a subtype bound.
Quantified types may themselves be instance arguments, including polymorphic
callbacks. Explicit selection retains the implementation's original universal
obligations and the selected view's calling domain.

An alias such as `Box` is a description abbreviation and creates no subject.
In contrast, `box(A): {value: A}` requires one subject to satisfy every instance,
including the empty type. Quantified records and lists retain shared subjects,
scopes, and their original obligations through projection and selection.

Inference collects lower and upper requirements with the appropriate variance.
Repeated variables, nested callbacks, dependent subtype bounds, empty lists,
and multiple selected views have regression coverage. Inference is sufficient
rather than complete: an unsuccessful candidate leaves an obligation pending.
Type parameters are erased proof variables and cannot be returned as runtime
values or used as ordinary data operands.

## Live descriptions and fresh packets

```cue
Live: int
scale: func(x: Live) -> int: 2 * x
answer: scale(2)
```

`Live` is an open description coordinate. Its integer upper bound proves the
arithmetic body; it does not prove that every integer belongs to `Live`.
The call creates a fresh packet constrained by both `2` and `Live`. Refining
`Live` to `>=0` preserves `answer: 4`; refining it to `>2` refutes that invocation.
The packet never writes `2` back into the declaration.

Parameter constraints can complete an open record or select a compatible data
alternative. They remain attached to saved partial arguments and ignored
operands. A contradictory packet is an ordinary failing computation. An
unfinished demanded packet remains incomplete, even when the body is constant.
Higher-order packet components also require independent implementation and
coverage evidence; conjoining an arrow cannot manufacture that evidence.

```cue
R: int
same: func(x: R) -> R: x
constant: func() -> R: 2
checked: func(x: int) -> R: x & R
```

The first and third bodies have result evidence. `constant` remains pending
until the store proves the opposite inclusion, for example by adding `R: 2`.
Adding `R: >2` instead supplies a result counterexample. A result annotation is
a proof goal and cannot construct fields, choose alternatives, or solve cycles
in the body. The explicit assertion in `checked` is an executable constraint.

A fixed alias such as `let Fixed = int` is a fixed predicate. A reference inside
an alias still retains its live coordinate. Equal current record data does not
prove inclusion into an open result description: future constraints may add
fields. Concrete scalar and closed scalar-list singletons admit stronger facts.

Required field descriptions are available before their values are supplied.
For example, `z!: int` supports the body proof of `func(x: int) -> int: x+z`,
but the closure and its calls still demand a supplied, valid `z`. Lexical
references and selections use the same description lookup. Optional fields
remain unavailable until presence is established.

Record meets collect all source declarations into a common field scope before
checking methods. References inside a copied record see its sibling refinements;
external references keep their original scope. This also applies through named
records, embeddings, and aliases. Presence and live description dependencies
survive collection. For closed records, the checker replays the source's
structural operators through the evaluator with checked field summaries, so
definitions and embeddings retain ordinary CUE closedness. Unsupported record
forms keep the existing conservative proof path.

## One propagation graph

The implementation retains the paper's five components:

| Component | Representation |
| --- | --- |
| Exact source relation | Vertices, conjuncts, environments, activation packets, and original expressions |
| Facts and bounds | Scoped descriptions, live references, membership facts, and synthesized body summaries |
| Certificates | Function/body derivations, separate coverage goals, and explicit hypothesis support |
| Obligations and observations | Retained goals, live-vertex dependencies, and relevance traversal triggers |
| Enabled work | The propagation queue and its shared work allowance |

Evaluation and proof production use the same graph. Evaluation may establish a
capture or description bound needed by a proof. A grounded core derivation,
protocol proof, and current observation pass enable an invocation. Activations
retain that evidence; there is no static-phase flag or replacement of evaluator
hooks with a second checker.

Each derivation has an assumption store. Queued work restores the store in which
it was installed. A callback assumption can justify a client body, but must be
discharged by the supplied callback's own certificate when linking the packet.
A body cannot use its pending result annotation as its own premise. Checked
inductive rules justify recursive partial-correctness proofs; membership in a
worklist cycle alone supplies no proof.

Goals distinguish established evidence, checked rejection, pending work, and a
quiet observation pass. Quietness is a completed search status, not a theorem.
New information wakes dependent goals and invalidates quiet passes. Persistent
certificates retain their target, scope, and support. Unsupported search does
not become semantic bottom and cannot discard a union alternative.

Proof work, region expansion, and evaluator callbacks consume the shared
allowance. Exhaustion retains the obligations. Adding work resumes the graph
and reuses proved certificates. Individual search attempts also have a finite
allowance; renewed attempts may receive more work. Runtime recursion requires
finite structural descent before executing another recursive activation.

## Capabilities and observations

Attaching `A -> B` to an implementation with domain `C` produces two distinct
goals: coverage `A <= C`, and a body theorem at result `B` under input `A`.
A successful call-local packet meet supplies neither theorem. All source
requirements survive explicit selection, partial application, copying, and
closure export. Defaults and label/omission rules belong to the implementation's
protocol; an annotation does not insert an adapter.

Explicit interfaces retain their observation roots through normalization.
The finite kernel checks equal-domain and overlapping arrow regions, ground
conflicts, field presence, tuples, constant intervals, rigid universals, and
independently anchored instances. An empty result creates an input-emptiness
demand. A verified witness in that region supplies a relevance refutation.
An unresolved live input remains pending. Candidate witnesses are checked by
membership and disjointness proofs, never accepted from upper bounds alone.

Direct literal and parameter-return bodies can also retain result
counterexamples. Other unproved bodies remain pending when the implemented
finite rules cannot decide them. Negative checking evidence is a diagnostic
about a scoped requirement; the corresponding partial arrow is not rewritten
to the empty semantic type.

Bodyless declarations supply conditional invocation evidence and result
constraints. They do not materialize an implementation. The paper's native
adapter example is tested as a conditional declaration: logical rejection is
available, while execution remains incomplete without a linked implementation.
Every registered native builtin supplies an implementation-owned parameter and
successful-result contract. The checker uses that contract for direct calls,
aliased calls, and attached function interfaces. Native arguments must be
covered by the declared parameter descriptions, including labels and defaults;
an incompatible argument cannot become an empty packet that proves any result.
Client annotations remain independent obligations. A foreign function with no
native contract still needs linked implementation evidence.

## Builtins and operators

Native result kinds are conservative bounds on successful returns. Generated
checking signatures additionally retain Go container elements, nested maps and
lists, and converted record fields, including optional JSON fields. Frozen
package declarations retain their own lexical environments. This checking
evidence is separate from runtime argument constraints: it must not complete
an unknown argument or change an incomplete native call into a value. Bare
validators support their ordinary call forms; implicit validator constructors
check the saved arguments and describe inhabitants of the validated parameter's
type. These contracts do not promise success, termination, or concrete operands.

For implementation-declared pure natives, finite concrete input descriptions
also provide exact successful results. This covers scalar alternatives and
closed lists of known values, as well as constructed or supplied records.
Record predicates alone do not determine serialization: even a closed record
type does not specify field order. Schema-consuming natives retain their
declared contracts instead of treating schema inhabitants as the schema itself.
The generator records package purity in the native descriptor; client CUE
attributes cannot authorize proof-time execution. Enumeration uses a fraction
of the proof work allowance, preserving the declared contract when the finite
domain or a requested repetition, shift, formatting precision, or numeric range
is too large. Direct calls and attached interfaces use the same rule.

Native checking signatures can retain universal input-output relationships and
conditional result refinements. For example, `math.Abs` admits every number
while preserving integer results for integer inputs. Such a refinement adds
result evidence after its narrower domain is covered; it does not restrict the
primary call domain. Published package interfaces are checked against this
implementation-owned evidence.

List transformations additionally preserve known tuple positions and length
bounds. `FlattenN` uses the supplied depth, while `Take` and `Drop` account for
clamping at the actual input length. An open tail contributes possible elements
without promising that any are present. Large finite shape expansions can fall
back to the generic element contract within the proof budget. Sort templates
are checked with `x` and `y` supplied by the input element description; a known
empty or singleton list has no comparator invocation. These rules also check
attached interfaces and do not execute symbolic inputs as concrete lists.

The structural primitives have additional rules. `len` preserves known length
bounds. `close` preserves input constraints while adding closedness where its
record description is known; a rigid or live input retains its identity.
`and` meets the guaranteed list prefix, and `or` joins every possible element,
including an open tail. These same rules check attached universal interfaces.

```cue
merge(A): func(xs: [A, ...A]) -> A: and(xs)
x: merge([{a: 1}, {b: 1}]) // {a: 1, b: 1}
choose(A): func(xs: [...A]) -> A: or(xs)
```

The nonempty input in `merge` is necessary: `and([])` returns `_`, which does
not belong to every `A`. Thus `func(xs: [...A]) -> A: and(xs)` remains unproved.
An optional tail cannot narrow the result of a conjunction because it may be
absent. The empty disjunction has no successful result; accepting its partial
result contract does not materialize a runtime value.

Every unary and binary operator checks its operand domains. Alternative
operands are checked independently, with source failures kept distinct from
unsupported operations. Regex matching, structural equality, string and bytes
operations, symbolic bounds, and interpolation participate in the same rules.
Numeric operations use upper bounds without claiming to preserve an arbitrary
numeric subtype. For example, `+x` preserves `A: number`, but `-x` need not.

List indexing with an integer index retains all possible element types; an
out-of-range index can fail. Finite record-label alternatives each require a
known field. List and bytes slicing check their bounds and preserve available
element information. Comparisons of open record descriptions yield Boolean
results without treating those descriptions as complete singleton values.

## Validation, refinement, and export

`Value.Err()` reports evaluation failures. `Value.Validate()` additionally
checks explicit observations and supplied function bodies, including uncalled
bodies. Missing or rejected proof goals are reported even without
`cue.Concrete(true)`. Concrete validation additionally demands completed data,
linked implementations, and runtime captures. `cue vet -c=false` still reports
undischarged proof obligations.

`Unify`, `FillPath`, source export, and reimport preserve the dependencies that
can be refined later. A derived scalar is not permission to forget its argument
packet or live result constraint. Source reconstruction retains code origins,
lexical substitutions, captured values, defaults, and original obligations.
Shared code declarations may be hoisted, but environments containing exposed
captures are instantiated in their lexical records. Export and reimport must
preserve independent refinement of copies, including templates with required
captures that have not yet been supplied.
Standalone closure export fails explicitly when a required capture cannot be
represented faithfully. JSON output has no representation for a function or an
unresolved implementation.

## Code and verification

- `internal/core/adt/propagate.go` owns goals, dependencies, suspension, and work.
- `internal/core/subsume/propagate.go` installs scoped inference propagators.
- `internal/core/subsume/infer*.go` contains the finite body derivation rules.
- `internal/core/subsume/capability.go` separates activation from coverage.
- `internal/core/subsume/relevance*.go` and `refutation.go` retain observations
  and checked negative evidence.
- `internal/core/adt/live.go`, `expr.go`, and `call_contract.go` retain live
  references, fresh activations, and saved packet constraints.
- `internal/core/compile/quantified.go` and `erasure.go` establish lexical type
  scopes and check erased variables.
- `internal/core/export` reconstructs faithful source and closure environments.

Every listing in the current paper is indexed and reproduced verbatim in the
[paper catalogue](../cue/testdata/quantified/paper/README.md). Syntax and
pseudocode listings are distinguished from executable examples; the malformed
call example has a compile-error check. Still-valid examples from older paper
versions remain in `paper_history`.

API regressions exercise later refinement, observation triggers, independent
coverage, packet isolation, strict operands, closure identity, and repeated
export. The native inventory test checks every registered builtin, rejects
incompatible argument slots and unproved results, and checks validator forms.
Operator matrices cover accepted and rejected operand domains; execution tests
check the resulting values. Kernel tests inspect refutation support and resume
real proofs after budget exhaustion. [Independent oracles](oracle.md) cover
finite semantics and preservation separately from these example tests.

```sh
go test ./cue ./internal/core/...
tools/test-quantified-oracles.sh fast
go test ./...
```
