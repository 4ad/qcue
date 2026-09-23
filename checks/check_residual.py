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

"""Finite completion-set checks for a bounded relational propagation kernel.

Every residual clause is retained, including guards and existential unions.
Bounds are propagation approximations, never substitutes for those clauses.
"""

import itertools

from finite import Checks, FUNCTIONS, SUBSETS, VALUES, arrow, members, subset

ASSIGNMENTS = tuple(itertools.product(VALUES, repeat=2))


def relation(predicate):
    return sum(1 << i for i, (x, y) in enumerate(ASSIGNMENTS) if predicate(x, y))


def box(x_domain, y_domain):
    return relation(lambda x, y: x in members(x_domain) and y in members(y_domain))


def project(r, coordinate):
    return sum(1 << x for x in {a[coordinate] for i, a in enumerate(ASSIGNMENTS) if r & (1 << i)})


def completions(domains, clauses):
    result = box(*domains)
    for clause in clauses:
        result &= clause
    return result


def propagate(domains, clauses, budget):
    x, y = domains
    for _ in range(budget):
        for clause in clauses:
            supported = box(x, y) & clause
            x, y = project(supported, 0), project(supported, 1)
    return x, y


def main():
    checks = Checks()
    vocabulary = (
        relation(lambda x, y: x == y),
        relation(lambda x, y: x < y),
        relation(lambda x, y: x + y == 3),
        relation(lambda x, y: x != 0 or y == 0),
        relation(lambda x, y: (x == 0 and y == 1) or (x == 1 and y == 0)),
        relation(lambda x, y: x == y + 1),
    )
    for mask in range(1 << len(vocabulary)):
        clauses = tuple(r for i, r in enumerate(vocabulary) if mask & (1 << i))
        for domains in itertools.product(SUBSETS, repeat=2):
            before = completions(domains, clauses)
            for budget in range(3):
                after = propagate(domains, clauses, budget)
                checks.check("bounded propagation", completions(after, clauses) == before)
                checks.check("refinement", subset(after[0], domains[0]) and subset(after[1], domains[1]))
                if not after[0] or not after[1]:
                    checks.check("sound refutation", before == 0)

    for clause in vocabulary:
        for domains in itertools.product(SUBSETS, repeat=2):
            if completions(domains, (clause,)):
                continue
            for a, b in itertools.product(SUBSETS, repeat=2):
                if subset(a, domains[0]) and subset(b, domains[1]):
                    checks.check("persistent refutation", completions((a, b), (clause,)) == 0)

    guard = vocabulary[3]
    checks.check("guard retention", completions((3, 3), (guard,)) != box(3, 3))
    checks.check("guard retention", completions((1, 2), (guard,)) == 0)
    checks.check("guard retention", completions((2, 2), (guard,)) != 0)
    alternative = vocabulary[4]
    checks.check("existential correlation", alternative != box(project(alternative, 0), project(alternative, 1)))
    checks.check("existential correlation", completions((1, 1), (alternative,)) == 0)
    checks.check("existential correlation", completions((1, 2), (alternative,)) != 0)

    # Cyclic arithmetic descriptions retain their equations under grounding.
    cycles = (
        (relation(lambda x, y: x == y + 1), relation(lambda x, y: y == x - 1)),
        (relation(lambda x, y: x == y + 1), relation(lambda x, y: y == x + 1)),
    )
    for clauses in cycles:
        for x, y in ASSIGNMENTS:
            ground = (1 << x, 1 << y)
            expected = all(clause & box(*ground) for clause in clauses)
            checks.check("grounded cycles", bool(completions(ground, clauses)) == expected)
            checks.check("grounded cycles", completions(propagate(ground, clauses, 1), clauses) == completions(ground, clauses))

    universal = (1 << len(FUNCTIONS)) - 1
    for a in SUBSETS:
        universal &= arrow(a, a)
    for i, function in enumerate(FUNCTIONS):
        bad_packets = [x for x in VALUES if function[x] != x]
        checks.check("quantified counterexamples", bool(universal & (1 << i)) == (not bad_packets))
        # Testing only one successful singleton cannot certify the universal.
        if function[0] == 0 and bad_packets:
            checks.check("sample incompleteness", not universal & (1 << i))
    checks.write(__file__, "residual-results.json", "two four-valued witnesses, six retained relational clauses")


if __name__ == "__main__":
    main()
