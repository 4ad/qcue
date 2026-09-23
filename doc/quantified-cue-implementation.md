# Quantified CUE: implementation and use

This implementation supports fragments of profiles **S_H** (predicative
higher-rank quantification) and **A** (opaque existential packages) in
[the proposal](quantified-cue.tex). The `quantified` experiment is **enabled by
default** for CUE language version `v0.18.0` and later, including standalone files
and Go API calls with no pinned language version.

Build and install from this checkout:

```sh
go install ./cmd/qcue
qcue version
```

The executable is `qcue`, so it can coexist with upstream `cue`. Its version
output identifies the build, supported CUE language version, S_H and A
extensions, and default activation. The CLI library keeps the import path
`cuelang.org/go/cmd/cue/cmd`.

The experiment includes function syntax and the proposal's capability semantics.
The explicit `@experiment(quantified)` attribute remains accepted but is not
required. To use the previous experimental function semantics in a file, write
`@experiment(functions,quantified=false)`; `@experiment(quantified=false)` alone
opts out of the quantifier extension. Modules pinned to earlier language
versions retain their earlier defaults.

General value-dependent binders and the dependent profile **D** are out of scope.
Finite literal value ranges, such as `exists (n in 1 | 2)`, are supported as finite
unions or intersections.

For a new module, use
`qcue mod init --language-version v0.18.0 example.com/quantified`. For an existing
module, set its language version with `qcue mod edit --language-version v0.18.0`.

## Quantifiers and type application

A quantified declaration constrains **one subject** at every type instance:

```cue
id(A): func(x: A) -> A: x
out: [id(3), id[string]("hello")]
```

`id(A): ...`, `id: forall A ...`, and `id: func<A>(...) -> ...` introduce lexical
universal binders. `forall` and `exists` can occur inside signatures and records.
Quantifier blocks at the beginning of a record bind the rest of that record.
Binder names may be shadowed; identity follows their declarations, not spelling.

`A: number` is a subtype bound. `A in Type(0)` supplies an explicit predicative
universe; a quantifier over `Type(n)` lives above level `n`. Ordinary data and
monomorphic function types inhabit the base universe. Invalid level bounds and
self-instantiation cycles are rejected. Unknown inferred universe relationships
remain incomplete; an explicit level annotation can make a higher-rank boundary
checkable.

A parametric alias abbreviates a description and creates no subject:

```cue
Box(A) = {value: A}
item: Box(int) & {value: 3}
```

In contrast, `box(A): {value: A}` requires one value belonging to every admissible
`A`, including the empty type, and is contradictory. `empty(A): [...A]` describes
the empty list. Type selection also applies to quantified composite subjects:
`module[int].operation(...)` retains the module's data and universal obligations.
Selecting a function instance likewise retains its original universal contract;
certifying one selected instance cannot certify an invalid generic body.
Selection also considers universal clauses attached to a separately supplied
implementation. Each selection consumes one binder of the selected clauses;
the implementation's original call protocol and universal obligations remain.
Calls with empty containers or unused binders can infer the empty predicate,
provided the selected instance admits every supplied argument.

Type-sorted names denote predicates. An ordinary refinable field used in a
signature denotes its eventual singleton. For example, `x: int` and
`f: func(x) -> string` describe a function that accepts the particular eventual
value of `x`. The current approximation `int` cannot discharge that obligation.
For records and lists, the singleton includes the entire eventual data shape;
it is not an open structural predicate that admits additional fields.

Erased type parameters may appear in signatures, type selections, and seal
witnesses. They cannot supply runtime return values, arguments, defaults, or
ordinary data indexes. A bound such as `A: int` does not make `A` an integer
argument. The compiler rejects these runtime uses, including indirect uses
through local aliases. Finite literal value binders retain their selected values
as runtime captures and are distinct from erased type binders.

## Functions, refinement, and checking

Conjoining function contracts retains every guarded capability clause. An
implementation keeps its original labels, defaults, omitted-argument behavior,
and extra-argument policy. Applicable clauses constrain its actual call result.
An unresolved applicability guard remains an obligation.

Builtins obey the same rule. Their package declarations define their original
protocol; adding a client contract cannot install a default or rename a slot.
Attached contracts require an independent conformance proof. The current rules
cover `len` and the total string primitives `ToUpper`, `ToLower`, `ToTitle`,
`Compare`, `Contains`, `ContainsAny`, `HasPrefix`, and `HasSuffix`. Supported
inclusion proofs and exhaustive singleton scalar packets can discharge a clause;
other builtin promises remain incomplete. Source export retains client clauses.

