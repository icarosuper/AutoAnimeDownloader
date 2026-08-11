package files

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseEpisodes le o arquivo de episodios em JSONL (um objeto JSON por linha).
//
// O formato de texto legado (`id:hash:name`) foi REMOVIDO junto com EpisodeStruct.EpisodeID: ele
// nao carregava anime id nem numero de episodio, que sao exatamente a chave de um episodio hoje
// (ver decisions.md #52). Um registro desses nao tem como ser identificado, entao suportar o
// formato so serviria para produzir chaves (0,0) colidindo entre si.
func ParseEpisodes(content string) ([]EpisodeStruct, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return []EpisodeStruct{}, nil
	}

	var episodes []EpisodeStruct
	for lineNum, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var episode EpisodeStruct
		if err := json.Unmarshal([]byte(line), &episode); err != nil {
			return nil, fmt.Errorf("line %d: failed to parse JSON: %w", lineNum+1, err)
		}

		episodes = append(episodes, episode)
	}

	return episodes, nil
}

// SerializeEpisode serializes an episode to JSONL format (one JSON object per line)
func SerializeEpisode(episode EpisodeStruct) (string, error) {
	jsonBytes, err := json.Marshal(episode)
	if err != nil {
		return "", fmt.Errorf("failed to marshal episode: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// SerializeEpisodes serializes episodes to JSONL format (one JSON object per line)
func SerializeEpisodes(episodes []EpisodeStruct) (string, error) {
	var builder strings.Builder
	for _, episode := range episodes {
		line, err := SerializeEpisode(episode)
		if err != nil {
			return "", err
		}
		builder.WriteString(line)
	}
	return builder.String(), nil
}
