// Mock of the one GitHub endpoint the daemon calls: the latest release, for the update check.
// LATEST_TAG sets the tag it answers (default v99.0.0, newer than any build).
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	tag := os.Getenv("LATEST_TAG")
	if tag == "" {
		tag = "v99.0.0"
	}

	http.HandleFunc("/repos/icarosuper/AutoAnimeDownloader/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://github.com/icarosuper/AutoAnimeDownloader/releases/tag/%s","draft":false,"prerelease":false}`, tag, tag)
	})
	log.Printf("Mock GitHub server starting on port %s (latest %s)", port, tag)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
