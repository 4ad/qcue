# Universal types and functions in CUE

A universal type constrains one value at every type instance. For a function,
this lets an interface relate its inputs and results while preserving CUE's
ordinary refinements. The implementation can be passed to another function,
stored in a record, and constrained by declarations in several files.

This document explains the language in [version 9 of the paper](paper.pdf)
for experienced CUE users. The examples run in this fork, which enables the
feature by default. The [command guide](cmd/cue.md) explains how to run it.

## Reading a universal function

`func(x: A) -> B: e` declares a function with parameter `x`, input description
`A`, result description `B`, and body `e`. Its result description is a contract
that the body must establish. Omitting `: e` gives a function interface.
An interface describes the callable value to be supplied.

In `id(A): ...`, `A` is a type parameter: the **same** `id` must satisfy the
declaration for every choice of `A`. Explicit type arguments use square
brackets; ordinary calls use parentheses.

<!-- paper: 5 -->
```cue
id(A): func(x: A) -> A: x
integer: id(7)                 // 7
text:    id("seven")           // "seven"
record:  id({port: 443})        // {port: 443}
chosen:  id[int & >=0](7)       // 7
```

The body works because its argument already satisfies `A`. Selecting
`id[int & >=0]` records a particular interface of that same function. Calls
can also infer an instance from their arguments and required results.

Throughout this document, **certified** means that the checker has established
a contract. **Rejected** means that it has evidence against a stated
requirement. **Pending** means that a proof obligation still needs evidence;
**incomplete** means that demanded data is still missing. A call can also
fail with ordinary CUE bottom, `_|_`.

Comments give the paper's checking judgments. The implementation's finite
prover sometimes reports an invalid function obligation as `... remains
unproved`. That diagnostic withholds callable evidence. The explanations
distinguish an invalid claim, for which they give a counterexample, from an
open goal that can acquire the missing evidence through refinement.

For example, a constant body cannot satisfy identity's universal contract:

```cue
wrong(A): func(x: A) -> A: 0    // rejected
```

Choose the singleton type `A = 1`. The call accepts `1` and successfully
returns `0`, which violates its result contract. Testing a universal claim
at a singleton is often the quickest way to understand it.

Examples below have their own scopes. Negative lines are individual checks;
run them separately from the successful cases. A later refinement is an
additional declaration conjoined with the example's original source.

## Descriptions and their completions

An ordinary CUE description specifies possible completed values. Write
`A ≤ B` when every value allowed by `A` is allowed by `B`: `A` refines `B`,
or, equivalently, `B` subsumes `A`. Thus `3 ≤ int ≤ number ≤ _`.

At the level of allowed values, `&` is intersection, `|` is union, `_|_` is
the empty set, and `_` admits every value. A literal is a singleton set.
Defaults additionally carry CUE's usual preference information. Record
descriptions additionally say which fields are allowed and which must be
present; their open or closed character remains significant.

Completing an ordinary field chooses a value satisfying its constraints.
Copies of a schema can choose different values:

<!-- paper: 4 -->
```cue
#Cell: {value: int}
left:  #Cell & {value: 1}
right: #Cell & {value: 2}
```

Declarations about the *same* field instead constrain a common completion.
References preserve the sharing defined by CUE's scope rules. Copying a
record rebinds its internal references to the copy; references to an outer
scope continue to name that outer scope. Consequently, the semantic object
for an open configuration is its set of **joint completions**: assignments
that satisfy all its connected fields together.

One can hide a local field after checking these joint constraints. In logical
terms, that asks whether *there exists* a completion of the hidden field.
Sharing must be established before hiding: choosing one `x` satisfying both
`P(x)` and `Q(x)` is a stronger requirement than choosing independent values
for the two predicates.

### One subject at every instance

A **semantic type** is a predicate on complete values, or equivalently the
set of values satisfying it. The extension adds function values to the
ordinary data values. Types can describe either. A universal type is the
intersection of its instance predicates, all applied to one subject.

`forall (A: U) T` means: for every semantic type `A` refining `U`, the subject
satisfies `T`. An omitted bound means `_`. If `T[A := B]` denotes replacement
of the bound type variable by `B`, the rule is:

```text
v satisfies forall (A: U) T
    exactly when v satisfies T[A := B] for every B ≤ U.
```

The subject `v` stays fixed as `B` varies. Every bound includes the empty
type among its subtypes. These facts explain all three declarations below:

```cue
empty(A): [...A]          // [] is the only possible value
impossible(A): A          // _|_
box(A): {value: A}        // _|_
```

At `A = _|_`, the list can have no elements; `[]` also satisfies every other
instance. `impossible` would have to be a value of the empty type. `box`
would need a present `value` field of that empty type.

A **parametric alias**, written with `=`, constructs a description by
substitution. Each use supplies its type arguments:

```cue
Box(A) = {value: A}
one: Box(int) & {value: 1}
```

Here `Box(int)` expands to `{value: int}`. The quantified declaration
`box(A): ...` above constrains one field at all instances; the alias defines
an expression abbreviation. This distinction determines whether a generic
schema creates useful instances or an impossible universal subject.

### Binder spellings and scope

The following declarations express the same universal interface. Their
callback parameter has type `func(A) -> B`; their list parameter and result
have element types `A` and `B` respectively.

<!-- paper: 2 -->
```cue
map(A, B): func(func(A) -> B, [...A]) -> [...B]
map: forall (A, B) func(func(A) -> B, [...A]) -> [...B]
map: func<A, B>(func(A) -> B, [...A]) -> [...B]
```

A single unbounded variable can be written `forall A`. A bound is written
`A: U`, and may use earlier type parameters. For example,
`forall (A: number, B: A) ...` gives `B ≤ A ≤ number` in its body.
Quantifiers extend as far right as their enclosing expression permits;
parentheses delimit a quantified argument or result.

A lexical `forall` prefix inside a record quantifies the record. These two
declarations have the same meaning:

<!-- paper: 3 -->
```cue
r: {
    forall A
    id: func(A) -> A
}
r: forall A {id: func(A) -> A}
```

Binders have lexical identity. Renaming a bound variable preserves meaning;
using the same spelling in a different scope creates a different binder.
Another file can constrain `r` while the binder's private scope is preserved.

