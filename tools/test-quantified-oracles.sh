#!/bin/sh
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

# Periodic extended suite. Native Go fuzzing records and minimizes failures in
# testdata/fuzz; a recorded failure is replayed by ordinary go test thereafter.
set -eu
cd "$(dirname "$0")/.."
export CUE_QUANTIFIED_ORACLE="${1:-extended}"
case "$CUE_QUANTIFIED_ORACLE" in
    fast|extended) ;;
    *) printf '%s\n' 'usage: test-quantified-oracles.sh [fast|extended]' >&2; exit 2 ;;
esac

go test ./cue ./internal/core/adt \
    -run '^TestQuantified(Oracle|Semantic|PacketDomainModel$)' \
    -count=1 -v -timeout=15m

if [ "$CUE_QUANTIFIED_ORACLE" = fast ]; then
    exit 0
fi

for target in FiniteOracle Preservation FeatureCombinations PacketDomain; do
    package=./cue
    if [ "$target" = PacketDomain ]; then
        package=./internal/core/adt
    fi
    go test "$package" -run '^$' -fuzz "^FuzzQuantified${target}$" \
        -fuzztime="${CUE_QUANTIFIED_FUZZ_TIME:-30s}" \
        -parallel="${CUE_QUANTIFIED_FUZZ_WORKERS:-2}" -timeout=15m
done
