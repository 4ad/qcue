# Quantified CUE: implementation and use

This implementation supports fragments of profiles **S_H** (predicative
higher-rank quantification) and **A** (opaque existential packages) in
[the proposal](quantified-cue.tex). The `quantified` experiment is **enabled by
default** for CUE language version `v0.18.0` and later, including standalone files
and Go API calls with no pinned language version.

Build and install from this checkout:

```sh
go install ./cmd/cue
cue version
```

The executable is `cue`, so it can directly replace an upstream `cue` binary.
Its version output identifies the build, supported CUE language version, S_H and A
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
Expansion has a work budget shared by nested and deferred finite bodies.
Exhaustion retains the complete scoped predicate and leaves validation
incomplete; it never publishes a truncated union or intersection. Constant
literal bodies avoid the product after checking the ranges for emptiness.

For a new module, use
`cue mod init --language-version v0.18.0 example.com/quantified`. For an existing
module, set its language version with `cue mod edit --language-version v0.18.0`.

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
Conjoining distinct checked views of the same closure retains their admitted
call domains: `id[int] & id[string]` supports both instances in either order.
The body runs once, and every applicable view's result obligation is checked.
This does not weaken the explicit argument check on `id[int]` alone.
Calls with empty containers or unused binders can infer the empty predicate,
provided the selected instance admits every supplied argument.
Inference keeps lower and upper constraints separate, reverses variance at
callback inputs, and propagates requirements through dependent subtype bounds.
Data fields are collected before instantiating callbacks, including callbacks
nested in records and lists. A chosen instance must admit every supplied slot.
A failed candidate remains incomplete unless an independent necessary bound or
universe condition excludes every instance; guarded clauses cannot disappear
because one guess failed. This remains a sufficient, incomplete inference rule.

Type-sorted names denote predicates. An ordinary refinable field used in a
signature denotes its eventual singleton. For example, `x: int` and
`f: func(x) -> string` describe a function that accepts the particular eventual
value of `x`. The current approximation `int` cannot discharge that obligation.
For records and lists, the singleton includes the entire eventual data shape;
it is not an open structural predicate that admits additional fields.
Ordinary and parametric aliases decode their constituent references in the
context of each use. Predicate and runtime uses have separate caches, while lexical
binder identities and function code origins remain shared. Unresolved singleton
constraints inside unused record arguments also keep a call incomplete.

Erased type parameters may appear in signatures, type selections, and seal
witnesses. They cannot supply runtime return values, arguments, defaults, or
ordinary data indexes. A bound such as `A: int` does not make `A` an integer
argument. The compiler rejects these runtime uses, including indirect uses
through local aliases. The check follows free dependencies and nested argument
substitutions. Index guards belong to each alias use: checking an erased argument
does not invalidate a separate fixed integer index through the same template.
Finite literal value binders retain their selected values as runtime captures
and are distinct from erased type binders. Type application and sealing share
the same sorted witness check: value witnesses must belong to their range; type
witnesses must satisfy their universe and subtype bounds. A mixed seal creates
opaque carriers only for its type witnesses.

## Functions, refinement, and checking

Conjoining function contracts retains every guarded capability clause. An
implementation keeps its original labels, defaults, omitted-argument behavior,
and extra-argument policy. Applicable clauses constrain its actual call result.
An unresolved applicability guard remains an obligation. Applicability is
checked against the original supplied packet, before parameter constraints or
defaults enrich the activation. Abstract application retains missing execution
as an obligation and propagates only independently admitted result clauses.
For callable domains, including callbacks inside records or lists, applicability
requires independent conformance evidence from the original argument.
Completing a call also checks the actual packet's callable membership
obligations before returning or caching its result. Ignoring a callback, or
calling it on one successful input, cannot discharge its universal contract.
These checks include hidden runtime fields in packets, captured records and
package implementations. Host schema validation of undemanded definitions and
absent optional fields remains separate from runtime conformance.

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
Those contracts remain separate validation obligations. Data `==` and `!=` use
the same recursive runtime comparison through container and opaque boundaries;
nesting cannot make erased selections or redundant contracts observable.
The same runtime identity governs singleton membership and abstract-value
equality, recursively through records, lists, and builtins. An opaque meet keeps
both private constraint graphs and their validation obligations. Constraint
equality used for graph deduplication still distinguishes retained contracts.

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

`cue vet file.cue` requires concrete validation, including function type
conformance. An unresolved contract makes validation incomplete; use `cue vet -c`
to see the detailed errors. There is no separate function-verification flag.
`cue vet -c=false` explicitly permits incomplete constraints for further
refinement and does not establish that every retained implementation satisfies
its type. Definitions and absent optional fields remain schemas until demanded
as ordinary values, following CUE's usual concreteness rules.