Type parameters provide proof information and are erased from execution.
Function parameters bind the body; parameter and result descriptions are
formed in the enclosing scope of data references and type binders. For
example, a captured outer `Limit` can occur in a result description, while
a term parameter's name is available in the body.

### Predicting what a type parameter preserves

A repeated type parameter denotes a common predicate. Its arguments can be
different inhabitants of that predicate:

<!-- paper: 6 -->
```cue
pair(A: number): func(x: A, y: A) -> [A, A]: [x, y]
p: pair[int & >=1](2, 5)       // [2, 5]
q: pair(1, 2.5)                // [1, 2.5]
bad: pair[int](1, 2.5)         // rejected: 2.5 conflicts with int
swap(A, B): func(p: [A, B]) -> [B, A]: [p[1], p[0]]
s: swap([7, "seven"])          // ["seven", 7]
```

For `q`, the predicate `1 | 2.5` is one admissible choice of `A`. The bound
`number` allows both inhabitants. The independent variables in `swap`
preserve the two component descriptions separately.

A bound gives the operations available on arbitrary inhabitants. A numeric
bound permits comparison, so selecting one of the inputs preserves `A`:

<!-- paper: 7 -->
```cue
min(A: number): func(x: A, y: A) -> A: {
    if x <= y {out: x}
    if x > y  {out: y}
}.out
m: min[int & >=10](12, 20)      // 12
same(A: number): func(x: A) -> A: x + 1 // rejected
```

Every branch of `min` returns an existing `A`. In `same`, addition establishes
a numeric result, but the required result is the particular subtype `A`.
The instance `A = 0` exposes the mismatch: the result is `1`. Likewise, an
arbitrary numeric subtype need not be closed under negation or multiplication.

Universal ranges are part of the contract:

<!-- paper: 8 -->
```cue
choose(A: int):    func(A, A) -> A
choose(A: string): func(A, A) -> A
stronger(A: int | string): func(A, A) -> A

r(A): {id: func(A) -> A}
r: {label: "utilities"}
// The same accumulated constraint:
r(A): {id: func(A) -> A, label: "utilities"}
```

The two `choose` clauses cover integer subtypes and string subtypes.
`stronger` also covers mixed subtypes such as `1 | "one"`, hence calls with
one inhabitant of each kind. Shrinking a bound checks fewer instances and
weakens that universal obligation. Adding another declaration with `&`
adds an obligation to the same subject.

Universals distribute over conjunction when their ranges agree. A constraint
independent of a binder, such as `label`, can move inside it. Independent
universal binders can exchange order. Disjunction needs more care: allowing
either of two predicates at *each* instance can admit a subject that satisfies
neither predicate at *every* instance. Thus distributing `forall` over `|`
would change its meaning.

## Function contracts and open refinement

A function type is a predicate on an implementation. For the mathematical
notation `A → B`, `A` describes the accepted arguments and `B` describes
successful results. With several parameters, `A` describes an **argument
packet**: the supplied positions, labels, and omissions together.

The arrow means: for every complete packet satisfying `A`, every successful
return of this implementation satisfies `B`. This is a **partial-correctness
contract**. Execution can encounter a constraint failure or remain unfinished.
Successful return is what establishes the result's membership in `B`.

### A result contract is a proof obligation

A result annotation asks the checker to prove something about the body.
An explicit body constraint such as `x & >0` is an executable **assertion**:
it forwards `x` when the constraint succeeds and fails otherwise.

<!-- paper: 34 -->
```cue
asText: func(x: int) -> string: x           // rejected
asTwo:  func(x: int) -> 2: 1                // rejected
positive: func(x: int) -> (int & >0): x     // rejected
positiveChecked: func(x: int) -> (int & >0): x & >0
ok: positiveChecked(3)                     // 3
bad: positiveChecked(0)                    // _|_
pick: func(x: int | string) -> int: x       // rejected
pickChecked: func(x: int | string) -> int: x & int
stop(A): func(x: A) -> A: _|_               // certified partial function
```

The first three bodies can successfully return values outside their promised
results: an integer, `1`, or `0` respectively supplies a counterexample.
`pick` can return a string. Their asserted counterparts establish the desired
constraint on every successful execution. `stop` has no successful return,
so it satisfies its partial contract. Its closure is a value; calling it
produces bottom.

### Contravariance checks a capability

Suppose a client requires `A → B`, and the implementation accepts packets
described by `C`. Attaching that interface creates two obligations:

1. **Coverage:** prove `A ≤ C`, so every promised packet is accepted.
2. **Result:** under an arbitrary input in `A`, prove that the body returns
   a value in `B` whenever it succeeds.

If the implementation already has result type `D`, `D ≤ B` proves the second
obligation. Thus, when `A ≤ C` and `D ≤ B`, we have `(C → D) ≤ (A → B)`.
Inputs are **contravariant** and results **covariant**: accepting more inputs
and promising a more precise result gives a stronger capability.

<!-- paper: 29 -->
```cue
wide: func(number) -> string
wide: func(x: int) -> string: "ok"       // rejected: number ≤ int fails
narrow: func(int) -> string
narrow: func(x: number) -> string: "ok"  // accepted: int ≤ number
apply: func(g: func(number) -> string) -> string: g(1.5)
ints: func(x: int) -> string: "ok"
bad: apply(ints)                         // rejected at callback linking
```

The fractional packet `1.5` witnesses the failed coverage checks. The constant
body's string result satisfies its result obligation, independently of that
input failure. Supplying a callback links an actual implementation to the
capability assumed by its client.

### A live description remains connected to its calls

A field used as a type is a **live description**. Its constraints can be
refined by later declarations. An alias of a fixed type names that fixed
predicate:

<!-- paper: 16 -->
```cue
let Fixed = int
fixed: func(x: Fixed) -> int: 2 * x

Live: int
live: func(x: Live) -> int: 2 * x
a: fixed(2)                 // 4
b: live(2)                  // 4

// A later file may add Live: >=0.
```

`Live: int` establishes the upper bound `Live ≤ int`. It supports the
multiplication proof before `Live` is concrete. The function still names
`Live`, and the call allocates a fresh packet constrained by `Live & 2`.
The packet takes its own completion; `Live` remains available for other
calls and later refinements. A `let` alias containing a live reference
preserves that reference's dependency.

For this call, the retained relationship can be written with a fresh argument
cell `p` and result cell `b`:

```text
p satisfies Live
p = 2
b = 2 * p
b satisfies int
```

Computing `b = 4` preserves all four requirements. Adding `Live: >=0` is
compatible. Adding `Live: >2` refutes this invocation because its packet
still has to satisfy `Live`.

The result can be constrained independently in another file:

<!-- paper: 17 -->
```cue
_input: int
_output: int
_f: func(x: _input) -> int: 2 * x
a: _f(2)
a: _output

// Each is a separate possible refinement:
// _input: >=0               // a remains 4
// _input: >2                // invocation fails
// _output: >=4              // a remains 4
// _output: <4               // result fails
```

Each restriction describes that call's packet or result. The reusable
function's result contract remains `int`.

### Why this refinement preserves contravariance

An open configuration describes joint choices of data, live descriptions,
and implementations. Call one such joint choice a **world**. The constraints
already present select a set of worlds, the **store**. Adding a declaration
restricts this store to worlds satisfying both declarations.

At each world, `Live → int` has the ordinary contravariant interpretation.
Across refinements, the source expression still refers to the same `Live`
coordinate. The store narrows; the source graph and its references keep
their identity. This gives two distinct comparisons:

| Comparison | What follows |
| --- | --- |
| Fixed types `Small ≤ int` | `(int → B) ≤ (Small → B)` by contravariance. |
| An open graph followed by `Live: >=0` | Joint completions must satisfy the old graph **and** the new constraint. |

For a precise statement, let `P` be the predicate of joint completions of
the original graph and `R` the new declaration. The refined graph denotes
`P ∩ R`, hence a subset of `P`. Hiding local cells still gives a subset of
the original observations. Replacing the closed type `int → B` by
`(int & >=0) → B`, in contrast, would compare two different predicates on
functions; the second requires behavior on fewer inputs.

**Refine the shared description by conjunction, and keep its dependencies.**
This preserves CUE's information order while each function capability is
checked contravariantly. An established result can stay the same or become
inconsistent. A displayed default retains the ordinary CUE preference rules
and the alternatives from which it was selected.

### The two directions of evidence

An upper bound tells the checker what operations an input supports. A result
or instance membership may require the reverse inclusion. An open goal
stays pending when its supporting facts establish only one direction.

<!-- paper: 18 -->
```cue
A: number
f: func(x: A) -> int: x      // initially pending

// Added by another file:
A: int                      // proves A ≤ int
answer: f(2)                // 2
```

Initially, `A ≤ number` leaves open whether `A ≤ int`. A fraction in
`number` is a *possible* inhabitant of `A`; that alone supplies no persistent
counterexample. Later `A: int` proves the result goal. A fixed `number`
parameter would admit `1.5` outright and refute the identity body's integer
result promise.

The same distinction appears on the result side:

<!-- paper: 19 -->
```cue
R: int
same: func(x: R) -> R: x                  // certified
constant: func(_: int) -> R: 2            // pending
checked: func(x: int) -> R: x & R         // certified partial body

// R: 2   supplies the constant's result proof.
// R: >2  refutes that constant result.
```

`same` forwards the evidence its input already carries. `constant` needs
`2 ≤ R`, whereas the declaration supplies `R ≤ int`. `checked` establishes
membership by executing the assertion. Even equality of currently visible
record fields can leave a live result goal pending: later constraints may
add fields or validation requirements to that description.

Coverage also needs the direction appropriate to the client's capability:

<!-- paper: 24 -->
```cue
Small: int
local: func(x: Small) -> int: x + 1
consumer: func(g: func(int) -> int) -> int: g(2)
a: consumer(local)                      // coverage pending
```

The client promises itself all integer inputs, so linking requires
`int ≤ Small`. The field proves only `Small ≤ int`. The particular packet
`Small & 2` can be compatible while this universal coverage goal remains
unproved. A fixed `let Small = int` supplies both inclusions.

Bounds on type variables can themselves be live:

<!-- paper: 25 -->
```cue
Upper: number
choose(A: Upper): func(x: A, y: A) -> A: {
    if x <= y {out: x}
    if x > y  {out: y}
}.out
a: choose[Upper](2, 5)               // 2, retaining packet constraints
needs: choose[int & >=0](2, 5)       // instance-bound proof pending
// A later file may add Upper: int; a still has result 2.
```

The body uses `A ≤ Upper ≤ number`. Selecting `Upper` proves its own bound
by reflexivity. Selecting `int & >=0` requires `(int & >=0) ≤ Upper`, which
the numeric upper bound does not supply. Adding `Upper: int` strengthens
that upper bound while the reverse inclusion remains a separate obligation.

For a live universal bound, the same store interpretation applies: at each
world, check every `A` included in that world's `Upper`. Refinement restricts
the worlds in which this fixed formula is considered. Previously established
proofs retain their references and assumptions; new declarations can supply
missing premises or impose additional goals.

## Passing and returning functions

A function parameter is checked using its promised contract. The caller then
supplies a function with evidence for that contract. For example, `map`
assumes its callback `g` accepts `A` and successfully returns `B`:

<!-- paper: 9 -->
```cue
map(A, B): func(g: func(A) -> B, xs: [...A]) -> [...B]: [
    for x in xs {g(x)}
]
inc:  func(x: int) -> int: x + 1
text: func(x: int) -> string: "\(x)"
numbers: map(inc, [1, 2, 3])     // [2, 3, 4]
strings: map(text, [1, 2, 3])    // ["1", "2", "3"]
invalid: map[int, string](inc, [1]) // rejected: callback result
```

While checking `map`, `A` and `B` are **rigid**: arbitrary predicates whose
properties follow only from their bounds and the parameter contracts.
Every `x` has type `A`; the callback establishes `B`; the comprehension
constructs `[...B]`. At the final call, `inc` cannot satisfy the required
integer-to-string result contract.

Selecting existing elements preserves their description. Nested traversal
uses the callback's element contract at each step:

