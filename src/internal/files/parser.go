package files

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseEpisodes(content string) ([]EpisodeStruct, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return []EpisodeStruct{}, nil
	}

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
		EpisodeID:   id,
		EpisodeHash: parts[1],
	}

	if len(parts) == 3 {
		episode.EpisodeName = parts[2]
	}

	return episode, nil
}

func SerializeEpisode(episode EpisodeStruct) string {
	if episode.EpisodeName == "" {
		return fmt.Sprintf("%d:%s\n", episode.EpisodeID, episode.EpisodeHash)
	}
	return fmt.Sprintf("%d:%s:%s\n", episode.EpisodeID, episode.EpisodeHash, episode.EpisodeName)
}

func SerializeEpisodes(episodes []EpisodeStruct) string {
	var builder strings.Builder
	for _, episode := range episodes {
		builder.WriteString(SerializeEpisode(episode))
	}
	return builder.String()
}