Concrete closures compare by code origin, captured runtime values, and bound
partial arguments. Type arguments are erased. Two different bodies are different
implementations, even if they happen to return equal results on tested inputs.
Two copies of one closure retain their identity. Unknown capture equality stays
incomplete until refinement settles it.
Adding contracts to a captured function does not change its runtime identity,
including when it is nested in a captured record, list, or partial argument.
Those contracts remain separate validation obligations.

Higher-rank callback contracts can be checked with rigid type variables. A
polymorphic callback can be instantiated independently at its uses; a monomorphic
callback is not silently generalized. Ground calls support finite list
comprehensions and recursive calls with a demonstrated decrease in one fixed
finite list argument. Other recursive calls retain cycle or incomplete errors.
Finite acyclic chains of captured closures can execute and be certified even
when their distinct instances share one function literal.

Validation distinguishes retaining constraints from requiring a complete value:

| Request | Meaning |
| --- | --- |
| `value.Validate()` | Report established contradictions; allow residual obligations. |
| `value.Validate(cue.Concrete(true))` | Require materialized values, complete runtime captures, and function implementations proved to satisfy their declared contracts for arbitrary admitted inputs. |

`qcue vet file.cue` requires concrete validation, including function type
conformance. An unresolved contract makes validation incomplete; use `qcue vet -c`
to see the detailed errors. There is no separate function-verification flag.
`qcue vet -c=false` explicitly permits incomplete constraints for further
refinement and does not establish that every retained implementation satisfies
its type. Definitions and absent optional fields remain schemas until demanded
as ordinary values, following CUE's usual concreteness rules.

The conformance checker handles annotated structural bodies, higher-rank
arguments, records, lists, projections, finite comprehensions, supported pure
primitives, and defaults proved to belong to the argument domain. Partial
closures retain the original implementation obligations; attached residual
contracts are checked using the saved argument slots. Bound and captured
callbacks require their own proofs, including callbacks inside composites.
An implementation supplies the remaining row of an open signature, preserving
required parameters and omission defaults. A bodyless open signature retains
an unresolved row. The checker does not use a target annotation as evidence for
itself. Knowing a closure's code and captures, or successfully evaluating one
call, does not discharge its declared contract.

Unproved arithmetic implications, recursive termination proofs, optional
presence branches, arbitrary quantified Boolean inclusion, and general
existential witness synthesis remain incomplete. Effect annotations are retained
and compared as capabilities, but implementation proofs for functions marked
with effects or `extern` remain unsupported. An `extern` declaration does not
supply an implementation or execute foreign code by itself. Pure certification
cannot assume a checked callback is pure. Two effectful clauses with disjoint result
types are not contradictory solely on that basis when they admit a shared effect.

Quantified checking currently consists of finite literal enumeration, extremal
instances for covariant data, distribution through conjunction and fixed record
fields, and rigid-variable proofs for supported arrows. General universal
Boolean predicates, such as `forall A ((func(A) -> A) | (func() -> int))`, remain
exact residual obligations. The current checker has no general decision rule
for these predicates, even when a supplied implementation happens to satisfy
one branch uniformly. Parsing and retaining such a predicate does not imply
that concrete validation can discharge it. See the
[checking-fragment regressions](../cue/testdata/quantified/certification/quantified_fragment.txtar).

## Opaque packages

Sealing supplies an explicit private representation and creates a fresh abstract
carrier. Opening introduces a local abstract type and a view of the declared
interface:

```cue
#Counter: exists State {
    zero: State
    next: func(State) -> State
    read: func(State) -> int
}

counter: seal #Counter with (State = int) {
    zero: 0
    next: func(x: int) -> int: x + 1
    read: func(x: int) -> int: x
}

out: (open counter as (S, C) {
    result: C.read(C.next(C.zero))
}).result
```

The result is `1`. The private representation is accessible only through the
boundary adapters. Those adapters transport records, lists, generic operations,
unions, and higher-order callbacks. Union branches are matched in the source
representation before transport; overlapping branches remain incomplete when
they would expose different public values. Optional and pattern fields follow
the interface. Private implementation fields are not implicitly exported.
This projection applies only to the module's root interface. Ordinary open-record
arguments and results retain their extra data fields, including inside nested
records and lists; adapters transport the abstract occurrences within them.
Omission passes through an adapter, so the private implementation chooses its
own default rather than receiving the interface's default as an argument.

