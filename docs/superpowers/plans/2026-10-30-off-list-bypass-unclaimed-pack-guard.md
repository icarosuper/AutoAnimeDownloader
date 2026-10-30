# Off-list bypass of the unclaimed pack guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete a multi-season pack on the next pass once no claimed anime of that torrent is in anyone's watching list, while still keeping it when a claimed season is in watching or another season is adopted in the same pass.

**Architecture:** `hasUnclaimedContent` stays. A new helper `unclaimedHolds(group, watching)` is the second gate: unclaimed content holds the torrent only if `watching == nil` (API / unknown) or some `ep.AnimeID` of the group is in the map. The automatic pass always passes a non-nil map built from this pass's download list. `deleteEpisodesByStatus` also receives `newEpisodes` so a same-pass S2 adoption is visible before S1's status deletion runs.

**Tech Stack:** Go daemon (`internal/daemon`). No frontend, no new API, no config field.

**Spec:** `docs/superpowers/specs/2026-10-30-off-list-bypass-unclaimed-pack-guard-design.md`

**Branch:** `flatten-restart-packs` (already checked out). Do not create a worktree or a new branch.

## Global Constraints

- Everything in English: identifiers, comments, logs, docs, commit subjects. Portuguese only in `messages/pt-BR.json`. Found Portuguese in a file being touched → translate it in the same change.
- Conventional Commits, English, subject line only, no body, no trailing period, all lowercase after `type:`.
- `go test ./...` after any code change. If GOCACHE is permission-denied, set `GOCACHE` to a writable dir (e.g. `/tmp/aad-gocache`) rather than skipping tests.
- No new config field, no new endpoint, no CLI command.
- Do not store sequel AniList ids. Do not walk SEQUEL at deletion. Do not drop `hasUnclaimedContent`. Do not change `force` (manual torrent delete still bypasses the guard).
- `watching == nil` keeps today's #74 behaviour (API per-episode delete). Empty non-nil map is off-list and drops the pack.
- TDD: write the failing test, watch it fail for the right reason, then the minimal production code.
- New decision number is **#117**.
- NEVER commit or push from a request the user did not make; this plan's per-task commits are requested.
- Comments explain **why**, not **what**. Non-obvious decisions point at `decisions.md #117`.

---

## File structure

| File | Responsibility |
|---|---|
| `src/internal/daemon/episodes.go` | `unclaimedHolds`; `watching` on `removeEpisodesAndLinks` / `canRemoveTorrent` / `forgettableKeys`; `handleEpisodesData.watching`; `deleteEpisodesByStatus` takes `newEpisodes` + `watching` |
| `src/internal/daemon/verification.go` | Build watching map from `animes`; pass it to both deletion calls; pass `newEpisodes` into status deletion |
| `src/internal/daemon/episodes_test.go` | Predicate pins; flipped Kaguya/continuous tests; keep-while-watching; status-deletion same-pass pin |
| `src/internal/daemon/selection_test.go` | `canRemoveTorrent` gains `watching` (`nil`) |
| `src/internal/daemon/orchestration_test.go` | `removeEpisodesAndLinks` gains `watching` (`nil`) |
| `src/internal/daemon/limits_test.go` | Compiles; zero-value `watching` is fine |
| `docs/agents/decisions.md` | #117 + #116 sentence + #74 location line |
| `docs/agents/architecture.md` | Deletion bullet |
| `docs/TODO.md` | Close Method A |
| `docs/superpowers/specs/README.md` | Drop the executed row |
| `docs/superpowers/specs/2026-10-30-off-list-bypass-unclaimed-pack-guard-design.md` | Delete after implementation (executed specs do not stay) |

Signatures this plan introduces (later tasks consume these exact names):