<!-- paper: 10 -->
```cue
filter(A): func(p: func(A) -> bool, xs: [...A]) -> [...A]: [
    for x in xs if p(x) {x}
]
partition(A): func(p: func(A) -> bool, xs: [...A]) -> {
    yes: [...A], no: [...A]
}: {
    yes: [for x in xs if p(x) {x}]
    no:  [for x in xs if !p(x) {x}]
}
positive: func(x: int) -> bool: x > 0
kept: filter(positive, [-2, 0, 4])       // [4]
parts: partition(positive, [-2, 0, 4])  // {yes: [4], no: [-2, 0]}
flatMap(A, B): func(g: func(A) -> [...B], xs: [...A]) -> [...B]: [
    for x in xs for y in g(x) {y}
]
duplicate(A): func(x: A) -> [A, A]: [x, x]
twice: flatMap[int, int](duplicate[int], [1, 2]) // [1, 1, 2, 2]
```

`partition` uses the same pure predicate for the two selections. `flatMap`
produces `B` elements because every successful callback result is a list of
`B`. The fixed pair returned by `duplicate[int]` satisfies that list contract.

The same reasoning applies field by field. Reusing `text` from `map`:

<!-- paper: 11 -->
```cue
mapPoint(A, B): func(g: func(A) -> B, p: {x: A, y: A}) -> {
    x: B, y: B
}: {x: g(p.x), y: g(p.y)}
p: mapPoint(text, {x: 2, y: 3}) // {x: "2", y: "3"}
```

The parameter guarantees the selected fields. Its open record description
also admits extra fields in an argument. Each call of `g` establishes the
corresponding result field.

### Composition and captures

A returned function can capture its creator's parameters. `compose` below
uses the earlier `inc` and `text` functions:

<!-- paper: 12 -->
```cue
compose(A, B, C): func(g: func(B) -> C, f: func(A) -> B) ->
    func(A) -> C: func(x: A) -> C: g(f(x))
pipeline: compose[int, int, string](text, inc)
answer: pipeline(4)             // "5"
withPrefix: func(prefix: string) -> func(string) -> string:
    func(s: string) -> string: prefix + s
warn: withPrefix("warning: ")
message: warn("retry")          // "warning: retry"
```

On success, `f` produces a `B`, which is an accepted input to `g`, so the
composition produces `C`. More generally, an intermediate result description
may refine the next function's input description. Captures participate in
the proof under their established bounds; `prefix` supplies a string bound.

Composition also preserves the components' execution requirements. With
`compose` and `text` as above:

<!-- paper: 37 -->
```cue
ignore: func(_: _|_) -> int: 7
x: ignore(_|_)                // _|_
bounded: func(x: int) -> int: x & >0
pipe: compose[int, int, string](text, bounded)
good: pipe(3)                 // "3"
bad: pipe(0)                  // _|_
```

Calls are strict in their demanded arguments: a failed argument propagates
failure even when the body is constant. `pipe` has an integer-to-string
partial contract, and zero fails its first component's positivity assertion.
If both components succeed on the inputs they receive, their certified
composition succeeds with the stated result. Additional caller constraints
on the packet or result must also succeed.

### A polymorphic callback

A **higher-rank** interface places a universal inside another function type.
In the following parameter, `p` itself is universal. The body can therefore
use the same supplied function at two different instances:

<!-- paper: 13 -->
```cue
useBoth: func(p: forall A func(A) -> A) -> [int, string]: [
    p[int](7), p[string]("seven"),
]
ok: useBoth(id)                 // [7, "seven"]
onlyInt: func(x: int) -> int: x
bad: useBoth(onlyInt)            // rejected
```

Here `id` is the universal identity introduced at the start. `onlyInt`
cannot supply the callback's string instance. Moving `forall A` outside
`useBoth` would give the caller a choice of one `A` for each outer call;
placing it on `p` gives the body all of `p`'s instances together.

Universals can also describe returned functions and occur deeper inside
callback interfaces:

<!-- paper: 14 -->
```cue
constant(A): func(x: A) -> (forall B func(B) -> A):
    func(_) -> A: x
seven: constant[int](7)
a: seven[string]("ignored")     // 7
b: seven[bool](true)            // 7

consume: func(k: func(forall A func(A) -> A) -> int) -> int: k(id)
use: consume(func(p: forall A func(A) -> A) -> int: p[int](8)) // 8
```

`seven` captures one integer and accepts an argument of every type `B`.
Its argument remains subject to strict validation. In `consume`, the callback
must itself accept a universal identity; supplying `id` discharges that
inner requirement.

### Universal types as type arguments

An **impredicative** instance uses a type containing a universal as the
argument to another universal. `Identity` below is a fixed alias for the
polymorphic identity interface; `id` can accept a value of this type too:

<!-- paper: 15 -->
```cue
let Identity = forall A func(A) -> A
again: id[Identity](id)
a: again[int](3)                // 3
b: again[string]("three")       // "three"
ids: [...Identity]
ids: [id, again]
c: ids[0][bool](true)           // true
bad: id[Identity](onlyInt)       // rejected
```

The returned value is the same polymorphic closure. List storage and
selection preserve its universal obligations. `onlyInt`, defined above,
lacks the required universal capability.

Instances are scoped proof choices on a shared implementation. This small
example makes the independence of calls and callback linking explicit:

<!-- paper: 27 -->
```cue
id(A): func(x: A) -> A: x
first: id[int](3)
second: id[string]("three")
use: func(p: forall A func(A) -> A) -> int: p[int](4)
result: use(id)                         // 4
```

Each selection retains its own instance expression. Checking `use` assumes
its parameter's universal contract; supplying `id` proves that assumption.
Type inference gathers the corresponding lower and upper requirements with
their variance. An explicit instance can make this proof choice precise.
An unresolved inference problem remains pending.

## Combining interfaces on one implementation

Function types use ordinary CUE conjunction. Each conjunct constrains the
same function value. In particular, the arrow laws are:

```text
(A → B) & (A → C) = A → (B & C)
(A → B) & (C → B) = (A | C) → B
```

On an input covered by several clauses, all their result constraints apply.
The input-result relationship of each clause is retained:

<!-- paper: 30 -->
```cue
convert: (func(int) -> string) & (func(string) -> int)
refine:  (func(number) -> number) & (func(int) -> int)
choice:  (func(int) -> int) | (func(int) -> bool)
classify: func(x: int) -> ("negative" | "nonnegative"): {
    if x < 0  {out: "negative"}
    if x >= 0 {out: "nonnegative"}
}.out
classify: (func(int & <0) -> "negative") &
          (func(int & >=0) -> "nonnegative")
c: classify(-3)                          // "negative"
```

