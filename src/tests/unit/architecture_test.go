package unit

// Architecture fitness checks: the import rules the docs state and the compiler does not enforce.
// An import cycle already fails the build; these are the edges that would compile fine and still
// break a documented design. A rule belongs here only when a doc or a decision states it. Add the
// rule here and the sentence there in the same change.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const modulePrefix = "AutoAnimeDownloader/src/"

// goSource is one non-test Go file under src/internal or src/cmd, parsed once for every rule.
type goSource struct {
	pkg  string // "internal/nyaa", "cmd/cli"
	path string // repo-relative, for the failure message
	file *ast.File
}

var parsedSources struct {
	once  sync.Once
	files []goSource
	err   error
}

// sources parses the tree once for every rule in the package.
func sources(t *testing.T) []goSource {
	t.Helper()
	parsedSources.once.Do(func() {
		parsedSources.files, parsedSources.err = parseSources(repoRoot(t))
	})
	if parsedSources.err != nil {
		t.Fatal(parsedSources.err)
	}
	return parsedSources.files
}

// parseSources reads every non-test Go file under src/internal and src/cmd, nested packages
// included, regardless of build tags, so a windows-only file breaks a rule just like the one the
// test happens to build on. It skips the directories the go tool ignores (testdata, ".x", "_x")
// and the frontend's node_modules.
func parseSources(root string) ([]goSource, error) {
	var out []goSource
	fset := token.NewFileSet()
	src := filepath.Join(root, "src")
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(src, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			pkg, _ := filepath.Rel(src, filepath.Dir(path))
			rel, _ := filepath.Rel(root, path)
			out = append(out, goSource{pkg: filepath.ToSlash(pkg), path: rel, file: parsed})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// internalImports returns the module's own packages a file imports, as "internal/files".
func internalImports(f *ast.File) []string {
	var out []string
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if rest, ok := strings.CutPrefix(p, modulePrefix); ok {
			out = append(out, rest)
		}
	}
	return out
}

// importRule: pkg, and every package nested under it, must not import any of forbidden; nil
// forbidden means "no internal package".
type importRule struct {
	pkg       string
	forbidden []string
	why       string
}

var importRules = []importRule{
	{
		pkg: "internal/datadir",
		why: "leaf package: files and logger both need the path, and files imports logger (architecture.md, Configuration & State Files)",
	},
	{
		pkg:       "internal/files",
		forbidden: []string{"internal/anilist"},
		why:       "anilist_cache is opaque bytes to files so files never imports anilist (architecture.md, Configuration & State Files)",
	},
	{
		pkg:       "internal/nyaa",
		forbidden: []string{"internal/anilist"},
		why:       "nyaa does not import anilist: the daemon passes what it needs, e.g. a bool for FINISHED (decisions/107)",
	},
	{
		pkg:       "internal/torrents",
		forbidden: []string{"internal/files"},
		why:       "torrents does not know files.Config: the daemon applies config-driven guards before Add (decisions/048)",
	},
}

// inPackage reports whether pkg is root or nested under it: a rule on internal/nyaa also binds a
// future internal/nyaa/parse.
func inPackage(pkg, root string) bool {
	return pkg == root || strings.HasPrefix(pkg, root+"/")
}

func TestImportRules(t *testing.T) {
	sources := sources(t)
	for _, r := range importRules {
		seen := false
		for _, s := range sources {
			if !inPackage(s.pkg, r.pkg) {
				continue
			}
			seen = true
			for _, imp := range internalImports(s.file) {
				if r.forbidden == nil || slices.Contains(r.forbidden, imp) {
					t.Errorf("%s imports %s; %s", s.path, imp, r.why)
				}
			}
		}
		if !seen {
			t.Errorf("rule for %s matches no file: the package moved or was renamed, update importRules", r.pkg)
		}
	}
}

// TestDataDirPathIsComputedOnce: the data dir's folder name and the APPDATA lookup are spelled only
// in datadir, so no second copy of the path can disagree with datadir.Path() (architecture.md,
// "computed in one place"). Comments do not count; string literals do.
func TestDataDirPathIsComputedOnce(t *testing.T) {
	for _, s := range sources(t) {
		if inPackage(s.pkg, "internal/datadir") {
			continue
		}
		ast.Inspect(s.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.Contains(v, ".autoAnimeDownloader") || v == "APPDATA" {
				t.Errorf("%s spells %q; build the path with datadir.Path() instead", s.path, v)
			}
			return true
		})
	}
}