```go
func unclaimedHolds(group []files.EpisodeStruct, watching map[int]bool) bool

func canRemoveTorrent(group []files.EpisodeStruct, deleteSet map[files.EpisodeKey]bool, watching map[int]bool) bool

func forgettableKeys(
	keysToDelete []files.EpisodeKey,
	byKey map[files.EpisodeKey]files.EpisodeStruct,
	byHash map[string][]files.EpisodeStruct,
	force bool,
	watching map[int]bool,
) []files.EpisodeKey

func removeEpisodesAndLinks(
	fm FileManagerInterface,
	backend torrents.TorrentBackend,
	librarian files.Librarian,
	keysToDelete []files.EpisodeKey,
	savedEpisodes []files.EpisodeStruct,
	keepData bool,
	force bool,
	watching map[int]bool,
) error

type handleEpisodesData struct {
	savedEpisodes   []files.EpisodeStruct
	keysToDelete    []files.EpisodeKey
	checkedEpisodes []files.EpisodeKey
	newEpisodes     []files.EpisodeStruct
	watching        map[int]bool
}

func deleteEpisodesByStatus(
	deletableMedia map[int]bool,
	fileManager FileManagerInterface,
	backend torrents.TorrentBackend,
	librarian files.Librarian,
	savedEpisodes []files.EpisodeStruct,
	newEpisodes []files.EpisodeStruct,
	watching map[int]bool,
)
```

Helpers already in `episodes_test.go`: `packRange`, `keysOf`, `fakeWithTorrents`, `testLibrarian`, `mockFileManagerForEpisodes`, `kaguyaS1ID = 101921`.

---

### Task 1: `unclaimedHolds` and the `watching` argument

**Files:**
- Modify: `src/internal/daemon/episodes.go` (`removeEpisodesAndLinks`, `canRemoveTorrent`, `forgettableKeys`; both internal `canRemoveTorrent` call sites; `RemoveEpisodesWithLinks` and `RemoveTorrentWithEpisodes` pass `nil`; `deleteEpisodesByStatus` / `handleSavedEpisodes` pass `nil` until later tasks)
- Modify: `src/internal/daemon/episodes_test.go` (new pins + every existing `removeEpisodesAndLinks(...)` gains `, nil`)
- Modify: `src/internal/daemon/orchestration_test.go` (every `removeEpisodesAndLinks(...)` gains `, nil`)
- Modify: `src/internal/daemon/selection_test.go` (`canRemoveTorrent(group, deleteSet)` → `canRemoveTorrent(group, deleteSet, nil)`)

**Interfaces:**
- Consumes: existing `hasUnclaimedContent`, `packRange`, `keysOf`
- Produces: `unclaimedHolds`, `removeEpisodesAndLinks(..., watching map[int]bool)`, `canRemoveTorrent(..., watching)`, `forgettableKeys(..., watching)`

- [ ] **Step 1: Write the failing predicate tests**

Append to `src/internal/daemon/episodes_test.go` (after `TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack` is fine; keep it next to the other `removeEpisodesAndLinks` pins):

```go
// Off-list bypass (#117): an empty watching map is "nobody is watching", not "unknown".
// Unknown is nil and keeps today's #74 hold (KeepsPackWithUnrecordedContent).
func TestRemoveEpisodesAndLinks_DropsUnclaimedPackWhenWatchingEmpty(t *testing.T) {
	const hash = "8888888888888888888888888888888888888888"
	saved := packRange(108465, 1, 11, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false, map[int]bool{}); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("unclaimed content does not hold the torrent when no claimed anime is in watching")
	}
	if len(fm.deletedEpisodeKeys) != len(saved) {
		t.Errorf("the S1 records must leave with the torrent, got %d deleted", len(fm.deletedEpisodeKeys))
	}
}

func TestRemoveEpisodesAndLinks_KeepsUnclaimedPackWhenAnimeStillWatching(t *testing.T) {
	const hash = "9999999999999999999999999999999999999999"
	saved := packRange(108465, 1, 11, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false, map[int]bool{108465: true}); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("unclaimed content still holds while a claimed anime is in watching")
	}
	if len(fm.deletedEpisodeKeys) != 0 {
		t.Errorf("forgettableKeys must keep the records of a held pack, got %d deleted", len(fm.deletedEpisodeKeys))
	}
}
```

