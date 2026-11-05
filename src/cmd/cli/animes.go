package main

import (
	"fmt"
	"strconv"

	"AutoAnimeDownloader/src/internal/api"

	"github.com/urfave/cli/v2"
)

func animesCommand() *cli.Command {
	return &cli.Command{
		Name:  "animes",
		Usage: "List and manage animes",
		Action: func(c *cli.Context) error {
			return handleAnimes()
		},
		Subcommands: []*cli.Command{
			{
				Name:      "search",
				Usage:     "Search AniList for animes to add",
				ArgsUsage: "<query>",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "unreleased", Usage: "Include unreleased titles"},
				},
				Action: handleAnimesSearch,
			},
			{
				Name:      "candidates",
				Usage:     "List ranked whole-title torrent candidates for an anime",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "query", Usage: "Custom Nyaa search query"},
				},
				Action: handleAnimesCandidates,
			},
			{
				Name:      "add",
				Usage:     "Add a standalone anime by AniList media id",
				ArgsUsage: "<media_id>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "magnet", Usage: "Magnet URI (skips auto-pick)"},
				},
				Action: handleAnimesAdd,
			},
			{
				Name:      "remove",
				Usage:     "Remove a standalone anime",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					yesFlag(),
					&cli.BoolFlag{Name: "delete-episodes", Usage: "Also delete downloaded episodes"},
				},
				Action: handleAnimesRemove,
			},
			{
				Name:      "settings",
				Usage:     "Show or update anime settings",
				ArgsUsage: "<id> [progress <n>]",
				Action:    handleAnimesSettings,
			},
			{
				Name:      "debug",
				Usage:     "Run the search/match pipeline for one anime without downloading",
				ArgsUsage: "<id>",
				Description: "Same pass as the daemon's --debug-anime flag, run by the running daemon:\n" +
					"prints which episodes would be searched and how many magnets each found.\n" +
					"The full trace (AniList response, every Nyaa row, what matched) goes to the\n" +
					"daemon log — read it with `logs`. Can take minutes on a long series.",
				Action: handleAnimesDebug,
			},
			{
				Name:      "episodes",
				Usage:     "List episodes of an anime",
				ArgsUsage: "<id>",
				Action:    handleAnimesEpisodes,
			},
			{
				Name:      "download",
				Usage:     "Download an episode",
				ArgsUsage: "<id> <episode>",
				Action:    handleAnimesDownload,
			},
			{
				Name:      "redownload",
				Usage:     "Redownload one or more episodes",
				ArgsUsage: "<id> <episode> [<episode> ...]",
				Description: "With several episodes, one failing does not stop the others; the ones that\n" +
					"failed are listed and the command exits non-zero.",
				Flags:  []cli.Flag{yesFlag()},
				Action: handleAnimesRedownload,
			},
			{
				Name:      "delete",
				Usage:     "Delete one or more downloaded episodes",
				ArgsUsage: "<id> <episode> [<episode> ...]",
				Description: "Several episodes are deleted in one request, so a pack they share is freed.\n" +
					"If any of them is not downloaded, nothing is deleted.",
				Flags:  []cli.Flag{yesFlag()},
				Action: handleAnimesDelete,
			},
			{
				Name:      "release",
				Usage:     "Release an episode from manual management",
				ArgsUsage: "<id> <episode>",
				Action:    handleAnimesRelease,
			},
			{
				Name:      "replace",
				Usage:     "Replace an anime with a magnet",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					yesFlag(),
					&cli.StringFlag{Name: "magnet", Usage: "Magnet URI"},
				},
				Action: handleAnimesReplace,
			},
			{
				Name:      "replace-episode",
				Usage:     "Replace an episode with a magnet",
				ArgsUsage: "<id> <episode>",
				Flags: []cli.Flag{
					yesFlag(),
					&cli.StringFlag{Name: "magnet", Usage: "Magnet URI"},
				},
				Action: handleAnimesReplaceEpisode,
			},
		},
	}
}

func handleAnimes() error {
	animes, err := getClient().GetAnimes()
	if err != nil {
		return mapClientError(err)
	}
	return printValue(animes, "No animes found")
}

