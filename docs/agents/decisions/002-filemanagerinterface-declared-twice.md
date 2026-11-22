# 2. `FileManagerInterface` declared twice

**What it looks like:** The same interface exists in both `internal/daemon/helpers.go` and `internal/api/server.go` instead of a shared `interfaces` package.

**Why it's right:** `api` already imports `daemon` (for `*State`, `LoopControl`, etc.). If `daemon` imported `api` (or a shared package that `api` also imports transitively), the import graph would cycle. Duplicating the interface keeps each package self-contained and Go's type system satisfies both interfaces structurally — no explicit coupling needed.

**Don't "fix" by:** extracting to a shared `interfaces` or `types` package without first tracing the full import graph. A seemingly neutral extraction will break the build.
