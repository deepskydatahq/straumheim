---
description: Validate story, epic, or mission completion against acceptance criteria
argument-hint: "<artifact-id|--in-progress>"
---
# Product Judgment

Use the `straumheim-product-workflow` skill with argument `$ARGUMENTS`.

Validate whether the requested story, epic, mission, or in-progress work has achieved its defined outcomes by checking TOML criteria against actual code, configuration, documentation, build output, and tests.

Update product TOML statuses only when evidence supports the change. Report pass/fail criteria with file paths, line numbers, and command results where possible.