The new tests name a `watching` argument the production function does not have yet. Step 3 adds that argument and threads `nil` at every existing call site so the tests compile; the body still uses `hasUnclaimedContent`, so the empty-map test then fails on behaviour.

- [ ] **Step 2: Confirm the old pins still pass on HEAD**

Run: `go test ./src/internal/daemon/ -run 'TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent|TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack' -count=1`

Expected: PASS (HEAD behaviour). Do not run the new tests yet; they do not compile.

- [ ] **Step 3: Add the `watching` parameter and thread `nil` so the tree compiles**

In `episodes.go`, change the signature of `removeEpisodesAndLinks` to add `watching map[int]bool` as the last parameter. Update its comment: `watching == nil` means the caller is not the automatic pass, so unclaimed content holds as in #74; a non-nil map (even empty) is this pass's watching set (#117).

Pass `watching` through:

- both `canRemoveTorrent(byHash[ep.EpisodeHash], deleteSet)` calls become `canRemoveTorrent(byHash[ep.EpisodeHash], deleteSet, watching)`
- `forgettableKeys(..., force)` becomes `forgettableKeys(..., force, watching)`

`RemoveEpisodesWithLinks` last line:

```go
return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, false, false, nil)
```

`RemoveTorrentWithEpisodes` last line (force stays `true`):

```go
return removeEpisodesAndLinks(fm, backend, librarian, keys, saved, opts.KeepData, true, nil)
```

`deleteEpisodesByStatus` inner call: add `, nil`.
`handleSavedEpisodes` inner call: add `, nil`.

Every `removeEpisodesAndLinks(...)` in `episodes_test.go` and `orchestration_test.go`: add `, nil` except the two new tests, which already pass a map.

`canRemoveTorrent` signature:

```go
func canRemoveTorrent(group []files.EpisodeStruct, deleteSet map[files.EpisodeKey]bool, watching map[int]bool) bool {
```

Keep the body as `return !hasUnclaimedContent(group)` for this step (watching unused). Add `watching` to `forgettableKeys` and keep `hasUnclaimedContent(...)` in its loop.

`selection_test.go` line that calls `canRemoveTorrent(group, deleteSet)` → `canRemoveTorrent(group, deleteSet, nil)`.

- [ ] **Step 4: Run the empty-map test and watch it fail on behaviour**

Run: `go test ./src/internal/daemon/ -run TestRemoveEpisodesAndLinks_DropsUnclaimedPackWhenWatchingEmpty -count=1`

Expected: FAIL — `unclaimed content does not hold the torrent when no claimed anime is in watching` (torrent still in the fake backend). If it fails to compile, a call site was missed.

- [ ] **Step 5: Implement `unclaimedHolds` and switch the two call sites**

Add next to `hasUnclaimedContent` in `episodes.go`:

```go
// unclaimedHolds is the second gate on #74: unclaimed content holds the torrent unless the
// automatic pass has a watching set and no claimed anime of the group is in it (decisions.md #117).
// watching == nil is the API / unknown default and keeps today's hold.
func unclaimedHolds(group []files.EpisodeStruct, watching map[int]bool) bool {
	if !hasUnclaimedContent(group) {
		return false
	}
	if watching == nil {
		return true
	}
	for _, ep := range group {
		if watching[ep.AnimeID] {
			return true
		}
	}
	return false
}
```

`canRemoveTorrent` last line: `return !unclaimedHolds(group, watching)`

Update its comment: the second condition is `unclaimedHolds`, not raw `hasUnclaimedContent`. Off-list (non-nil empty watching) lets the torrent leave even when the span exceeds claimed totals.

`forgettableKeys` loop:

```go
		if ok && ep.EpisodeHash != "" && unclaimedHolds(byHash[ep.EpisodeHash], watching) {
			continue
		}
```

