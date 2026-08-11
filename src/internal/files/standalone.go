package files

// Animes avulsos: media ids que o usuario mandou acompanhar sem que eles estejam em nenhuma
// lista da AniList. O arquivo e um array JSON de ids, exatamente a convencao de
// blocked_episodes — a unica pergunta feita a ele e "este id esta aqui?", entao nao ha objeto
// nem added_at.
func (m *FileManager) LoadStandaloneAnimes() ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadIntListLocked(m.standaloneAnimesPath, "standalone animes")
}

// AddStandaloneAnime e idempotente: adicionar duas vezes o mesmo id nao duplica nem falha.
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

// RemoveStandaloneAnime nao trata id ausente como erro: quem chama (o DELETE e a remocao
// automatica do passe de verificacao) so quer saber que o id nao esta mais la.
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
