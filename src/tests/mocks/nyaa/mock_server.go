package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/", handleNyaa)
	log.Printf("Mock Nyaa server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleNyaa(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	scenario := os.Getenv("SCENARIO")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if scenario == "empty" || !strings.Contains(query, "6") {
		// Return empty results
		w.Write([]byte(getEmptyHTML()))
		return
	}

	// Return mock HTML with torrent results
	w.Write([]byte(getMockHTML(query)))
}

func getEmptyHTML() string {
	return `<!DOCTYPE html>
<html>
<head><title>Nyaa - Mock</title></head>
<body>
	<table class="torrent-list">
		<tbody>
		</tbody>
	</table>
</body>
</html>`
}

func getMockHTML(query string) string {
	// Extract episode number from query if possible
	episode := "6"
	if strings.Contains(query, " ") {
		parts := strings.Fields(query)
		if len(parts) > 1 {
			episode = parts[len(parts)-1]
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Nyaa - Mock</title></head>
<body>
	<table class="torrent-list">
		<tbody>
			<tr>
				<td></td>
				<td>
					<a href="#" class="comments">Comments</a>
					<a href="/view/12345">Test Anime - %s [1080p][SubsPlease]</a>
				</td>
				<td>
					<a href="/download/12345.torrent">DL</a>
					<a href="magnet:?xt=urn:btih:test123456789">Magnet</a>
				</td>
				<td></td>
				<td></td>
				<td>100</td>
			</tr>
		</tbody>
	</table>
</body>
</html>`, episode)
}
