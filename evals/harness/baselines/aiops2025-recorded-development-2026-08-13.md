# AIOps2025 Recorded Development Baseline

## Scope

- Source: AIOps Challenge 2025 public dataset, 400 labelled fault cases.
- Licence: CC BY-NC 4.0.
- Corpus version: `aiops2025-2025-06`.
- Split: SHA-256 grouped by `fault_type | instance_type | target | observation_date`.
- Development: 296 cases; frozen holdout: 104 cases. No family crosses the split.
- Suites: GoS and Evidence only. RAG, Plan and Tool were intentionally excluded because this source does not provide independent knowledge-retrieval labels or executable tool traces for those suites.

## Run

- Profile / role: `recorded` / `development`.
- Executed suite cases: 296 GoS + 296 Evidence = 592.
- Result: succeeded.
- Passed gates: Evidence claim support, citation traceability, cross-suite diagnostic trace completeness and diagnostic evidence completeness.
- Corpus source fingerprints: `input.json` `db10c6d517ca5a8ab8fb73f203b345f729bdc396581e9d9d55afd92d6591f0e9`; `groundtruth.jsonl` `e79a77f610b4a0d08f23c3a6d6aa96576035e4b7f5ec481abae90801d5e70eb0`.
- Split fingerprint: `d58958bd82405c3a47ce9500eba72d9c5c8309f082947254a40feadd7d36967b`.

## Boundary

This run verifies corpus preparation, provenance, split isolation, case schema and recorded adapter contracts. The TaskResult is a recorded fixture built from public labels, so this is not a measurement of live GoS accuracy, RAG quality, tool reliability or production impact.

The external-corpus capacity gap remains explicit: development has 2,704 fewer cases than the 3,000 target; holdout has 596 fewer than the 700 target. Future expansion requires additional authorised, de-identified incident sources through the same prepare/validate process.