`convert` requires both capabilities of one implementation. `refine` applies
both result promises on integers and only the numeric promise on other
numbers. `choice` permits either whole function predicate. `classify` uses
executable conditions whose branch facts establish the more precise clauses.
Bodyless fields such as `convert` still need implementations to be called.

### Shared records and declarations across files

A universal can constrain an entire record. Its fields belong to the same
record subject, and extra declarations can add ordinary metadata:

<!-- paper: 28 -->
```cue
utilities(A): {
    id:   func(x: A) -> A: x
    wrap: func(x: A) -> {value: A}: {value: x}
}
utilities: {name: "core"}
wrapped: utilities.wrap[string]("payload") // {value: "payload"}

// interface.cue
wrap(A): func(A) -> {value: A}
// metadata.cue
wrap(B): func(B) -> {tag: "wrapped"}
// implementation.cue
wrap(T): func(x: T) -> {value: T, tag: "wrapped"}: {
    value: x, tag: "wrapped"
}
```

Selection preserves `utilities`' shared record and quantifier. The three
`wrap` declarations align their bound variables by renaming. The implementation
must supply both result fields for each type instance.

An implementation body can likewise discharge independently written result
constraints. These five separate records have the same accumulated result
requirement and each contains a body with an executable positivity assertion:

<!-- paper: 35 -->
```cue
one: {f: func(x: int) -> (int & >0): x & >0}
two: {
    f: func(int) -> (int & >0)
    f: func(x: int) -> _: x & >0
}
three: {
    f: func(x: int) -> int: x & >0
    f: func(int) -> >0
}
four: {
    f: func(int) -> int
    f: func(x: int) -> >0: x & >0
}
five: {
    f: func(int) -> int
    f: func(int) -> >0
    f: func(x: int) -> _: x & >0
}
```

All five establish `int → (int & >0)`, succeed on `3`, and fail on `0`.
The body evidence remains available when another declaration adds an
interface. Permuting, regrouping, or duplicating conjuncts preserves the
accumulated obligations.

### Checking explicitly impossible interfaces

Partial correctness permits a function that always fails to satisfy
`int → _|_`: it has no successful return violating that result predicate.
The language also checks the usefulness of explicitly stated interfaces
through the paper's **relevance check**.

For each explicit interface, this check groups inputs into **regions** with
the same applicable arrow clauses. It intersects all results promised in
each region. If that result is provably empty, it asks whether the input
region is empty too. An inhabited input region with an impossible promised
result rejects the interface.

<!-- paper: 31 -->
```cue
f: (func(int) -> int) & (func(int) -> bool)   // rejected
g: func(int) -> _|_                         // rejected
h: (func(number) -> int) & (func(int) -> bool) // rejected on int
three: (func(int) -> (0 | 1)) &
       (func(int) -> (1 | 2)) &
       (func(int) -> (0 | 2))               // rejected jointly
callback: func((func(int) -> int) &
              (func(int) -> bool)) -> string // rejected in parameter
alternative: (func(int) -> _|_) |
             (func(int) -> int)             // first arm rejected locally
```

For `f` and the integer region of `h`, successful results would need to
satisfy `int & bool`. For `three`, every pair of results intersects, but
the intersection of all three is empty. All three clauses therefore matter.
Checking also descends into callback parameters and written record or list
components.

The check records each declaration as an **observation root**, retaining its
scope and dependencies through aliases, copying, and simplification. Union
alternatives and optional fields have **guards** recording when that root
applies. A rejected union arm stays a rejection under its own guard. It
rejects the whole choice when all alternatives are rejected or that arm is
required. Relevance rejection is a diagnostic about a source requirement;
the partial arrow can still contain failing implementations semantically.

A live domain can leave the input-emptiness question open:

<!-- paper: 23 -->
```cue
limit: int
f: func(x: int & >limit & <10) -> _|_: _|_
// Pending: the result is empty; input reachability depends on limit.

// Separate refinements:
// limit: 10     // empty input domain; obligation discharged
// limit: 0      // input 1 witnesses rejection
```

The two outcomes need positive evidence: a proof that the region is empty,
or a witness that it is inhabited. With an unknown `limit`, neither fact
holds throughout the store. Once a rejection is proved throughout its
retained scope, further refinement preserves it.

### Universal observations and concrete instances

An independent concrete clause can supply a type instance at which to compare
a universal clause. The paper calls this an **anchor**:

<!-- paper: 32 -->
```cue
id(A): func(A) -> A
id: func(int) -> int             // valid redundant instance
quiet(A): func(A) -> A
quiet: func(int) -> bool         // rejected
```

The integer domain selects `A = int`. The universal then promises `int`,
and `quiet`'s independent clause promises `bool` on those same inputs.
Their result intersection is empty. The independence of that concrete
clause survives moving it beneath the binder.

For this bodyless `quiet` example, the current implementation reports
`interface relevance blocked`, leaving the anchored refutation pending.
The contradictory integer results above explain the paper's required rejection.

A generic result must be checked under its rigid variables. An intersection
of two arbitrary types is sometimes inhabited, so it supports a useful
partial operation:

<!-- paper: 33 -->
```cue
meet(A, B): func(x: A, y: B) -> (A & B): x & y
good: meet({a: 1}, {b: true})     // {a: 1, b: true}
bad: meet[int, bool](1, true)    // _|_
specialized: func(int, bool) -> (int & bool) // rejected interface
f(A): func(xs: [A, A]) -> A: xs[0] & xs[1]
x: f([{a: 1}, {b: 1}])           // {a: 1, b: 1}
y: f([1, 2])                     // _|_
```

`A & B` has no proof of emptiness uniform over all choices: `A = B = 0`
is an inhabited instance. The generic operation retains its original
observation root; the incompatible integer-Boolean call fails through ordinary
unification. Writing `specialized` creates its own explicit root, whose
inhabited domain and empty result directly reject it.

Open record operands can share a completion that contains both sets of fields,
as in `good` and `x`. Distinct scalar singletons have no common completion.
These results follow CUE's existing record and scalar unification rules.

## Calling conventions are part of the type

The accepted packet description includes positions, labels, and omissions.
Parameter syntax specifies how each original slot can be supplied:

