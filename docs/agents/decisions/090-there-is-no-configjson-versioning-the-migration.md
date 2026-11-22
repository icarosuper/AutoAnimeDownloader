# 90. There is no `config.json` versioning: the migration checks SHAPE, in one place

**Location:** `files/filemanager.go` — `LoadConfigs` (the two legacy-field migrations, ~22
lines). Negative fact: **there is no** `config_version` field, migration chain, or lock that
blocks the app with an old config. And the migration lives in **one** place —
`api/endpoint_config.go` deliberately does not repeat it on `PUT`.

**What it looks like:** a lack of robustness. The project is distributed (AUR + releases) and an
old-version `config.json` loading wrong is a silent failure. The natural way out looks like
numbering the format, writing a v1→v2→v3 chain and locking the app with a big warning when the
migration does not go through.

**Why it does not exist:** because the format already solves two of the three cases on its own.
`LoadConfigs` unmarshals the JSON **over `getDefaultConfig()`** (`filemanager.go`), so:

| Change on `Config` | Needs a migration? |
|---|---|
| **New** field | No — the default holds, an old config loads right |
| **Removed** field | No — JSON ignores an unknown key |
| **Renamed** field | **Yes** — the only case |

A version chain would exist to serve only the third row, which happened **twice** in the life of
the project (`anilist_username` → `anilist_usernames` and `excluded_list` → `excluded_lists`, both
in April 2026).

And the version number would be **less** reliable than the check that is there. `if
config.ExcludedList != "" && len(config.ExcludedLists) == 0` is **derived from the data**; a
`config_version` is **metadata asserting something about the data**, and `config.json` is a file
the user opens in an editor. They edit a field and the version sits still; they restore a backup
and the version comes from another moment than the shape. Then the chain runs the wrong
migration — with confidence, which is what the shape check does not do.

The lock has the opposite problem: **no `config.json` that exists today has a version field**.
Either "no version" is treated as v1, and then the lock never protects the real case, or it fires
on everyone at debut. And for a rename, migrating is never *impossible* — if you can instruct the
user on screen, you can do it.

**The precedent that closes the subject:** `save_path` (#31, amendment of 24 Aug 2026) was exactly
this question with a **worse** consequence — without migrating, rain finds the derived path empty
and re-downloads the entire library. The decision was to remove the migration and document a
manual step. And what locked there was not a missing version field: the operation preserves the
inode, carries `torrents.RootMarkerName` with it and there is no way to know whether the user
already moved the folder by hand. No chain makes that safe.

Which is the general ruler: a config migration is either **pure shape** (rename — 3 lines, no
version needed) or **a change with an effect outside the config** (moving data on disk — a version
does not make it safe). The framework helps in neither.

**When to revisit:** if config migration becomes frequent (3+ in a year) or if one appears whose
old→new mapping is not mechanical. Today it is 2 in ~2 years, both mechanical.

**Don't "fix" by:**
- Adding `config_version` + a chain: see above; shape already answers, and the number can lie.
- Locking the app when the config looks old: every config that exists today fires that lock.
- Repeating the migration on `PUT /config` "for the old client": that is what `endpoint_config.go`
  used to do, and the comment was wrong — no CLI version sent the singular key (the old switch's
  `case "anilist_username"` set `AnilistUsernames`, the plural). A body that still sends the
  singular key is persisted and cured on the next `LoadConfigs`; the copy in the handler only
  anticipated the cure by one cycle, at the cost of two migrations to diverge.
- Taking the ~22 lines out of `LoadConfigs` "because it's been months": an empty account list is
  **not** an error state (#49 — a standalones-only install never configures an account), so a
  pre-April config would load with no accounts, without complaining, and the daemon would simply
  download nothing from a list.