Update its comment: records stay only while the torrent is held for unclaimed content that still has a watching claim (or `watching == nil`). When the pack leaves, the records leave with it so they cannot orphan a deleted torrent.

`force` still short-circuits `forgettableKeys` and `canRemoveTorrent` callers (`if force || canRemoveTorrent(...)`). Do not look at `watching` when `force` is true.

- [ ] **Step 6: Run the daemon tests**

Run: `go test ./src/internal/daemon/ -count=1`

Expected: PASS, including:

- `TestRemoveEpisodesAndLinks_DropsUnclaimedPackWhenWatchingEmpty`
- `TestRemoveEpisodesAndLinks_KeepsUnclaimedPackWhenAnimeStillWatching`
- `TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent` (`nil` watching)
- `TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`
- `TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard`
- `TestSelectEpisodes_PackRotation`

Then: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/episodes_test.go src/internal/daemon/orchestration_test.go src/internal/daemon/selection_test.go
git commit -m "feat: hold unclaimed packs only while a claimed anime is watching"
```

---

### Task 2: Automatic pass — `handleSavedEpisodes` watching set

**Files:**
- Modify: `src/internal/daemon/episodes.go` (`handleEpisodesData.watching`; `handleSavedEpisodes` forwards it instead of `nil`)
- Modify: `src/internal/daemon/verification.go` (build the map from `animes`, pass it in `handleEpisodesData`)
- Modify: `src/internal/daemon/episodes_test.go` (flip the two wait-for-S2 tests; add keep-while-watching)

**Interfaces:**
- Consumes: `removeEpisodesAndLinks(..., watching)` from Task 1
- Produces: `handleEpisodesData.watching map[int]bool`; verification always passes a non-nil map

- [ ] **Step 1: Write the failing keep-while-watching test and flip the two wait tests**

Replace `TestHandleSavedEpisodes_KaguyaMultiSeasonPackWaitsForS2NotInWatching` with:

```go
// Completing Kaguya S1 with S2 still absent from watching: identifyEpisodesNotInWatching puts
// every S1 record in the delete set. The flattened span still exceeds S1's 12 (EMBER 1-24,
// DB 1-41), but #117 drops the pack because nobody in the group is in watching.
func TestHandleSavedEpisodes_KaguyaMultiSeasonPackDropsWhenS2NotInWatching(t *testing.T) {
	cases := []struct {
		name   string
		html   string
		url    string
		wantHi int
	}{
		{name: "EMBER S1+S2 nyaa.si/view/1355132", html: emberKaguyaS1S2HTML(), url: "https://nyaa.si/view/1355132", wantHi: 24},
		{name: "DB S1+S2+S3+Movie+OVA nyaa.si/view/1928670", html: dbKaguyaS1S2S3HTML(), url: "https://nyaa.si/view/1928670", wantHi: 41},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer mockKaguyaFileList(tc.html)()

			info, ok := nyaa.PackFileRange(tc.url, 12)
			if !ok {
				t.Fatal("want PackFileRange to read the file list")
			}
			if info.StartEpisode != 1 || info.EndEpisode != tc.wantHi {
				t.Fatalf("stored range under S1: want 1-%d, got %d-%d", tc.wantHi, info.StartEpisode, info.EndEpisode)
			}

			const hash = "kaguyahashkaguyahashkaguyahashkaguyaha"
			saved := packRange(kaguyaS1ID, 1, 12, 12, info.StartEpisode, info.EndEpisode, hash)
			backend := fakeWithTorrents(hash)
			fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

			handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true}, backend, testLibrarian(), handleEpisodesData{
				savedEpisodes:   saved,
				checkedEpisodes: nil,
				watching:        map[int]bool{},
			})

			if _, still := backend.Get(hash); still {
				t.Fatal("S1 left watching and S2 was never adopted: the torrent must leave")
			}
			if len(fm.deletedEpisodeKeys) != len(saved) {
				t.Errorf("the S1 records must leave with the torrent, got %d deleted", len(fm.deletedEpisodeKeys))
			}
		})
	}
}
```

Replace `TestHandleSavedEpisodes_ContinuousTwoSeasonPackWaitsForS2NotInWatching` with:

```go
func TestHandleSavedEpisodes_ContinuousTwoSeasonPackDropsWhenS2NotInWatching(t *testing.T) {
	const hash = "continuoushashcontinuoushashcontinuous"
	saved := packRange(kaguyaS1ID, 1, 12, 12, 1, 24, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		checkedEpisodes: nil,
		watching:        map[int]bool{},
	})

	if _, ok := backend.Get(hash); ok {
		t.Fatal("a 1-24 pack under a 12-episode S1 leaves once S1 is off-list")
	}
	if len(fm.deletedEpisodeKeys) != len(saved) {
		t.Errorf("the S1 records must leave with the torrent, got %d deleted", len(fm.deletedEpisodeKeys))
	}
}
```

Add immediately after that:

```go
func TestHandleSavedEpisodes_KeepsUnclaimedPackWhileS1StillWatching(t *testing.T) {
	const hash = "watchkeepwatchkeepwatchkeepwatchkeepwatc"
	saved := packRange(kaguyaS1ID, 1, 12, 12, 1, 24, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		keysToDelete:    keysOf(saved),
		checkedEpisodes: keysOf(saved),
		watching:        map[int]bool{kaguyaS1ID: true},
	})

	if _, ok := backend.Get(hash); !ok {
		t.Fatal("watched pruning must not drop a multi-season pack while S1 is still in watching")
	}
	if len(fm.deletedEpisodeKeys) != 0 {
		t.Errorf("forgettableKeys must keep the records of a held pack, got %d deleted", len(fm.deletedEpisodeKeys))
	}
}
```

- [ ] **Step 2: Run the flipped tests and watch them fail**

Run: `go test ./src/internal/daemon/ -run 'TestHandleSavedEpisodes_KaguyaMultiSeasonPackDropsWhenS2NotInWatching|TestHandleSavedEpisodes_ContinuousTwoSeasonPackDropsWhenS2NotInWatching' -count=1`

Expected: FAIL — torrent still present, because `handleSavedEpisodes` still passes `nil` into `removeEpisodesAndLinks`. The keep-while-watching test would already pass (nil watching holds); do not treat that as done.

- [ ] **Step 3: Forward `watching` from `handleSavedEpisodes`**

Add to `handleEpisodesData`:

```go
	watching        map[int]bool
