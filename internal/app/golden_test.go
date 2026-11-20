package app

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

var allRules = []string{
	"V01", "V02", "V03", "V04", "V05", "V06", "V07",
	"V10", "V11", "V12", "V13", "V14", "V15", "V16",
	"V20", "V21", "V22", "V23", "V24",
	"V30", "V31", "V32", "V33", "V34", "V35",
	"V40", "V41", "V42", "V43", "V44", "V45",
	"V50", "V51", "V52", "V53",
	"V60", "V61", "V62", "V63",
	"V70", "V71", "V72", "V73",
}

type caseSpec struct {
	Env    map[string]string `yaml:"env"`
	Delete []string          `yaml:"delete"`
	Empty  []string          `yaml:"empty"`
	Expect *int              `yaml:"expect"`
	Forbid []string          `yaml:"forbid"`
}

func corpusRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "config", "testdata", "config")
}

func TestGoldenCorpus(t *testing.T) {
	root := corpusRoot(t)
	base := filepath.Join(root, "base")
	for _, kind := range []string{"legal", "invalid", "ordering"} {
		dirs := sortedDirs(t, filepath.Join(root, kind))
		for _, caseDir := range dirs {
			caseDir := caseDir
			t.Run(kind+"/"+filepath.Base(caseDir), func(t *testing.T) {
				work := t.TempDir()
				copyTree(t, base, work)
				overlay := filepath.Join(caseDir, "files")
				if _, err := os.Stat(overlay); err == nil {
					copyTree(t, overlay, work)
				}
				spec := loadCaseSpec(t, filepath.Join(caseDir, "case.yaml"))
				applyDeletes(t, work, spec.Delete)
				applyEmpty(t, work, spec.Empty)

				lookup := func(name string) (string, bool) {
					v, ok := spec.Env[name]
					return v, ok
				}
				code, stdout, stderr := run(t, []string{"config-check", "--config", work}, lookup)

				wantExit := 0
				if kind != "legal" {
					wantExit = 1
				}
				if spec.Expect != nil {
					wantExit = *spec.Expect
				}
				if code != wantExit {
					t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, wantExit, stdout, stderr)
				}

				wantStdout := wantFile(t, caseDir, "want.stdout")
				if wantStdout == "" && kind == "legal" {
					wantStdout = readFile(t, filepath.Join(root, "golden", "summary.stdout"))
				}
				wantStderr := wantFile(t, caseDir, "want.stderr")

				if *updateGolden {
					if kind == "invalid" || (kind == "legal" && hasOverlayOrEnv(caseDir, spec)) {
						writeGoldenIfNonEmpty(t, filepath.Join(caseDir, "want.stdout"), stdout)
					}
					writeGoldenIfNonEmpty(t, filepath.Join(caseDir, "want.stderr"), stderr)
					return
				}
				if stdout != wantStdout {
					t.Fatalf("stdout mismatch\n--- want ---\n%s\n--- got ---\n%s", wantStdout, stdout)
				}
				if stderr != wantStderr {
					t.Fatalf("stderr mismatch\n--- want ---\n%s\n--- got ---\n%s", wantStderr, stderr)
				}
				if strings.Contains(stdout+stderr, work) {
					t.Fatalf("temporary directory leaked into output:\n%s%s", stdout, stderr)
				}
				for _, forbidden := range spec.Forbid {
					if strings.Contains(stdout+stderr, forbidden) {
						t.Fatalf("forbidden value %q leaked", forbidden)
					}
				}
			})
		}
	}
}

func TestRuleCatalogCoverage(t *testing.T) {
	root := corpusRoot(t)
	for _, rule := range allRules {
		for _, kind := range []string{"legal", "invalid"} {
			prefix := strings.ToLower(rule) + "-"
			found := false
			for _, dir := range sortedDirs(t, filepath.Join(root, kind)) {
				if strings.HasPrefix(filepath.Base(dir), prefix) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s/%s* missing", kind, prefix)
			}
		}
		invalid := findRuleDir(t, filepath.Join(root, "invalid"), strings.ToLower(rule)+"-")
		if invalid == "" {
			continue
		}
		stderr := readFile(t, filepath.Join(invalid, "want.stderr"))
		if !strings.Contains(stderr, ": "+rule+": ") {
			t.Errorf("%s want.stderr does not contain rule %s:\n%s", invalid, rule, stderr)
		}
	}
	known := map[string]bool{}
	for _, r := range allRules {
		known[strings.ToLower(r)] = true
	}
	for _, kind := range []string{"legal", "invalid"} {
		for _, dir := range sortedDirs(t, filepath.Join(root, kind)) {
			name := filepath.Base(dir)
			if !strings.HasPrefix(name, "v") || len(name) < 4 {
				continue
			}
			if !known[name[:3]] {
				t.Errorf("unknown rule case directory %s", dir)
			}
		}
	}
}

var diagnosticLine = regexp.MustCompile(`^(.+):(\d+):(\d+): (V\d{2}): (.+)$`)

func TestDiagnosticFormat(t *testing.T) {
	root := corpusRoot(t)
	for _, kind := range []string{"invalid", "ordering"} {
		for _, dir := range sortedDirs(t, filepath.Join(root, kind)) {
			stderr := readFile(t, filepath.Join(dir, "want.stderr"))
			for _, line := range strings.Split(strings.TrimSuffix(stderr, "\n"), "\n") {
				if line == "" {
					continue
				}
				m := diagnosticLine.FindStringSubmatch(line)
				if m == nil {
					t.Errorf("%s: malformed diagnostic %q", dir, line)
					continue
				}
				if m[2] == "0" || m[3] == "0" {
					t.Errorf("%s: zero line/column in %q", dir, line)
				}
			}
		}
	}
}

func sortedDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func findRuleDir(t *testing.T, root, prefix string) string {
	t.Helper()
	for _, dir := range sortedDirs(t, root) {
		if strings.HasPrefix(filepath.Base(dir), prefix) {
			return dir
		}
	}
	return ""
}

func loadCaseSpec(t *testing.T, path string) caseSpec {
	t.Helper()
	var spec caseSpec
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return spec
		}
		t.Fatalf("read case spec: %v", err)
	}
	if err := yaml.Load(data, &spec); err != nil {
		t.Fatalf("parse case spec %s: %v", path, err)
	}
	return spec
}

func hasOverlayOrEnv(dir string, spec caseSpec) bool {
	if len(spec.Env) > 0 {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "files"))
	return err == nil
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

func applyDeletes(t *testing.T, work string, rels []string) {
	t.Helper()
	for _, rel := range rels {
		if err := os.RemoveAll(filepath.Join(work, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("delete %s: %v", rel, err)
		}
	}
}

func applyEmpty(t *testing.T, work string, rels []string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(work, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("empty %s: %v", rel, err)
		}
	}
}

func wantFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}
	return readFile(t, path)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func writeGoldenIfNonEmpty(t *testing.T, path, content string) {
	t.Helper()
	if content != "" {
		writeGolden(t, path, content)
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove empty golden %s: %v", path, err)
	}
}