func handleAnimesSearch(c *cli.Context) error {
	if err := requireArgs(c, 1, "animes search <query>"); err != nil {
		return err
	}
	results, err := getClient().SearchAniList(c.Args().Get(0), c.Bool("unreleased"))
	if err != nil {
		return mapClientError(err)
	}
	return printValue(results, "No results")
}

func handleAnimesCandidates(c *cli.Context) error {
	if err := requireArgs(c, 1, "animes candidates <id>"); err != nil {
		return err
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return err
	}
	candidates, err := getClient().GetTorrentCandidates(id, c.String("query"))
	if err != nil {
		return mapClientError(err)
	}
	return printValue(candidates, "No candidates")
}

func handleAnimesAdd(c *cli.Context) error {
	if err := requireArgs(c, 1, "animes add <media_id>"); err != nil {
		return err
	}
	mediaID, err := parsePositiveInt(c.Args().Get(0), "media_id")
	if err != nil {
		return err
	}
	req := api.StandaloneAnimeRequest{MediaID: mediaID}
	if magnet := c.String("magnet"); magnet != "" {
		req.Magnet = magnet
	}
	added, err := getClient().AddStandaloneAnime(req)
	if err != nil {
		return mapClientError(err)
	}
	return printValue(added, "")
}

func handleAnimesRemove(c *cli.Context) error {
	if err := requireYes(c, "--delete-episodes"); err != nil {
		return err
	}
	if err := requireArgs(c, 1, "animes remove <id> --yes"); err != nil {
		return err
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().RemoveStandaloneAnime(id, c.Bool("delete-episodes"))); err != nil {
		return err
	}
	return printMessage("Standalone anime removed", "Standalone anime removed")
}

func handleAnimesSettings(c *cli.Context) error {
	switch {
	case c.NArg() == 1:
		id, err := parsePositiveInt(c.Args().Get(0), "id")
		if err != nil {
			return err
		}
		settings, err := getClient().GetAnimeSettings(id)
		if err != nil {
			return mapClientError(err)
		}
		return printValue(settings, "")
	case c.NArg() == 3 && c.Args().Get(1) == "progress":
		id, err := parsePositiveInt(c.Args().Get(0), "id")
		if err != nil {
			return err
		}
		progress, err := parseNonNegativeInt(c.Args().Get(2), "progress")
		if err != nil {
			return err
		}
		if err := mapClientError(getClient().UpdateAnimeSettings(id, progress)); err != nil {
			return err
		}
		return printMessage(fmt.Sprintf("Progress set to %d", progress), "Progress updated")
	default:
		return fmt.Errorf("usage: animes settings <id> [progress <n>]")
	}
}

func handleAnimesEpisodes(c *cli.Context) error {
	if err := requireArgs(c, 1, "animes episodes <id>"); err != nil {
		return err
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return err
	}
	detail, err := getClient().GetAnimeEpisodes(id)
	if err != nil {
		return mapClientError(err)
	}
	if outputJSON {
		outputJSONResponse(detail)
		return nil
	}
	return writeSliceTable(detail.Episodes, "No episodes found")
}

func handleAnimesDebug(c *cli.Context) error {
	if err := requireArgs(c, 1, "animes debug <id>"); err != nil {
		return err
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return err
	}
	summary, err := getClient().RunAnimeDebug(id)
	if err != nil {
		return mapClientError(err)
	}
	if outputJSON {
		outputJSONResponse(summary)
		return nil
	}
	fmt.Printf("%s (%d)\n", summary.AnimeName, summary.AnimeID)
	return writeSliceTable(summary.Episodes, "No episodes selected")
}

func handleAnimesDownload(c *cli.Context) error {
	id, ep, err := animeEpisodeArgs(c, "animes download <id> <episode>")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().DownloadEpisode(id, ep)); err != nil {
		return err
	}
	return printMessage("Episode download started", "Episode download started")
}

