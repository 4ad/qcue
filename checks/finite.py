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

"""Shared finite universes and reporting for the proposal's reference checks."""

import itertools
import json
from collections import Counter
from pathlib import Path

VALUES = tuple(range(4))
SUBSETS = tuple(range(1 << len(VALUES)))
TOP = SUBSETS[-1]
REJECT = -1
FUNCTIONS = tuple(itertools.product((REJECT, *VALUES), repeat=len(VALUES)))


def members(mask):
    return tuple(x for x in VALUES if mask & (1 << x))


def subset(a, b):
    return a & ~b == 0


def image(f, domain):
    return sum(1 << y for y in {f[x] for x in members(domain)} if y != REJECT)


def arrow(domain, codomain):
    return sum(
        1 << i
        for i, f in enumerate(FUNCTIONS)
        if all(f[x] != REJECT and codomain & (1 << f[x]) for x in members(domain))
    )


class Checks:
    def __init__(self):
        self.families = Counter()

    def check(self, family, condition):
        if not condition:
            raise AssertionError(f"{family}: check {self.families[family] + 1} failed")
        self.families[family] += 1

    def write(self, script, filename, model):
        result = {
            "model": model,
            "status": "passed",
            "assertions": sum(self.families.values()),
            "families": dict(sorted(self.families.items())),
        }
        Path(script).with_name(filename).write_text(json.dumps(result, indent=2) + "\n")
        print(f"{filename}: {result['assertions']:,} assertions passed")