| Parameter | Binding |
| --- | --- |
| `a: T` | By position or label `a`. |
| `a!: T` | Required, by label `a`. |
| `a?: T` | Optional, by label `a`; omission means absence. |
| `_~x: T` | By position; `x` is an alias used in the body. |
| `T` or `_: T` | Anonymous positional slot. |

Positional-only slots precede slots accepting either form, which precede
label-only slots. Calls put positional arguments before labeled arguments.
All bindings resolve to the original slots before their values are checked.

<!-- paper: 38 -->
```cue
sum: func(a: int, b: int) -> int: a + b
p: sum(2, 3)                  // 5
q: sum(a: 2, b: 3)            // 5
r: sum(2, b: 3)               // 5
key: func(a!: int, note?: string) -> int: a
k: key(a: 4)                  // 4; note absent
n: key(a: 4, note: "audit")    // 4
badKey: key(4)                // rejected: a is label-only
scale: func(_~value: int, _: int) -> int: value * 2
s: scale(5, 99)               // 10
badLabel: scale(value: 5, 99)  // malformed: label and argument order
bad1: sum(1, a: 1)            // duplicate binding
bad2: sum(1, 2, 3)            // surplus argument
bad3: sum(a: 1, b: 2, c: 3)   // unknown label
bad4: sum(1)                  // missing required argument
pending: sum(_, 2)            // supplied but incomplete
```

Equal values still bind a slot twice in `bad1`. A body alias provides a name
for the implementation's use, while its slot remains positional. An optional
parameter requires presence evidence before the body can use its value.

Hidden labels carry their package identity. Attributes remain metadata on
the parameter:

<!-- paper: 39 -->
```cue
package internal
measure: func(_value: int @go(Int), unit!: string) -> int: _value
inside: measure(_value: 7, unit: "ms")
// Other packages can supply the positional slot, not this hidden label.
```

Within `internal`, `_value` names that package's hidden label. An external
caller can use the first position and the public `unit` label.

### Omission and defaults

An omission default, written `= d`, supplies a value when the caller leaves
out its slot. It is an executable part of the function's calling convention,
checked against the parameter description in its declaration environment:

<!-- paper: 40 -->
```cue
add: func(a: int, b: int = 10) -> int: a + b
x: int | *5
omitted:  add(1)             // 11
supplied: add(1, x)          // 6
unknown:  add(1, _)          // incomplete
render: func(text: string, suffix!: string = "!") -> string:
    text + suffix
hello: render("hello")                 // "hello!"
question: render("hello", suffix: "?") // "hello?"
badDefault: func(x: int = "x") -> int: x // rejected
```

A supplied argument suppresses the omission default. Its own CUE default
still participates in evaluation, as with `x`; `_` remains an incomplete
supplied argument. A required-label slot with an omission default is
omittable because that default supplies its value.

Coverage includes these calling conventions. To expose a new label or default,
write an adapter whose body makes the corresponding call:

<!-- paper: 41 -->
```cue
raw: func(_~n: int) -> int: n + 1
named: func(x: int) -> int: raw(x)
configured: func(x: int = 2) -> int: raw(x)
a: named(x: 2)              // 3
b: configured()            // 3
```

Each wrapper has its own implementation and packet contract. Attaching a
function interface asks the existing implementation to cover that interface's
actual labels and omissions.

### Open parameter rows and partial calls

An interface ending in `...` has an **open parameter row**: its remaining
slots are completed when an implementation is linked. A call ending in
`...` performs **partial application**: it checks and saves the supplied
slots and returns a function for the remainder.

<!-- paper: 42 -->
```cue
T: func(a: int, ...) -> number
f: T & (func(a: int, b: int) -> int: a + b)
x: f(1, 2)                   // 3
missing: f(1)                // rejected: b is required
add3: func(a: int, b: int, c: int) -> int: a + b + c
part: add3(1, ...)
part: func(int, int) -> int
last: part(2, ...)
answer: last(3)              // 6
same: add3(a: 1, ...) & part
again: same(2, 3)            // 6
```

Linking supplies `T`'s remaining slot and its requiredness. Partial calls
retain unbound defaults and requirements. Each saved binding identifies its
original slot, so binding `a` by label or first position yields the same
partial function. Successive disjoint bindings normalize to the same binding
map as supplying them together. The body executes when the remaining packet
is supplied; even saving every slot explicitly leaves a zero-argument function.

## Records, captured data, and function identity

A field description can support a proof before the field has a concrete
value. Presence remains part of that evidence:

<!-- paper: 36 -->
```cue
get: func(r: {a: int}) -> int: r.a
ok: get({a: 1, b: true})                    // 1
bad: func(r: {a: int}) -> int: r.b          // rejected: no field evidence
needsB: func(r: {a: int, b: bool}) -> int: r.a
missing: needsB({a: 1})                    // incomplete: b is unresolved
absent: {a?: int & bool}                   // a must be absent
conflict: {a: int & bool}                  // _|_
fixed: [int, string] & [int]               // _|_: length conflict
```

`get` has a guaranteed integer field to select. Its input is open, so the
extra `b` is allowed. In `missing`, the packet combines the open argument
with the parameter's `b: bool` constraint and still demands a value for `b`.
An impossible optional field forces absence; an impossible present field
refutes its record. Fixed list lengths must agree.

A captured field similarly provides its known bound to the body proof while
its runtime value remains a separate demand:

<!-- paper: 21 -->
```cue
factor: int
scale: func(x: int) -> int: factor * x  // body certified
factor: 3
answer: scale(4)                       // 12

_f: {
    z!: int
    f: func(x: int, y: int) -> int: x + y + z
}
bar: (_f & {z: 10}).f(100, 1000)        // 1110
```

The first declaration of `factor` supplies the integer bound needed for
multiplication; its second declaration supplies the value needed to compute
`12`. A later contradiction in `factor` remains a contradiction of that call.

In `_f`, the required declaration `z!: int` supplies the body-proof bound.
The meet creates a completed instance whose method sees `z: 10`. The prototype
still requires its own `z`; another instance can supply a different value.
Record unification collects sibling constraints before checking methods and
rebinds their internal references to the shared record. Outer references
retain their lexical targets.

### What identifies a function value

A function value is a **closure**: executable code together with its free
captures and saved partial arguments. Its identity consists of its code
origin, the values of its free captures, and the normalized map of saved
slots. The origin identifies the source implementation through copying.

