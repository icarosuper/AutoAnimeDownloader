# Dashboard account filter in the CLI

Added on 2026-11-15 (moved from TODO.md on 2026-11-22). Not started. Ships as a patch whenever it
is tackled. Delete this file when it is done or dropped.

## What

`animes --account anilist:alice`: the same membership rule as the dashboard (the account has the
anime in a download status), applied to `GET /animes`'s `accounts`.

## Why

The web filter shipped in `v2.10.0`; the CLI half of the plan did not (CLI parity, see CLAUDE.md).