The conformance checker handles annotated structural bodies, higher-rank
arguments, records, lists, projections, finite comprehensions, supported pure
primitives, and defaults proved to belong to the argument domain. Partial
closures retain the original implementation obligations; attached residual
contracts are checked using the saved argument slots. Bound and captured
callbacks require their own proofs, including callbacks inside composites.
Every saved argument must also belong to its parameter's required domain; a
callback's own valid annotation is insufficient. This check is separate from
the universal proof of the original implementation and residual contracts.
An implementation supplies the remaining row of an open signature, preserving
required parameters and omission defaults. A bodyless open signature retains
an unresolved row. The checker does not use a target annotation as evidence for
itself. Knowing a closure's code and captures, or successfully evaluating one
call, does not discharge its declared contract.
Calls within certified bodies use the remaining protocol of a partial closure.
They consider the whole available arrow intersection, including finite unions
of admitted input packets, while keeping original obligations separate from a
selected view's available domain. Definition fields and absent optional fields
do not introduce executable callback hypotheses.
Primitive proof rules check the actual labels, arity, and omission policy before
using a known total result rule.
Numeric translation by a constant preserves supported bounds.
Boolean negation and numeric signs are also certified. Numeric negation
preserves unions and exclusions and reverses strict and non-strict bounds.
Constructed records carry their exact field set during proof. Package clients
can be checked by opening the admitted interface under a fresh abstract carrier,
using its operation contracts as hypotheses, and checking that the carrier
cannot escape.

Certification shares one bounded proof context through nested evaluator calls.
Completed proofs are reusable only when their inherited hypotheses are still
available; a proof in progress is never evidence for itself. The work budget
bounds repeated proof expansion as well as depth. Exhaustion reports
incompleteness and leaves the original obligations available for another check.

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
records and lists; hidden and definition labels keep their package identity.
Adapters transport the abstract occurrences within these values. A scoped
transport plan supplies both execution and totality certification. Identity
branches retain their entire constraint graph. Changing composite branches
retain the original graph behind an inverse-image predicate, preserving
closedness, correlations, optional restrictions, and patterns under later
refinement. Definitions and absent optional fields undergo predicate transport.
Unchanged pattern regions also retain their visible lexical dependencies and
label bindings, so scope and universe checks cannot lose them.
Omission passes through an adapter, so the private implementation chooses its
own default rather than receiving the interface's default as an argument.
Independent nested packages pass through by identity, preserving their seals
and private validation obligations. A nested public interface that depends on
the outer carrier requires a stronger transport rule and remains incomplete.
The dependency check includes definitions, optional fields, patterns, and free
references of residual quantifiers. Another seal binds its own witness only;
free outer abstract dependencies cannot escape behind it. Repackaging an outer
representation as a private witness behind an independent interface is allowed.

Transporting a callback inward and back outward through the same interface
preserves its observable closure identity. The adapters remain in place for
execution and validation; identity comparison recognizes inverse transports
without removing their contracts or identifying independent operation handles.

Concrete validation certifies opaque operations by proving the private
implementation and its interface contract, then checking that transport is
total for the supported schema. This includes monomorphic structural operations,
callbacks, and unions whose source branches have disjoint kinds. Unknown generic
transport, recursive schemas, and overlapping transports can still leave an
operation incomplete even when individual concrete calls succeed.
An operation with an arrow intersection retains every clause and environment.
Calls select an admitted transport view and enforce all applicable result
promises. Independent private proofs are insufficient when public domains
overlap: the transports must agree as well. Disjoint domains, or a supported
proof of transport agreement, make the intersection checkable. Unknown guards
and transport agreement remain incomplete. Partial overloaded operations
currently require a common packet coordinate system; differing rows remain
incomplete. Whole-interface metadata also participates in adapter caching and
recognition of inverse callback transports.
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
Escape checks also follow singleton witnesses, every retained function or
builtin clause, and generic bounds. A returned closure can retain private
runtime captures behind an ordinary public signature; its later results still
undergo the scope check.
Membership in another instance of an existential interface checks its captured
predicates against the same sealed witness; sharing a template is insufficient.
Every conjunct of a refined interface is retained during sealing and opening.
Unsupported additional existential proofs remain incomplete. Transparent
existential membership preserves the candidate's own shape and closedness:
missing refinable fields remain incomplete, while fields forbidden by a closed
candidate establish a contradiction. It cannot certify a missing field by
checking a separate, augmented record.

The current `open` operation supports record-shaped interfaces with one
unbounded representation binder. Multi-carrier, bounded, and scalar existential
packages cannot yet be opened. For an admitted transparent covariant record,
opening constructs the same greatest admissible type witness used by its
membership rule. Repeated opening of a shared subject preserves that witness.
Certification of a client also checks that this witness construction has total
public transport; it cannot assume every existential member was explicitly
sealed. Noncovariant transparent descriptions still require a supplied witness;
general existential witness synthesis remains unsupported.

