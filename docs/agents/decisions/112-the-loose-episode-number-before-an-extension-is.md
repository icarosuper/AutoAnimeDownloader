# 112. The loose episode number before an extension is every video extension, compiled from the map

**Location:** `nyaa/nyaa_regex.go` — `videoExtEpisodeRegexp`, `reEpisodePatterns`; `nyaa/nyaa.go` — `videoExtensions`, `IsVideoFile`.

**What it looks like:** the pattern used to be `\s(\d{1,4})\.mkv`. It is now built at init from `videoExtensions`, the same keys `IsVideoFile` uses.

**Why it's right:** the dash terminator is space / `v2` / end / `[`. A file `K-On! - 01.mp4` (MystVortex, measured 2026-10-29) missed the dash because `.` is none of those, then missed the mkv-only suffix, and `PackFileRange` / the Jellyfin rename saw no number. Hardcoding the suffix list next to the map is how `.mp4` was forgotten. Compiling from the map is the coupling.

The dash terminator still has no `\.`: `- 17.5 (OVA)` would then parse as 17, and a half-episode would enter min–max.

**Don't "fix" by:** adding `\.` to the dash terminator "so any extension works"; listing the extensions a second time in the regex.
