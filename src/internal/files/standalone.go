package files

// Standalone animes: media ids the user asked to follow even though they are on no
// AniList list. The file is a JSON array of ids, exactly the blocked_episodes
// convention — the only question asked of it is "is this id here?", so there is no
// object and no added_at.
func (m *FileManager) LoadStandaloneAnimes() ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadIntListLocked(m.standaloneAnimesPath, "standalone animes")
}

// AddStandaloneAnime is idempotent: adding the same id twice neither duplicates nor fails.
func (m *FileManager) AddStandaloneAnime(mediaID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids, err := m.loadIntListLocked(m.standaloneAnimesPath, "standalone animes")
	if err != nil {
		return err
	}

	for _, id := range ids {
		if id == mediaID {
			return nil
		}
	}

	return m.saveIntListLocked(m.standaloneAnimesPath, append(ids, mediaID), "standalone animes")
}

// RemoveStandaloneAnime does not treat a missing id as an error: the caller (DELETE and the
// automatic removal in the verification pass) only wants to know the id is no longer there.
func (m *FileManager) RemoveStandaloneAnime(mediaID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids, err := m.loadIntListLocked(m.standaloneAnimesPath, "standalone animes")
	if err != nil {
		return err
	}

	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if id != mediaID {
			filtered = append(filtered, id)
		}
	}

	if len(filtered) == len(ids) {
		return nil
	}

	return m.saveIntListLocked(m.standaloneAnimesPath, filtered, "standalone animes")
}
