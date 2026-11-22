# Subtitle language filter

Added on 2026-10-18 (moved from TODO.md on 2026-11-22). Designed, not implemented. Ships as a patch
whenever it is tackled. Delete this file when it is done or dropped.

## What

Filter torrent ranking (and optionally hard-filter) by subtitle language in priorities (e.g.
PT-BR). Spec designed on 2026-08-24:
[subtitle-language-filter](../superpowers/specs/2026-08-24-subtitle-language-filter-design.md).

The language is read from the release **name** (`[PT-BR]`, `MultiSub`, …), not from the torrent
file list and not from scraping the Nyaa description.
