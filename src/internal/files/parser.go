package files

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseEpisodes parses episodes from JSONL format (one JSON object per line)
// Falls back to old text format if JSON parsing fails
func ParseEpisodes(content string) ([]EpisodeStruct, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return []EpisodeStruct{}, nil
	}

	// Try JSONL format first
	episodes, err := parseEpisodesJSONL(content)
	if err == nil {
		return episodes, nil
	}

	// Fall back to old text format
	episodes, err = parseEpisodesTextFormat(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse as JSONL and text format: %w", err)
	}

	return episodes, nil
}

// parseEpisodesJSONL parses episodes from JSONL format (one JSON object per line)
func parseEpisodesJSONL(content string) ([]EpisodeStruct, error) {
	var episodes []EpisodeStruct
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
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

// parseEpisodesTextFormat parses episodes from old text format (id:hash:name)
// Used for migration from old format
func parseEpisodesTextFormat(content string) ([]EpisodeStruct, error) {
	var episodes []EpisodeStruct
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		episode, err := parseEpisodeLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		episodes = append(episodes, episode)
	}

	return episodes, nil
}

func parseEpisodeLine(line string) (EpisodeStruct, error) {
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 2 {
		return EpisodeStruct{}, fmt.Errorf("invalid format: expected 'id:hash' or 'id:hash:name', got '%s'", line)
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return EpisodeStruct{}, fmt.Errorf("invalid episode ID '%s': %w", parts[0], err)
	}

	if parts[1] == "" {
		return EpisodeStruct{}, fmt.Errorf("episode hash cannot be empty")
	}

	episode := EpisodeStruct{
		EpisodeID:    id,
		EpisodeHash:  parts[1],
		DownloadDate: time.Now(), // Default to current time for migrated episodes
	}

	if len(parts) == 3 {
		episode.EpisodeName = parts[2]
	}

	return episode, nil
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
