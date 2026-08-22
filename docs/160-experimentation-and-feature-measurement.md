# 160 — Experimentation & Feature Measurement

## Objective
Measure product changes scientifically.

## Experiment
```text
hypothesis
control
variant
population
primary_metric
guardrail_metrics
start
end
```

## Example
```text
New checkout
→ conversion
→ cancellation guardrail
→ payment failure guardrail
```

## Randomization
Choose stable assignment keys such as user/account identifiers where appropriate.

Do not repeatedly switch users between variants.

## Analysis
Define success metrics before launching.

Avoid interpreting noisy short-term changes as guaranteed product improvements.

## Definition of Done
Major experiments have measurable hypotheses, guardrails and reproducible assignment.