<!-- paper: 43 -->
```cue
x: {f: func(y: int) -> int: y}
a: x.f & x.f
b: x.f & {x}.f
c: x.f & (x & {g: 1}).f
r: [a(2), b(2), c(2)]        // [2, 2, 2]
d: [for v in [1, 2] {func(y: int) -> int: y + v}]
e: d[0] & d[1]              // _|_: unequal captures
```

The three references to `x.f` retain its origin and free captures; the
unrelated `g` field changes neither. The comprehension creates closures with
the same origin but different captured `v` values, hence distinct inhabitants.
When captures are still open, their equality remains a constraint to resolve.

To combine the results of different implementations, a new body can call
them both and unify their results:

<!-- paper: 44 -->
```cue
left:  func(x: int) -> {a: int}: {a: x}
right: func(x: int) -> {b: int}: {b: x}
both: func(x: int) -> {a: int, b: int}: left(x) & right(x)
r: both(3)                  // {a: 3, b: 3}
first:  func(x: int) -> int: 1
second: func(x: int) -> int: 2
conflict: first & second    // _|_: distinct origins
choice: first | second      // two possible implementations
preferred: *first | second
p: preferred(3)             // 1 after default selection
```

`both` establishes its combined result in its body. Unifying `first` and
`second` intersects two distinct singleton closure values. Their disjunction
keeps alternatives, and a default chooses an implementation under CUE's
ordinary preference rules. Source intended for later refinement retains the
alternatives and the dependencies of that choice.

### Validation follows the selected value

A record's hidden validation constraints remain relevant when a public field
is selected. A call record can likewise expose its packet and result for
constraints from other files:

<!-- paper: 45 -->
```cue
positive: func(x: int) -> bool: x > 0
#Positive: {value: int, _valid: true & positive(value)}
n: (#Positive & {value: 7}).value  // 7
bad: #Positive & {value: -1}       // _|_
raw: func(x: number) -> string: "\(x)"
call: {arg: number, result: raw(arg)}
call: {arg: int, result: string}   // another file's restriction
ok: (call & {arg: 7}).result       // "7"
wrong: (call & {arg: 1.5}).result // _|_
```

The hidden `_valid` field checks the selected `value`. The two declarations
of `call` constrain one packet and result. Their integer restriction excludes
the fractional call while `raw` keeps its numeric input capability. This is
the same distinction between call refinement and function coverage developed
earlier, expressed as an ordinary reusable CUE record.

## Builtins, native boundaries, and finite traversal

A builtin or standard-library function participates through a contract owned
by its implementation. That contract supplies evidence for its supported
arguments and successful results. The [type reference](types.md) lists these
interfaces, including element relationships, overloads, labels, and validators.

### Structural operations preserve evidence

The builtin `and` unifies a list's elements. A guaranteed element gives the
result a constraint even when the remaining tail is empty. The builtin `or`
forms their disjunction, and integer indexing selects an element on success:

<!-- paper: 49 -->
```cue
merge(A): func(xs: [A, ...A]) -> A: and(xs)
x: merge([{a: 1}, {b: 1}]) // {a: 1, b: 1}
choose(A): func(xs: [...A]) -> A: or(xs)
y: choose(["ok"])          // "ok"
first(A): func(xs: [...A], i: int) -> A: xs[i]
z: first([1, 2], 1)        // 2
```

`merge`'s mandatory head is essential. Changing its input to `[...A]` would
admit `[]`, and `and([])` is `_`: there is then no element constraint to
establish an arbitrary result `A`. The checker leaves this unsupported body
goal unproved. The empty disjunction and an out-of-range index have no
successful result, consistent with the partial contracts of `choose` and
`first`.

`len` can retain known lengths and bounds. `close` preserves its input
constraints while adding closedness where the record description establishes
the field inventory. Operators similarly need evidence that each possible
operand supports the operation; an arbitrary numeric subtype supports
arithmetic but can require a broader result type.

### Linking native implementations

A bodyless declaration supplies a named hypothesis under which a client can
be checked. Execution requires an implementation that discharges that
hypothesis. In the paper's boundary example, `externF` is supplied by a checked
native adapter:

<!-- paper: 46 -->
```cue
legacyBody: func(int) -> int
checked: func(x: int) -> (int & >0): legacyBody(x) & >0
externF: func(int) -> int
small: externF(42)
large: externF(1000000000000000000000000000000)
fractional: externF(1.5)        // rejected: logical argument mismatch
```

The source alone leaves the implementations to be linked. `checked` retains
its positivity assertion once `legacyBody` is supplied. A linked native
adapter may accept `42` and fail to represent the large integer in a machine
integer slot. Both are CUE integers; the adapter's conversion is an executable
check. `1.5` already violates the logical input contract. A portable partial
contract describes every successful native result while retaining those
boundary checks.

### A finite fold

A left fold repeatedly applies a step to an accumulator and the next list
element. Its empty case returns the seed. Its nonempty case updates the seed
and continues with the tail. If the accumulator has type `B` and an element
has type `A`, the step must have type `func(B, A) -> B`.

The paper declares this interface and links it to a finite traversal. Here is
the interface with the structurally decreasing implementation used by its
[executable example](../cue/testdata/quantified/paper/047-finite-iteration-and-repeated-invocation.txtar):

<!-- paper: 47 -->
```cue
fold(A, B): func(step: func(B, A) -> B, seed: B, xs: [...A]) -> B
fold(A, B): func(step: func(B, A) -> B, seed: B, xs: [...A]) -> B: {
    if len(xs) == 0 {out: seed}
    if len(xs) > 0 {out: fold(step, step(seed, xs[0]), xs[1:])}
}.out
plus: func(x: int, y: int) -> int: x + y
sum: fold[int, int](plus, 0, [1, 2, 3]) // 6
empty: fold[int, int](plus, 0, [])      // 0
twice: func(n: int) -> int: n + n
nested: twice(twice(twice(2)))          // 16
```

The result argument is induction on finite list length: the seed supplies
`B`, each successful step supplies another `B`, and the remaining list is
shorter. The step's failure propagates. Nested `twice` calls also create
finite ordinary activations. The implementation checks finite structural
descent for recursive execution and accounts for evaluation and proof work.

