---
name: mobile-performance
description: Measure-first performance work for the mobile apps — startup, lists, maps, realtime, and battery drain during background location. Use when addressing a performance complaint or before optimizing anything on mobile.
---

# Purpose

Fix performance problems that exist, using measurements rather than intuition.

# When to Use

A performance or battery complaint. Before any optimization. When a native module is proposed for performance reasons.

# Rules

- **Measure first.** Profile → identify the bottleneck → change one thing → measure again. An optimization with no before-and-after number is a guess, and guesses on mobile are frequently backwards.
- **Do not prematurely optimize.** Clear code that meets the budget is finished.
- Native code is not automatically faster, and it costs double maintenance (`native-module-boundary`). Justify it with a measurement.
- **Battery is a first-class metric for the driver app.** A driver whose phone dies mid-shift stops earning — that is an outage, not a papercut. Background location is the dominant cost (`docs/182`, `mobile-location`).
- Optimize against a stated budget (`docs/319`), not against "feels slow".

# Focus Areas (`docs/182`, `docs/336`)

- app startup time
- long lists (job history, catalog, earnings)
- map rendering and marker updates
- realtime event handling and re-render volume
- background execution and location batching
- battery drain across a realistic shift

Map marker churn and unbatched realtime updates are the two most common causes of both jank and battery drain in apps of this shape — check them before restructuring anything larger.

# Workflow

1. Reproduce on a real device. Simulator performance is not evidence.
2. Measure and record the baseline.
3. Identify the actual bottleneck — do not assume it.
4. Change one thing.
5. Measure again and record the delta.
6. Keep the change only if the number moved.

# Verification

Level 2 for a contained optimization; Level 5 if it touches background location or realtime handling, since both can regress correctness while improving a number.

Every performance claim needs before-and-after figures on a real device. "Feels smoother" is not a result.

# Blocking Conditions

- No performance budget is defined for the surface → establish one with the user before optimizing; otherwise there is no definition of done.
- The optimization would move business logic into the client or into native code → stop.
- Real-device testing is unavailable → report that the measurement could not be taken rather than substituting simulator numbers.

# Relevant Documentation

`docs/182-mobile-performance-and-battery.md` · `docs/319-performance-budget.md` · `docs/336-performance-testing-mobile.md` · `docs/101-map-performance-and-caching.md` · `docs/181-performance-load-and-stress-testing.md`
