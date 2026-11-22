# 12. Build logic lives in `scripts/build.sh`, not in Makefile targets

**What it looks like:** `build-linuxamd64` / `build-linuxarm64` / `build-windows` just delegate to `bash scripts/build.sh <platform> <version>` with no logic in the Makefile itself.

**Why it's right:** Make's autocomplete exposes every named target, including internal ones (`docker-build`, `docker-buildx-build`, `docker-build-classic`, `checksums`, `check-docker`) and file/directory targets (`build/linux-amd64`, `build/linux-amd64/autoanimedownloader-daemon`, etc.). Moving the implementation to a script eliminates all internal targets from the completion list without requiring per-machine shell configuration.

**Don't "fix" by:** inlining Docker logic back into Makefile targets or re-introducing named intermediate targets. That re-pollutes autocomplete on every machine.