Concrete validation certifies opaque operations by proving the private
implementation and its interface contract, then checking that transport is
total for the supported schema. This includes monomorphic structural operations,
callbacks, and unions whose source branches have disjoint kinds. Unknown generic
transport, recursive schemas, and overlapping transports can still leave an
operation incomplete even when individual concrete calls succeed.
Concrete validation also traverses private representation values, including
functions nested inside records or lists. Hiding a function behind an abstract
carrier does not discharge its conformance obligations.

Copies preserve seal identity and exported aliasing. Executing a new seal creates
a distinct carrier, even when its representation is the same. Abstract values
cannot be interchanged between carriers or escape an opening, including through
later calls of returned closures. A closed existential package can leave the
scope. The initial opaque profile uses unbounded representation binders; a
transparent bound would expose extra representation structure.
Delayed escape checks persist through optional fields, disjunctions, patterns,
and open list tails, including values materialized by later API refinement.
Membership in another instance of an existential interface checks its captured
predicates against the same sealed witness; sharing a template is insufficient.
Every conjunct of a refined interface is retained during sealing and opening.
Unsupported additional existential proofs remain incomplete. Transparent
existential membership preserves the candidate's own shape and closedness:
missing refinable fields remain incomplete, while fields forbidden by a closed
candidate establish a contradiction. It cannot certify a missing field by
checking a separate, augmented record.

The current `open` operation supports record-shaped sealed interfaces with one
unbounded representation binder. Multi-carrier and scalar existential packages
cannot yet be opened. Transparent existential descriptions do not provide an
`open` witness; their current membership rules cover covariant data and reuse of
sealed witnesses, rather than general witness synthesis or elimination.

Opaque values and package operations cannot be serialized as their private
implementations. Observe ordinary data through public operations before exporting
JSON. Source export preserves supported generic functions and concrete captures.
Shared function literals are emitted once, with separate arguments for erased
predicates and runtime captures, so recompilation preserves closure identity.
Finite concrete capture graphs may contain other implemented functions.
Residual quantifiers retain selected arguments, outer predicates, and local
binder scopes. These rules apply to both ordinary and final source export;
unsupported captures, independent partial closures, and opaque operations report
incompleteness rather than being replaced with a weaker description.
Sealed packages also report incomplete source export when their visible
interface contains only ordinary data; export cannot erase their seal identity.

## Implementation map and regression coverage

- `cue/ast`, `cue/parser`, and `cue/format` define lexical syntax and its round
  trips. `internal/core/compile` records binder identity and runtime captures.
- `internal/core/adt/quantified.go`, `subject.go`, `universe.go`, and `witness.go`
  handle type instances, shared subjects, universe checks, and correlated values.
- `internal/core/adt/capability.go`, `closure.go`, `abstract.go`, and `recursion.go`
  implement call obligations, operational identity, symbolic calls, and checked
  finite-list descent.
- `internal/core/adt/package.go` implements existential residuals and opaque
  boundary transport. `internal/core/subsume` separates sufficient inclusion
  checks from implementation certification.
- [The quantified test index](../cue/testdata/quantified/README.md) links every
  layer's txtar fixtures and explains their assertions. Semantic cases run in
  the ordinary evaluator corpus under `cue/testdata/quantified/`.
- [The examples](../cue/testdata/quantified/examples/) are standalone regression
  tests, initially copied from the proposal and maintained independently.
  Intentional errors and specification-only examples assert their errors or
  residual status. Syntax-only cases live in the parser corpus.
- Additional txtar fixtures cover refinement, universes, opacity, identity,
  file ordering, certification, and export. Small Go harnesses retain checks
  requiring Go API operations or AST identity; their input programs are txtar
  sections too. Parser, formatter, AST, exporter, and CLI corpora all have
  `quantified` in their paths or filenames.

Run the semantic corpus and the API checks with:

```sh
go test ./internal/core/adt -run TestEvalV3/quantified
go test ./cue -run TestQuantified
```

See [the test index](../cue/testdata/quantified/README.md) for focused commands
for each layer. Run the complete regression suite with `go test ./...`.

The [finite reference checks](../checks/README.md) independently exercise the
proposal's finite set laws and residual completion semantics. Their deterministic
reports are reproducible with Python 3; they do not certify the Go implementation
or the infinite-domain calculus.