func handleAnimesRedownload(c *cli.Context) error {
	if err := requireYes(c); err != nil {
		return err
	}
	id, episodes, err := animeEpisodesArgs(c, "animes redownload <id> <episode> [<episode> ...] --yes")
	if err != nil {
		return err
	}
	client := getClient()
	if len(episodes) == 1 {
		if err := mapClientError(client.RedownloadEpisode(id, episodes[0])); err != nil {
			return err
		}
		return printMessage("Episode redownload started", "Episode redownload started")
	}

	result, err := client.BatchRedownloadEpisodes(id, episodes)
	if err != nil {
		return mapClientError(err)
	}
	if outputJSON {
		outputJSONResponse(result)
	} else {
		fmt.Printf("Redownload started: %v\n", result.Redownloaded)
		for _, f := range result.Failed {
			fmt.Printf("Failed: episode %d: %s\n", f.Episode, f.Error)
		}
	}
	if len(result.Failed) > 0 {
		return fmt.Errorf("%d of %d episodes failed to redownload", len(result.Failed), len(result.Failed)+len(result.Redownloaded))
	}
	return nil
}

func handleAnimesDelete(c *cli.Context) error {
	if err := requireYes(c); err != nil {
		return err
	}
	id, episodes, err := animeEpisodesArgs(c, "animes delete <id> <episode> [<episode> ...] --yes")
	if err != nil {
		return err
	}
	client := getClient()
	if len(episodes) == 1 {
		if err := mapClientError(client.DeleteEpisode(id, episodes[0])); err != nil {
			return err
		}
		return printMessage("Episode deleted", "Episode deleted")
	}
	if err := mapClientError(client.BatchDeleteEpisodes(id, episodes)); err != nil {
		return err
	}
	return printMessage(fmt.Sprintf("%d episodes deleted", len(episodes)), "Episodes deleted")
}

func handleAnimesRelease(c *cli.Context) error {
	id, ep, err := animeEpisodeArgs(c, "animes release <id> <episode>")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().ReleaseEpisode(id, ep)); err != nil {
		return err
	}
	return printMessage("Episode released", "Episode released")
}

func handleAnimesReplace(c *cli.Context) error {
	if err := requireYes(c); err != nil {
		return err
	}
	if err := requireArgs(c, 1, "animes replace <id> --magnet URI --yes"); err != nil {
		return err
	}
	magnet := c.String("magnet")
	if magnet == "" {
		return fmt.Errorf("usage: animes replace <id> --magnet URI --yes")
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().ReplaceAnime(id, magnet)); err != nil {
		return err
	}
	return printMessage("Anime replacement started", "Anime replacement started")
}

func handleAnimesReplaceEpisode(c *cli.Context) error {
	if err := requireYes(c); err != nil {
		return err
	}
	id, ep, err := animeEpisodeArgs(c, "animes replace-episode <id> <episode> --magnet URI --yes")
	if err != nil {
		return err
	}
	magnet := c.String("magnet")
	if magnet == "" {
		return fmt.Errorf("usage: animes replace-episode <id> <episode> --magnet URI --yes")
	}
	if err := mapClientError(getClient().ReplaceEpisode(id, ep, magnet)); err != nil {
		return err
	}
	return printMessage("Episode replacement started", "Episode replacement started")
}

func animeEpisodeArgs(c *cli.Context, usage string) (int, int, error) {
	if err := requireArgs(c, 2, usage); err != nil {
		return 0, 0, err
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return 0, 0, err
	}
	ep, err := parsePositiveInt(c.Args().Get(1), "episode")
	if err != nil {
		return 0, 0, err
	}
	return id, ep, nil
}

// animeEpisodesArgs is animeEpisodeArgs for commands that take one or more episodes.
func animeEpisodesArgs(c *cli.Context, usage string) (int, []int, error) {
	if c.NArg() < 2 {
		return 0, nil, fmt.Errorf("usage: %s", usage)
	}
	id, err := parsePositiveInt(c.Args().Get(0), "id")
	if err != nil {
		return 0, nil, err
	}
	episodes := make([]int, 0, c.NArg()-1)
	for _, arg := range c.Args().Slice()[1:] {
		ep, err := parsePositiveInt(arg, "episode")
		if err != nil {
			return 0, nil, err
		}
		episodes = append(episodes, ep)
	}
	return id, episodes, nil
}

func parseNonNegativeInt(s, name string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s %q", name, s)
	}
	return n, nil
}
