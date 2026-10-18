package diagnostics

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// logFieldCall matches a zerolog field written with a literal key: .Str("anime", ...), and the
// same at the start of a chained line.
var logFieldCall = regexp.MustCompile(`\b(?:Str|Strs|Int|Ints|Int64|Int32|Uint32|Uint64|Uint|Float64|Floats64|Bool|Bools|Dur|Time|Interface|Any|RawJSON|AnErr|Stringer|Hex|Bytes|Uints|Times|Errs|Object|Array|Dict|IPAddr)\(\s*"([^"]+)"`)

// TestEveryLogKeyIsClassified is the invariant guard of the redactor (decisions.md #103): a new log
// field can't reach a bug report by default. Failing here means adding the key to keyClasses, and
// deciding whether it's kept, dropped, an account name or a title.
func TestEveryLogKeyIsClassified(t *testing.T) {
	srcRoot := filepath.Join("..", "..")
	unclassified := map[string]string{}
	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// src/tests holds test doubles; src/cmd/cli is urfave (c.Bool("flag")), not zerolog.
		// src/cmd/daemon still writes real log fields and must stay in the walk.
		if d.IsDir() && (path == filepath.Join(srcRoot, "tests") || path == filepath.Join(srcRoot, "cmd", "cli")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range logFieldCall.FindAllStringSubmatch(string(data), -1) {
			if _, ok := keyClasses[m[1]]; !ok {
				unclassified[m[1]] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var keys []string
	for k, path := range unclassified {
		keys = append(keys, k+" ("+path+")")
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		t.Fatalf("log keys missing from keyClasses in redact.go:\n%s", strings.Join(keys, "\n"))
	}
}
