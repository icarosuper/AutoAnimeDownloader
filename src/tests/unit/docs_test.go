package unit

// Docs rot checks: they run with `go test ./...`, so CI and the Stop hook in .claude/settings.json
// fail when a doc pointer stops resolving, instead of an agent following it into nothing.

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// Generated, vendored or scratch trees: nobody writes doc pointers there by hand.
var skippedDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "build": true, "runtime": true,
	".sandbox": true, ".superpowers": true, "test-results": true,
}

// repoFiles lists the files under root whose name matches keep, minus the skipped trees and the
// .debug_* output of `make debug-anime`.
func repoFiles(t *testing.T, root string, keep func(name string) bool) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".debug_") {
				return filepath.SkipDir
			}
			// A nested git worktree (.claude/worktrees/<slice>, docs/v3.0.0.md) is another chat's
			// checkout: its half-done docs are not this tree's.
			if path != root {
				if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if keep(d.Name()) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// decision.sh check already knows the house rules for docs/agents/decisions/ (heading matches the
// filename, index.md is what `decision.sh index` would write, links to entries resolve). It was
// only run by the record-decision skill; here it runs on every test pass.
func TestDecisionsCheckPasses(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, ".claude/skills/record-decision/decision.sh"), "check")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("decision.sh check failed:\n%s", out)
	}
}

var (
	decisionCite    = regexp.MustCompile(`decisions/(\d{3})\b`)
	nonCanonicalCit = regexp.MustCompile(`(?i)\bdecisions?(\.md)? ?#\d+`)
)

// Code cites a decision as `decisions/NNN` (CLAUDE.md). That form is what `decision.sh refs`
// greps, so another spelling hides the pointer from the next person changing the decision.
func TestDecisionCitationsInCodeResolve(t *testing.T) {
	root := repoRoot(t)
	decisionsDir := filepath.Join(root, "docs/agents/decisions")
	entries, err := os.ReadDir(decisionsDir)
	if err != nil {
		t.Fatal(err)
	}
	exists := map[string]bool{}
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[3] == '-' {
			exists[e.Name()[:3]] = true
		}
	}

	isCode := func(name string) bool {
		switch filepath.Ext(name) {
		case ".go", ".ts", ".js", ".svelte", ".sh":
			return true
		}
		return name == "Makefile"
	}
	for _, path := range repoFiles(t, root, isCode) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range decisionCite.FindAllStringSubmatch(line, -1) {
				if !exists[m[1]] {
					t.Errorf("%s:%d: cites decisions/%s, which has no file in docs/agents/decisions/", rel, i+1, m[1])
				}
			}
			if m := nonCanonicalCit.FindString(line); m != "" {
				t.Errorf("%s:%d: %q — cite it as decisions/NNN (zero-padded) so `decision.sh refs` finds it", rel, i+1, m)
			}
		}
	}
}

var (
	inlineLink  = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	refLinkDef  = regexp.MustCompile(`^\s{0,3}\[[^\]]+\]:\s+(\S+)`)
	codeSpan    = regexp.MustCompile("`[^`]*`")
	headingLine = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	htmlAnchor  = regexp.MustCompile(`<a\s+(?:name|id)="([^"]+)"`)
	mdLinkText  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// githubSlug is GitHub's heading anchor: lowercase, keep letters, digits, '-' and '_', spaces
// become '-'; a repeated slug gets -1, -2…
func githubSlug(heading string) string {
	heading = mdLinkText.ReplaceAllString(heading, "$1")
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// mdLines is the file's lines with fenced code blocks blanked: a link inside an example is not a
// link, and a `# comment` inside a shell block is not a heading.
func mdLines(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	inFence := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			lines[i] = ""
			continue
		}
		if inFence {
			lines[i] = ""
		}
	}
	return lines
}

func anchorsOf(lines []string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, l := range lines {
		for _, m := range htmlAnchor.FindAllStringSubmatch(l, -1) {
			anchors[m[1]] = true
		}
		m := headingLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		slug := githubSlug(m[1])
		if n := seen[slug]; n > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			anchors[slug] = true
		}
		seen[slug]++
	}
	return anchors
}

// Every relative link in a Markdown file must point at a file that exists, and a #fragment into
// a Markdown file at a heading it has: renaming a file or a heading must update its referrers.
func TestMarkdownLinksResolve(t *testing.T) {
	root := repoRoot(t)
	files := repoFiles(t, root, func(name string) bool { return strings.HasSuffix(name, ".md") })

	anchorCache := map[string]map[string]bool{}
	anchors := func(path string) map[string]bool {
		if a, ok := anchorCache[path]; ok {
			return a
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		a := anchorsOf(mdLines(data))
		anchorCache[path] = a
		return a
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range mdLines(data) {
			line = codeSpan.ReplaceAllString(line, "")
			var targets []string
			for _, m := range inlineLink.FindAllStringSubmatch(line, -1) {
				targets = append(targets, m[1])
			}
			if m := refLinkDef.FindStringSubmatch(line); m != nil {
				targets = append(targets, m[1])
			}
			for _, target := range targets {
				target = strings.Trim(target, "<>")
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				file, frag, _ := strings.Cut(target, "#")
				dest := path
				if file != "" {
					if strings.HasPrefix(file, "/") {
						dest = filepath.Join(root, file)
					} else {
						dest = filepath.Join(filepath.Dir(path), file)
					}
					if _, err := os.Stat(dest); err != nil {
						t.Errorf("%s:%d: link to %s — no such file", rel, i+1, target)
						continue
					}
				}
				if frag == "" || !strings.HasSuffix(dest, ".md") {
					continue // #L42 into a source file is GitHub's, not ours to check
				}
				if !anchors(dest)[strings.ToLower(frag)] {
					t.Errorf("%s:%d: link to %s — no heading with anchor #%s", rel, i+1, target, frag)
				}
			}
		}
	}
}
