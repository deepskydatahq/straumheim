---
description: Break a Straumheim mission TOML into scoped epics
argument-hint: "<mission-id>"
---
# Product Mission Breakdown

Use the `straumheim-product-workflow` skill and mission `$1`.

Create or update `product/epics/$1-E*.toml` files. Product TOML is the source of truth; do not use Beads/`bd`.

Report created epics, dependency order, coverage of mission outcomes, and any gaps needing review.
