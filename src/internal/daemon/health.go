package daemon

import "fmt"

// health.go holds the text and the edge tracking of the alert notifications — the events about the
// library and the daemon as a whole, not about one episode being fetched (notifications.isAlert).

// plural is "1 episode" / "3 episodes".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