```

In `handleSavedEpisodes`, change the `removeEpisodesAndLinks` call's last argument from `nil` to `data.watching`.

- [ ] **Step 4: Build the watching map in `verification.go` and pass it**

In `verification.go`, in Phase 3, immediately before `deleteEpisodesByStatus`:

```go
	watching := make(map[int]bool, len(animes))
	for _, a := range animes {
		watching[a.Media.Id] = true
	}
```

`make` is non-nil even when `animes` is empty.

Pass it on `handleEpisodesData`:

```go
	handleSavedEpisodes(fileManager, configs, backend, librarian, handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    keysToDelete,
		checkedEpisodes: checkedEpisodes,
		newEpisodes:     newEpisodes,
		watching:        watching,
	})
```

Leave `deleteEpisodesByStatus(...)` as it is this task (still `nil` watching internally).

- [ ] **Step 5: Run the daemon tests**

Run: `go test ./src/internal/daemon/ -count=1`

Expected: PASS, including the three tests above and `TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent` (S2 records survive the delete set; `watching` may stay nil in that test).

Then: `go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/verification.go src/internal/daemon/episodes_test.go
git commit -m "feat: drop a multi-season pack when no claimed season is watching"
```

---

### Task 3: `deleteEpisodesByStatus` sees `newEpisodes` and watching

**Files:**
- Modify: `src/internal/daemon/episodes.go` (`deleteEpisodesByStatus` signature and snapshot)
- Modify: `src/internal/daemon/verification.go` (pass `newEpisodes` and `watching`)
- Modify: `src/internal/daemon/episodes_test.go` (existing `deleteEpisodesByStatus` calls gain `nil, nil`; new same-pass pin)

**Interfaces:**
- Consumes: `watching` map from Task 2; `removeEpisodesAndLinks(..., watching)` from Task 1
- Produces: `deleteEpisodesByStatus(..., savedEpisodes, newEpisodes, watching)` using `saved ∪ new` as the snapshot and `watching` as the gate

- [ ] **Step 1: Write the failing same-pass pin**

Append to `episodes_test.go`:

```go
func TestDeleteEpisodesByStatus_NewEpisodesProtectSharedTorrent(t *testing.T) {
	const hash = "statusadoptstatusadoptstatusadoptstatusa"
	const s1, s2 = 108465, 127720
	saved := packRange(s1, 1, 11, 11, 1, 23, hash)
	newEps := packRange(s2, 1, 12, 12, -10, 12, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	deleteEpisodesByStatus(map[int]bool{s1: true}, fm, backend, testLibrarian(), saved, newEps, map[int]bool{s2: true})

	if _, ok := backend.Get(hash); !ok {
		t.Fatal("status deletion of S1 must not drop the pack S2 just adopted")
	}
}
```

Existing call sites still have the old arity; this test will not compile until Step 3. Write it first anyway, then in Step 3 update every call together.

- [ ] **Step 2: Update existing `deleteEpisodesByStatus` tests to the new arity with `nil, nil`**

In `episodes_test.go`, every current call:

```go
deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)
```

becomes:

```go
deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes, nil, nil)
```

There are three: `TestDeleteEpisodesByStatus_DroppedAnime`, `TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted`, `TestDeleteEpisodesByStatus_DeleteErrorIsTolerated`. `nil, nil` keeps today's #74 hold, which these tests do not exercise (they are not unclaimed packs).

- [ ] **Step 3: Change `deleteEpisodesByStatus` and the verification call**

Replace the function in `episodes.go` with:

```go
func deleteEpisodesByStatus(deletableMedia map[int]bool, fileManager FileManagerInterface, backend torrents.TorrentBackend, librarian files.Librarian, savedEpisodes []files.EpisodeStruct, newEpisodes []files.EpisodeStruct, watching map[int]bool) {
	if len(deletableMedia) == 0 {
		return
	}

	logger.Logger.Debug().
		Msg("Running status-based episode deletion")

	var keysToDelete []files.EpisodeKey
	for _, ep := range savedEpisodes {
		if deletableMedia[ep.AnimeID] && !ep.ManuallyManaged {
			keysToDelete = append(keysToDelete, ep.Key())
		}
	}

	if len(keysToDelete) == 0 {
		logger.Logger.Debug().Msg("Status-based deletion: no matching episodes found")
		return
	}

	logger.Logger.Info().
		Int("count", len(keysToDelete)).
		Msg("Deleting episodes for animes with delete statuses")

	saved := append(append([]files.EpisodeStruct{}, savedEpisodes...), newEpisodes...)
	if err := removeEpisodesAndLinks(fileManager, backend, librarian, keysToDelete, saved, false, false, watching); err != nil {
		logger.Logger.Warn().Err(err).Msg("Status-based deletion: failed to delete episodes from file")
	}
}
```

Keys still come from `savedEpisodes` only (S1). The snapshot passed to the guard is `saved ∪ new` so S2's new records survive the delete set. Comment that: without `newEpisodes`, off-list bypass would delete the torrent this pass just adopted (defect A on the status path, decisions.md #117).

In `verification.go` Phase 3, the watching map from Task 2 is already there. Change the call to:

```go
	deleteEpisodesByStatus(deletableMedia, fileManager, backend, librarian, savedEpisodes, newEpisodes, watching)
