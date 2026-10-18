package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

func torrentsCommand() *cli.Command {
	return &cli.Command{
		Name:  "torrents",
		Usage: "List and manage torrents",
		Action: func(c *cli.Context) error {
			return handleTorrents()
		},
		Subcommands: []*cli.Command{
			{
				Name:      "files",
				Usage:     "List files inside a torrent",
				ArgsUsage: "<hash>",
				Action:    handleTorrentFiles,
			},
			{
				Name:      "pause",
				Usage:     "Pause a torrent",
				ArgsUsage: "<hash>",
				Action:    handleTorrentPause,
			},
			{
				Name:      "resume",
				Usage:     "Resume a torrent",
				ArgsUsage: "<hash>",
				Action:    handleTorrentResume,
			},
			{
				Name:      "announce",
				Usage:     "Re-announce a torrent to trackers",
				ArgsUsage: "<hash>",
				Action:    handleTorrentAnnounce,
			},
			{
				Name:      "prioritize",
				Usage:     "Move torrent(s) to the front of the download queue",
				ArgsUsage: "<hash> [hash…]",
				Action:    handleTorrentPrioritize,
			},
			{
				Name:      "delete",
				Usage:     "Delete a torrent",
				ArgsUsage: "<hash>",
				Flags: []cli.Flag{
					yesFlag(),
					&cli.BoolFlag{Name: "keep-data", Usage: "Keep downloaded files"},
					&cli.BoolFlag{Name: "block", Usage: "Block the episode from automatic redownload"},
				},
				Action: handleTorrentDelete,
			},
		},
	}
}

func handleTorrents() error {
	list, err := getClient().GetTorrents()
	if err != nil {
		return mapClientError(err)
	}
	return printValue(list, "No torrents found")
}

func handleTorrentFiles(c *cli.Context) error {
	hash, err := torrentHashArg(c, "torrents files <hash>")
	if err != nil {
		return err
	}
	files, err := getClient().GetTorrentFiles(hash)
	if err != nil {
		return mapClientError(err)
	}
	return printValue(files, "No files found")
}

func handleTorrentPause(c *cli.Context) error {
	hash, err := torrentHashArg(c, "torrents pause <hash>")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().PauseTorrent(hash)); err != nil {
		return err
	}
	return printMessage("Torrent paused", "Torrent paused")
}

func handleTorrentResume(c *cli.Context) error {
	hash, err := torrentHashArg(c, "torrents resume <hash>")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().ResumeTorrent(hash)); err != nil {
		return err
	}
	return printMessage("Torrent resumed", "Torrent resumed")
}

func handleTorrentAnnounce(c *cli.Context) error {
	hash, err := torrentHashArg(c, "torrents announce <hash>")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().AnnounceTorrent(hash)); err != nil {
		return err
	}
	return printMessage("Announce sent", "Announce sent")
}

func handleTorrentPrioritize(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("usage: torrents prioritize <hash> [hash…]")
	}
	hashes := c.Args().Slice()
	var err error
	if len(hashes) == 1 {
		err = getClient().PrioritizeTorrent(hashes[0])
	} else {
		err = getClient().PrioritizeTorrents(hashes)
	}
	if err != nil {
		return mapClientError(err)
	}
	return printMessage("Torrent(s) prioritized", "Torrents prioritized")
}

func handleTorrentDelete(c *cli.Context) error {
	if err := requireYes(c, "--keep-data", "--block"); err != nil {
		return err
	}
	hash, err := torrentHashArg(c, "torrents delete <hash> --yes")
	if err != nil {
		return err
	}
	if err := mapClientError(getClient().DeleteTorrent(hash, c.Bool("keep-data"), c.Bool("block"))); err != nil {
		return err
	}
	return printMessage("Torrent deleted", "Torrent deleted")
}

func torrentHashArg(c *cli.Context, usage string) (string, error) {
	if err := requireArgs(c, 1, usage); err != nil {
		return "", err
	}
	hash := c.Args().Get(0)
	if hash == "" {
		return "", fmt.Errorf("usage: %s", usage)
	}
	return hash, nil
}
