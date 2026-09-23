# Copyright 2026 CUE Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Exhaustive finite checks of the quantified proposal's set-theoretic laws.

Run with Python 3, without third-party dependencies. This is a reference model,
not a test of the Go evaluator or a proof for infinite semantic universes.
"""

import itertools

from finite import Checks, FUNCTIONS, REJECT, SUBSETS, TOP, VALUES, arrow, image, members, subset


def main():
    checks = Checks()
    arrows = {(a, b): arrow(a, b) for a in SUBSETS for b in SUBSETS}
    all_functions = (1 << len(FUNCTIONS)) - 1
    for a, b, c in itertools.product(SUBSETS, repeat=3):
        checks.check("arrows", arrows[a | b, c] == arrows[a, c] & arrows[b, c])
        checks.check("arrows", arrows[a, b & c] == arrows[a, b] & arrows[a, c])
        if subset(a, b):
            checks.check("variance", subset(arrows[b, c], arrows[a, c]))
            checks.check("variance", subset(arrows[c, a], arrows[c, b]))
    identity = all_functions
    for a in SUBSETS:
        identity &= arrows[a, a]
    checks.check("universal identity", identity == 1 << FUNCTIONS.index(VALUES))
    checks.check("universal identity", arrows[0, 0] == all_functions)
    checks.check("universal identity", identity & arrows[1, 2] == 0)
    checks.check("universal identity", all_functions & ~arrows[TOP, TOP] != 0)

    # Two possible assignments in every data fiber, represented as two bits.
    relations = range(1 << (2 * len(VALUES)))

    def weaken(a):
        return sum(3 << (2 * x) for x in members(a))

    def exists(r):
        return sum(1 << x for x in VALUES if r & (3 << (2 * x)))

    def forall(r):
        return sum(1 << x for x in VALUES if r & (3 << (2 * x)) == 3 << (2 * x))

    for r in relations:
        for a in SUBSETS:
            checks.check("adjunctions", subset(exists(r), a) == subset(r, weaken(a)))
            checks.check("adjunctions", subset(weaken(a), r) == subset(a, forall(r)))
            checks.check("Frobenius", exists(r & weaken(a)) == exists(r) & a)
            checks.check("inhabited floating", forall(r & weaken(a)) == forall(r) & a)
        for s in relations:
            if subset(r, s):
                checks.check("quantifier monotonicity", subset(exists(r), exists(s)))
                checks.check("quantifier monotonicity", subset(forall(r), forall(s)))
    checks.check("projection counterexample", exists(1) & exists(2) != exists(1 & 2))
    relation = lambda x, y: x == y
    checks.check("mixed scope", all(any(relation(x, y) for y in VALUES) for x in VALUES))
    checks.check("mixed scope", not any(all(relation(x, y) for x in VALUES) for y in VALUES))
    checks.check("empty range", all(False for _ in ()) and not (False and all(True for _ in ())))
    for bound in SUBSETS:
        meet = all_functions
        for a in SUBSETS:
            if subset(a, bound):
                meet &= arrows[a, a]
        expected = sum(1 << i for i, f in enumerate(FUNCTIONS) if all(f[x] == x for x in members(bound)))
        checks.check("bounded universals", meet == expected)

    def apply(function_set, args):
        out = 0
        for i, f in enumerate(FUNCTIONS):
            if function_set & (1 << i):
                out |= image(f, args)
        return out

    for a, b, args in itertools.product(SUBSETS, repeat=3):
        result = apply(arrows[a, b], args)
        if subset(args, a):
            checks.check("application inclusion", subset(result, b))
        checks.check("application refinement", subset(apply(arrows[a, b] & identity, args), result))
    f = 1 << FUNCTIONS.index((0, 0, 0, 0))
    g = 1 << FUNCTIONS.index((0, 1, 2, 3))
    checks.check("application counterexample", apply(f & g, 1) != apply(f, 1) & apply(g, 1))

    # Bijections rename representations. Conjugate every partial operation,
    # retaining rejection as an outcome outside the data carrier.
    for permutation in itertools.permutations(VALUES):
        rename = lambda a: sum(1 << permutation[x] for x in members(a))
        for a, b in itertools.product(SUBSETS, repeat=2):
            checks.check("abstract predicates", rename(a & b) == rename(a) & rename(b))
            checks.check("abstract predicates", rename(a | b) == rename(a) | rename(b))
            checks.check("abstract predicates", subset(a, b) == subset(rename(a), rename(b)))
        for x, y in itertools.product(VALUES, repeat=2):
            checks.check("abstract equality", (x == y) == (permutation[x] == permutation[y]))
        for operation in FUNCTIONS:
            transported = [REJECT] * len(VALUES)
            for x in VALUES:
                y = operation[x]
                transported[permutation[x]] = REJECT if y == REJECT else permutation[y]
            for x in VALUES:
                y = operation[x]
                expected = REJECT if y == REJECT else permutation[y]
                checks.check("operation simulation", transported[permutation[x]] == expected)
    checks.write(__file__, "results.json", "four values, sixteen predicates, 625 partial functions")


if __name__ == "__main__":
    main()
