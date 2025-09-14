package modules

import (
	"fmt"
	"log"

	"github.com/irevenko/go-nyaa/nyaa"
)

func SearchNyaa(animeName string, episode int) {
	fmt.Println("Searching...")

	opt := nyaa.SearchOptions{
		Provider: "nyaa",
		Query:    animeName + " " + fmt.Sprintf("%02d", episode),
		Category: "anime-eng",
		SortBy:   "seeders",
	}

	torrents, err := nyaa.Search(opt)
	if err != nil {
		log.Fatal(err)
	}

	for _, t := range torrents {
		fmt.Printf("Name: %s\n", t.Name)
		fmt.Println("---------------------------")
	}
}