## Checking and evaluation make progress together

A **certificate** is a finite proof of a fact or function contract, recording
the source facts and hypotheses it uses. An upper bound may be sufficient
to certify a body while its captured data remains incomplete. Evaluation
can then supply values or stronger bounds that enable further checking:

<!-- paper: 20 -->
```cue
seed: func(x: int) -> int: x + 1
k: seed(2)
shift: func(x: int) -> int: x + k
limit: shift(1)
bounded: func(x: int) -> int: x & <=limit
a: bounded(3)                           // 3; limit is 4
```

`seed`'s certificate gives `k` an integer result bound. That bound supports
`shift`'s body; its execution computes `limit`; the assertion can then check
`3 ≤ 4`. Evidence and values can become available at different times while
remaining part of one constraint graph.

### Proof dependencies must be grounded

Each proof records its **support**: the references, binders, calling
conventions, and hypotheses on which it depends. A callback's assumed
contract can be used inside the client's proof and is discharged at linking.
An implementation's own result annotation is a goal to establish:

<!-- paper: 22 -->
```cue
k: f(0)
f: func(x: int) -> int: k
a: f(1)
```

Proving the result of `f` here requires evidence about `k`, which depends
on the unproved result of `f`. This cycle remains pending. Adding an
independent `k: int` supplies the body-proof premise; the cyclic computation
still needs evidence that produces a concrete value for `k`. A checked
inductive argument, as with finite fold, supplies its own justified rule.

Proofs about fixed references persist as the store is refined: a fact true
in every old world is true in every remaining world. A proof conditional on
an equality or other hypothesis keeps that condition in its support. New
requirements create new goals, and new information wakes their dependents.

### Computed results retain their constraints

A **residual** is the remaining source graph and its unfinished obligations
after partial evaluation. An exact residual describes the same joint
completions as the original graph on all coordinates available to later
unification. If `P` and `P'` describe the original and residual graphs, and
`R` is any future constraint on those coordinates, the invariant is:

```text
P' = P, hence P' & R = P & R.
```

The equality includes live descriptions, lexical references, captured data,
defaults, and validation dependencies. In particular, the expression
`4 & _f(2)` below retains the call that justifies the visible result:

<!-- paper: 26 -->
```cue
_x: int
_f: func(x: _x) -> int: 2 * x
a: 4 & _f(2)

ignore: func(_: int) -> int: 7
pending: ignore(_)              // incomplete demanded argument
bad: ignore(_|_)                // _|_
```

Adding `_x: >2` still refutes `a`. The ignored argument remains demanded in
both calls of `ignore`: the incomplete call cannot complete its validation,
and the failed call propagates bottom. A **derived atom** such as `4` is a
computed summary with those dependencies; a **completed value** has also
discharged its current demanded validations and proof obligations.

### Direct checks and suspended work

The mandatory checking rules cover kind and literal conflicts, constant
numeric intervals, record presence and closedness, fixed list lengths,
ground primitive operations, and malformed packets. They also combine
arrow regions, check anchored universal instances, and preserve guards on
alternatives. For example, refuting every union alternative proves that
union empty; an empty admissible instance proves a universal type empty.

<!-- paper: 48 -->
```cue
emptyInt: int & >0 & <1         // _|_: no integer in this interval
emptyData: {a: int} & {a: bool} // _|_
cycle: {a: int & (5 * b), b: int & (a - 1)}
grounded: cycle & {a: 0}        // _|_ by ground propagation
```

The first two contradictions are direct. The cycle's equations imply
`4*a = 5`, incompatible with integer `a`, but a checker needs an arithmetic
proof to exploit that fact. Local propagation can retain those equations
as a residual. Grounding `a` at zero makes the contradiction directly
available: `b = -1`, and then `a = -5` conflicts with zero.

A relevance pass that completes its direct checks without finding a uniform
result contradiction is called **quiet**. This records the current completed
search; later evidence triggers another pass. A pending proof, a quiet pass,
and a proved rejection therefore carry different information. The finite
checker can leave true statements unproved, and a work limit can suspend
checking with obligations still present.

The evaluator retains five things: the exact graph, proved facts and bounds,
certificates, outstanding goals with observation triggers, and a worklist of
enabled steps. The paper's propagation loop expresses their interaction:

<!-- paper: 1 -->
```text
propagate(state, budget):
    while work_remains(state) and budget_allows_step(budget):
        node = take_work(state)
        for item in one_accounted_step(node, state):
            verify_derivation_or_exact_rewrite(item)
            if accumulate_with_scope_and_support(state, item):
                enqueue_dependents(state, item)
    return retained_residual(state), scoped_diagnostics(state)
```

Constraints accumulate by conjunction, and checked evidence accumulates
alongside them. Evaluation, checking, and propagation each preserve the
residual's meaning. A finite run returns completed observations, justified
failures, or explicit unfinished obligations. On a fixed finite set of facts
and goals, fair propagation reaches the same result independently of work
order. Further evaluation can create additional work.

### Inspecting and refining a result

`cue eval` shows values and remaining constraints. `cue vet -c=false` checks
function obligations as well as data consistency, allowing ordinary
incomplete data. `cue vet -c` additionally demands completed data. To observe
a selected concrete result as JSON, use `cue export example.cue -e answer`.
JSON represents the resulting data; CUE source represents functions and
constraints available for subsequent refinement.

In the Go API, `Value.Err()` reports evaluation errors. `Value.Validate()`
also checks explicit interfaces and function bodies, including uncalled
bodies. `cue.Concrete(true)` additionally demands concrete data, linked
implementations, and runtime captures. Pending proof goals are reported by
validation even when concrete data is not requested.

`Unify`, `FillPath`, and faithful source export retain the graph needed for
further constraints. Exported closures preserve their origins, lexical
captures, saved slots, and obligations; importing the source reinstalls the
checks. Thus later refinement can finish a pending goal, invalidate a
particular call, or impose another interface on an existing implementation,
while every established proof remains valid under its retained premises.

The [paper example index](../cue/testdata/quantified/paper/README.md) maps every
listing to its executable fixture. The [implementation guide](implementation.md)
details the checking rules and API behavior, and the [test index](../cue/testdata/quantified/README.md)
lists regression coverage for refinement, linking, and source round trips.

<!--
 Copyright 2026 The CUE Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->
