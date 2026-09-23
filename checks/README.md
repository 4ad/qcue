# Finite reference checks

Run from the repository root with Python 3; no third-party packages are needed:

```sh
python3 checks/check_model.py
python3 checks/check_residual.py
```

The scripts rewrite their adjacent JSON reports deterministically. Counts are
computed from executed checks, rather than copied from the proposal. A failed
check exits unsuccessfully and does not replace its report. Assertions are
explicit checks and remain enabled with `python3 -O`.

`check_model.py` enumerates four values, their sixteen predicates, and all 625
partial unary functions, including rejection. It checks arrow and quantifier
laws, scope and application counterexamples, and bijective representation
transport. `check_residual.py` checks a finite relational kernel with bounded
propagation and retained residual clauses. It also checks grounded cycles,
guarded implications, correlated existential alternatives, and universal
function counterexamples.

These are executable reference models for the paper's finite examples. They do
not execute the Go implementation or establish correctness for infinite domains.
Implementation regressions are indexed in
[`cue/testdata/quantified/README.md`](../cue/testdata/quantified/README.md).
