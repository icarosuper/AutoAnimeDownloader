package files

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseEpisodes reads the episodes file in JSONL (one JSON object per line).
//
// The legacy text format (`id:hash:name`) was REMOVED along with EpisodeStruct.EpisodeID: it
// did not load anime id or episode number, which are exactly the key of an episode today
// (see decisions.md #52). A record like that cannot be identified, so supporting the
// format would only produce colliding (0,0) keys.
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