Opaque values and package operations cannot be serialized as their private
implementations. Observe ordinary data through public operations before exporting
JSON. Source export preserves supported generic functions and concrete captures.
Shared function literals are emitted once, with separate arguments for erased
predicates and runtime captures, so recompilation preserves closure identity.
Finite concrete capture graphs may contain other implemented functions.
Runtime captures retain hidden fields observable by the code, including fields
inside records and lists. A hidden label from another package that cannot be
rebound faithfully makes independent export incomplete.
Residual quantifiers retain selected arguments, outer predicates, and local
binder scopes. These rules apply to both ordinary and final source export;
unsupported captures, independent partial closures, retained transport
predicates without an interface codec, and opaque operations report
incompleteness rather than being replaced with a weaker description.
Sealed packages also report incomplete source export when their visible
interface contains only ordinary data; export cannot erase their seal identity.
Singleton membership and closure capture equality likewise retain seal identity,
including packages whose interface is empty or contains only ordinary data.

Captured predicates always export in schema mode, including with `cue.Final()`:
definition closedness, optional and required fields, patterns, and defaults
remain constraints on future calls. Both implementations and bodyless contracts
carry their lexical dependencies. Parametric and ordinary aliases retain their
declarations and bounds, with renamed bindings to avoid destination capture.
Unknown runtime witnesses make independent export incomplete.
Opened type names remain lexical export dependencies even though they are
erased from runtime captures. Neither source nor final export may emit a free
abstract name or silently acquire a binding from the destination scope.

Evaluated source export also retains quantified scalar, record and list introductions,
their selected telescopes, refinements, and shared copies. New type selections
after recompilation preserve the original selection interface. A graph that
exports both a composite introduction and a separate method from that same
introduction currently reports incomplete export: emitting independent code
origins would change closure identity. Incompatible lexical origins that would
require the same unsupported code projection are also rejected explicitly.
Projected methods from selected records and fixed lists retain their remaining
method telescope through export and reimport, including separately supplied
implementations. Original universal clauses remain proof obligations and cannot
restart a consumed binder. Scalar normalization, call-result detachment, and graph deduplication likewise
retain the introduction's universe and selection information. Passing a
quantified scalar or list through an identity call cannot erase its remaining
telescope or restart a consumed binder.

## Implementation map and regression coverage

[The semantic preservation design](quantified-cue-semantics-redesign.md)
describes the judgment boundaries, their representations and independent finite
models. [The boundary implementation audit](quantified-cue-boundaries.md) records
the subsequent consolidation, regressions, and final verification. The original audit reports are historical descriptions of their stated
revisions.

- `cue/ast`, `cue/parser`, and `cue/format` define lexical syntax and its round
  trips. `internal/core/compile` records binder identity and runtime captures.
- `internal/core/adt/quantified.go`, `subject.go`, `universe.go`, and `witness.go`
  handle type instances, shared subjects, universe checks, and correlated values.
- `internal/core/adt/capability.go`, `closure.go`, `abstract.go`, and `recursion.go`
  implement call obligations, operational identity, symbolic calls, and checked
  finite-list descent.
- `internal/core/adt/call_admission.go`, `call_contract.go`, and
  `runtime_identity.go` share original-packet admission, available call clauses,
  guarded results, and recursive runtime observations across their consumers.
- `internal/core/adt/package.go`, `existential_witness.go`, and
  `binder_witness.go` implement existential residuals and sorted construction
  and elimination of witnesses. `transport_plan.go` and
  `transport_constraint.go` share scoped transport and retain source predicates.
  `internal/core/subsume` separates sufficient inclusion checks from
  implementation certification.
- [The quantified test index](../cue/testdata/quantified/README.md) links every
  layer's txtar fixtures and explains their assertions. Semantic cases run in
  the ordinary evaluator corpus under `cue/testdata/quantified/`.
- [The examples](../cue/testdata/quantified/examples/) are standalone regression
  tests, initially copied from the proposal and maintained independently.
  Intentional errors and specification-only examples assert their errors or
  residual status. Syntax-only cases live in the parser corpus.
- Additional txtar fixtures cover refinement, universes, opacity, identity,
  file ordering, certification, and export. Small Go harnesses retain checks
  requiring Go API operations or AST identity. Most inputs are txtar sections;
  small API tables and generated resource stress cases also live in Go tests.
  Parser, formatter, AST, exporter, and CLI corpora all have
  `quantified` in their paths or filenames.

Run the semantic corpus and the API checks with:

```sh
go test ./internal/core/adt -run TestEvalV3/quantified
go test ./cue -run TestQuantified
```

See [the test index](../cue/testdata/quantified/README.md) for focused commands
for each layer. Run the complete regression suite with `go test ./...`.