```

- [ ] **Step 4: Run the daemon tests**

Run: `go test ./src/internal/daemon/ -run 'TestDeleteEpisodesByStatus_' -count=1`

Expected: PASS, including `TestDeleteEpisodesByStatus_NewEpisodesProtectSharedTorrent`.

Then: `go test ./src/internal/daemon/ -count=1` and `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/verification.go src/internal/daemon/episodes_test.go
git commit -m "fix: keep a pack status-deletion adopts in the same pass"
```

---

### Task 4: Docs

**Files:**
- Modify: `docs/agents/decisions.md` (index + #117 + #116 sentence + #74 location)
- Modify: `docs/agents/architecture.md` (deletion bullet)
- Modify: `docs/TODO.md` (Method A)
- Modify: `docs/superpowers/specs/README.md` (drop the row)
- Delete: `docs/superpowers/specs/2026-10-30-off-list-bypass-unclaimed-pack-guard-design.md`

**Interfaces:**
- Consumes: behaviour from Tasks 1–3; decision number **#117**
- Produces: nothing in code

- [ ] **Step 1: Index + #117**

In `docs/agents/decisions.md`, add after the #116 index line:

```
- [**#117** — Off-list bypasses the unclaimed pack guard](#117-off-list-bypasses-the-unclaimed-pack-guard)
```

Append at the end of the file:

```
### 117. Off-list bypasses the unclaimed pack guard

**Location:** `internal/daemon/episodes.go` (`unclaimedHolds`, `canRemoveTorrent`,
`forgettableKeys`, `removeEpisodesAndLinks`, `handleSavedEpisodes`, `deleteEpisodesByStatus`),
`internal/daemon/verification.go` (watching map from `animes`, `newEpisodes` into status
deletion). Pinned by `TestRemoveEpisodesAndLinks_DropsUnclaimedPackWhenWatchingEmpty`,
`TestRemoveEpisodesAndLinks_KeepsUnclaimedPackWhenAnimeStillWatching`,
`TestHandleSavedEpisodes_KaguyaMultiSeasonPackDropsWhenS2NotInWatching`,
`TestHandleSavedEpisodes_ContinuousTwoSeasonPackDropsWhenS2NotInWatching`,
`TestHandleSavedEpisodes_KeepsUnclaimedPackWhileS1StillWatching` and
`TestDeleteEpisodesByStatus_NewEpisodesProtectSharedTorrent`
(`internal/daemon/episodes_test.go`).

**What it looks like:** `#74`'s unclaimed-content hold is gated on this pass's watching set
(media ids in `DownloadStatuses` of any account, plus standalones). A S1+S2 pack stored as
`1..24` under a 12-episode S1 still has unclaimed content, but once S1 has left watching and
S2 was not adopted, `canRemoveTorrent` returns true and `forgettableKeys` forgets the
records. While S1 is still in watching, watched-episode pruning does not drop the pack.
`watching == nil` is the API default and keeps today's `#74` hold. Manual torrent delete
still passes `force`.

**Why it's right:** keeping the torrent after S1 left, in case S2 might enter watching later,
left multi-season packs on disk forever when the user never added S2. Same-pass adoption is
already paid for: `handleSavedEpisodes` merges `newEpisodes`, and status deletion now uses
the same snapshot so it cannot delete the hash S2 just adopted. Sequel ids are still not
stored (#116 / #71). Empty vs nil is the gate: the automatic pass always passes a non-nil
map (`make` even when `animes` is empty).

**Don't "fix" by:** dropping `hasUnclaimedContent` (watched pruning of S1 would then delete
S2's files while S1 is still CURRENT); walking SEQUEL at deletion; treating a nil watching
map as off-list (a per-episode API delete would drop the pack).
```

- [ ] **Step 2: Amend #74 location and #116 sentence**

In #74's **Location:** add `unclaimedHolds` to the function list and a pointer: the second gate is [#117](#117-off-list-bypasses-the-unclaimed-pack-guard). Do not rewrite the rest of #74.

In #116 **What it looks like**, replace:

`Completing S1 with S2 still absent from watching keeps the torrent ([#74](#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records)).`

with:

`Completing S1 with S2 still absent from watching leaves a span larger than S1's total, so [#74](#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records) still sees unclaimed content; the torrent is kept only while a claimed season is in watching ([#117](#117-off-list-bypasses-the-unclaimed-pack-guard)).`

- [ ] **Step 3: `architecture.md` deletion bullet**

Replace the sentence that currently ends with `every episode the pack covers is claimed by some entry in the group (\`canRemoveTorrent\` / \`hasUnclaimedContent\`, decisions.md #74)` with:

A batch torrent shared by multiple episodes is only removed once **all** its episodes are in the delete set **and** unclaimed content does not hold (`canRemoveTorrent` / `unclaimedHolds`, [decisions.md #74](decisions.md#74-the-pack-exclusion-guards-unit-is-the-torrents-contents-not-the-list-of-records) / [decisions.md #117](decisions.md#117-off-list-bypasses-the-unclaimed-pack-guard)): either the pack's span is fully claimed, or no claimed anime of the group is in this pass's watching set. `watching == nil` (manual per-episode delete) keeps the #74 hold. While it is held back, its records are kept too (`forgettableKeys`) — they carry the declared range. The manual torrent-delete path passes `force` and bypasses that guard.

Keep the rest of the deletion bullet (library hardlink, sweep, `.torrents`).

- [ ] **Step 4: `TODO.md` Method A**

In `## No release — unclaimed extras keep a pack after a list switch`, replace the last sentence `Method A (off-list bypasses the unclaimed guard) stays on the table.` with:

Method A (off-list bypasses the unclaimed guard) shipped as [decisions.md #117](agents/decisions.md#117-off-list-bypasses-the-unclaimed-pack-guard).

- [ ] **Step 5: Drop the executed spec**

Delete `docs/superpowers/specs/2026-10-30-off-list-bypass-unclaimed-pack-guard-design.md`.

In `docs/superpowers/specs/README.md`, remove the `off-list-bypass-unclaimed-pack-guard` row. Leave the other three.

The plan file stays.

- [ ] **Step 6: Commit**

```bash
git add docs/agents/decisions.md docs/agents/architecture.md docs/TODO.md docs/superpowers/specs/README.md
git add -u docs/superpowers/specs/2026-10-30-off-list-bypass-unclaimed-pack-guard-design.md
git commit -m "docs: record off-list bypass of the unclaimed pack guard"
```

---

## Self-review (spec coverage)

| Spec requirement | Task |
|---|---|
| `unclaimedHolds`: nil keeps #74; empty map drops; claimed id in map keeps | 1 |
| `canRemoveTorrent` / `forgettableKeys` share that helper; `force` unchanged | 1 |
| `hasUnclaimedContent` / `declaredSpan` untouched | 1 (not modified) |
| `RemoveEpisodesWithLinks` passes nil | 1 |
| `RemoveTorrentWithEpisodes` still `force=true` | 1 |
| Watching map from `animes` (DownloadStatuses union + standalones), always non-nil | 2 |
| `handleEpisodesData.watching`; handleSavedEpisodes forwards it | 2 |
| Flip Kaguya + continuous wait tests; rename to drop | 2 |
| Keep-while-watching pin (S1 in watching, all records in delete set, span 1–24) | 2 |
| Same-pass S2 adoption still keeps (`NewEpisodesProtectSharedTorrent`) | 2 (existing test stays) |
| `deleteEpisodesByStatus` gets `saved ∪ newEpisodes` and watching | 3 |
| Status-deletion same-pass pin | 3 |
| #117, #74 pointer, #116 sentence, architecture, TODO Method A | 4 |
| No sequel ids, no SEQUEL walk, no new endpoint/config/CLI | honored |
| Out of scope: re-ingest wrong span, frontend copy, grace period | honored |

No placeholders. Signatures in Tasks 2–3 match the File structure block. Existing `removeEpisodesAndLinks` / `canRemoveTorrent` / `deleteEpisodesByStatus` call sites are listed in Tasks 1 and 3.
