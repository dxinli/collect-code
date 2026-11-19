package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

type loadOptions struct {
	lookup func(string) (string, bool)
}

// Option customises Load.
type Option func(*loadOptions)

// WithEnv injects the environment lookup used for "env:NAME" secrets.
func WithEnv(lookup func(string) (string, bool)) Option {
	return func(o *loadOptions) {
		if lookup != nil {
			o.lookup = lookup
		}
	}
}

// Load reads and strictly parses the four configuration layers, then applies
// the V01..V63 validation rules. Structural errors (syntax, unknown keys,
// missing files) suppress semantic checks because the decoded model cannot be
// trusted; the returned diagnostics are always sorted and de-duplicated.
func Load(dir string, opts ...Option) (*Bundle, error) {
	o := loadOptions{lookup: defaultLookupEnv}
	for _, opt := range opts {
		opt(&o)
	}

	b := &Bundle{
		Profiles:       map[string]*Profile{},
		ProfileSources: map[string]string{},
	}
	cfg := Default()
	b.Config = &cfg

	var ds Diagnostics
	positions := map[string]*positionIndex{}

	pi, d := parseFile(dir, "config.yaml", b.Config)
	positions["config.yaml"] = pi
	ds = append(ds, d...)

	var links LinksFile
	pi, d = parseFile(dir, "links.yaml", &links)
	positions["links.yaml"] = pi
	ds = append(ds, d...)

	var devices DevicesFile
	pi, d = parseFile(dir, "devices.yaml", &devices)
	positions["devices.yaml"] = pi
	ds = append(ds, d...)

	loaded, sources, profilePositions, d := parseProfiles(dir)
	for file, p := range profilePositions {
		positions[file] = p
	}
	ds = append(ds, d...)

	b.Links = links.Links
	b.Devices = devices.Devices
	b.ProfileSources = sources
	for _, lp := range loaded {
		if lp.profile.Profile != "" {
			b.Profiles[lp.profile.Profile] = lp.profile
		}
	}

	// v4 skips a null struct/scalar but sets a null slice to nil. Normalise
	// nil back to the default list so "null" consistently means "unset";
	// an explicit empty list stays empty and disables key-name redaction.
	if b.Config.Log.RedactFields == nil {
		b.Config.Log.RedactFields = defaultRedactFields()
	}

	if hasStructuralDiagnostics(ds) {
		return nil, finalizeDiagnostics(ds)
	}

	if ds = validate(b, loaded, positions); len(ds) > 0 {
		return nil, finalizeDiagnostics(ds)
	}
	if ds = resolveSecrets(b, o.lookup, positions); len(ds) > 0 {
		return nil, finalizeDiagnostics(ds)
	}
	return b, nil
}

type loadedProfile struct {
	file    string
	profile *Profile
}

// parseFile reads one layer. It keeps the first-pass node tree so later
// semantic diagnostics can point at the exact key that is wrong.
func parseFile(dir, rel string, out any) (*positionIndex, Diagnostics) {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Diagnostics{pathDiagnostic(rel, false, err)}
	}

	var node yaml.Node
	if err := yaml.Load(data, &node); err != nil {
		return nil, classifyYAMLError(rel, err, nil)
	}
	if emptyNode(&node) {
		return nil, Diagnostics{{
			File: rel, Line: 1, Col: 1, RuleID: RuleEmptyDocument, Message: "empty document",
		}}
	}

	pi := buildPositionIndex(&node)
	if err := yaml.Load(data, out, yaml.WithKnownFields(), yaml.WithUniqueKeys()); err != nil {
		return pi, classifyYAMLError(rel, err, pi)
	}
	return pi, nil
}

func parseProfiles(dir string) ([]loadedProfile, map[string]string, map[string]*positionIndex, Diagnostics) {
	sources := map[string]string{}
	positions := map[string]*positionIndex{}

	entries, err := os.ReadDir(filepath.Join(dir, "profiles"))
	if err != nil {
		return nil, sources, positions, Diagnostics{pathDiagnostic("profiles", true, err)}
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	var loaded []loadedProfile
	var ds Diagnostics
	for _, name := range names {
		rel := "profiles/" + name
		var p Profile
		pi, d := parseFile(dir, rel, &p)
		if pi != nil {
			positions[rel] = pi
		}
		if len(d) > 0 {
			ds = append(ds, d...)
			continue
		}
		loaded = append(loaded, loadedProfile{file: rel, profile: &p})
	}

	first := map[string]string{}
	for _, lp := range loaded {
		name := lp.profile.Profile
		if name == "" {
			continue
		}
		if prev, ok := first[name]; ok {
			line, col := 1, 1
			if pi := positions[lp.file]; pi != nil {
				line, col = pi.at(fieldPath(nil, "profile"))
			}
			ds = append(ds, Diagnostic{
				File: lp.file, Line: line, Col: col, RuleID: RuleDuplicateProfile,
				Message: fmt.Sprintf("duplicate profile %q (also defined in %s)", name, prev),
			})
			continue
		}
		first[name] = lp.file
		sources[name] = lp.file
	}
	return loaded, sources, positions, ds
}

func emptyNode(n *yaml.Node) bool {
	if n == nil || n.Kind == 0 {
		return true
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 {
		return false
	}
	c := n.Content[0]
	return c.Kind == yaml.ScalarNode && c.Tag == "!!null"
}

var backtickValue = regexp.MustCompile("`[^`]*`")

// sanitizeYAMLMessage defensively removes scalar values that some YAML
// implementations echo in decode errors. v4 currently reports only tags,
// types and field names, but this keeps credentials safe across upgrades.
func sanitizeYAMLMessage(s string) string {
	return backtickValue.ReplaceAllString(s, "<value>")
}

func pathDiagnostic(file string, directory bool, err error) Diagnostic {
	msg := "file cannot be read"
	if directory {
		msg = "directory cannot be read"
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if directory {
			msg = "directory not found"
		} else {
			msg = "file not found"
		}
	case errors.Is(err, fs.ErrPermission):
		msg = "permission denied"
	}
	return Diagnostic{File: file, Line: 1, Col: 1, RuleID: RuleFile, Message: msg}
}

// classifyYAMLError turns yaml v4 errors into stable rule-coded diagnostics.
// LoadErrors unwraps to its first LoadError, so it MUST be checked first;
// otherwise a file with several decode errors would collapse to one.
func classifyYAMLError(file string, err error, pi *positionIndex) Diagnostics {
	if err == nil {
		return nil
	}
	if loadErrs, ok := errors.AsType[*yaml.LoadErrors](err); ok {
		ds := make(Diagnostics, 0, len(loadErrs.Errors))
		for _, le := range loadErrs.Errors {
			line, col := markPosition(le.Mark)
			msg := sanitizeYAMLMessage(le.Message)
			switch {
			case strings.Contains(msg, "no documents in stream"):
				ds = append(ds, Diagnostic{File: file, Line: 1, Col: 1, RuleID: RuleEmptyDocument, Message: "empty document"})
			case strings.Contains(msg, "expected single document"):
				ds = append(ds, Diagnostic{File: file, Line: 1, Col: 1, RuleID: RuleSyntax,
					Message: "syntax error: expected single document, found multiple"})
			case strings.Contains(msg, "not found in type"):
				key, ok := keyAtError(pi, le.Message, line, col)
				text := "unknown key"
				if ok {
					text = fmt.Sprintf("unknown key %q", key)
				} else if msg != "" {
					text = "unknown key: " + msg
				}
				ds = append(ds, Diagnostic{File: file, Line: line, Col: col, RuleID: RuleUnknownKey, Message: text})
			case strings.Contains(msg, "already defined"):
				ds = append(ds, Diagnostic{File: file, Line: line, Col: col, RuleID: RuleDuplicateKey, Message: msg})
			case strings.Contains(msg, "secret must be a scalar"):
				ds = append(ds, Diagnostic{File: file, Line: line, Col: col, RuleID: RuleCredential,
					Message: "secret must be a scalar"})
			default:
				ds = append(ds, Diagnostic{File: file, Line: line, Col: col, RuleID: RuleTypeMismatch, Message: msg})
			}
		}
		if len(ds) == 0 {
			ds = append(ds, Diagnostic{File: file, Line: 1, Col: 1, RuleID: RuleSyntax,
				Message: "syntax error: " + sanitizeYAMLMessage(err.Error())})
		}
		return ds
	}

	if le, ok := errors.AsType[*yaml.LoadError](err); ok {
		line, col := markPosition(le.Mark)
		if le.Mark.Line == 0 && le.Mark.Column == 0 && le.ContextMark.Line > 0 {
			line, col = markPosition(le.ContextMark)
		}
		msg := "syntax error: " + sanitizeYAMLMessage(le.Message)
		if le.ContextMsg != "" {
			msg += " (" + sanitizeYAMLMessage(le.ContextMsg) + ")"
		}
		return Diagnostics{{File: file, Line: line, Col: col, RuleID: RuleSyntax, Message: msg}}
	}

	return Diagnostics{{
		File: file, Line: 1, Col: 1, RuleID: RuleSyntax,
		Message: "syntax error: " + sanitizeYAMLMessage(err.Error()),
	}}
}

func markPosition(m yaml.Mark) (line, col int) {
	return nonZero(m.Line, m.Column)
}

var unknownFieldPattern = regexp.MustCompile(`field ([^ ]+) not found`)

func keyAtError(pi *positionIndex, message string, line, col int) (string, bool) {
	if pi != nil {
		if key, ok := pi.keyAt(line, col); ok {
			return key, true
		}
	}
	if m := unknownFieldPattern.FindStringSubmatch(message); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

func hasStructuralDiagnostics(ds Diagnostics) bool {
	for _, d := range ds {
		n := ruleNumber(d.RuleID)
		if (n >= 1 && n <= 7) || d.RuleID == RuleCredential {
			return true
		}
	}
	return false
}

func resolveSecrets(b *Bundle, lookup func(string) (string, bool), positions map[string]*positionIndex) Diagnostics {
	var ds Diagnostics
	add := func(file string, p nodePath, format string, args ...any) {
		line, col := 1, 1
		if pi := positions[file]; pi != nil {
			line, col = pi.at(p)
		}
		ds = append(ds, Diagnostic{
			File: file, Line: line, Col: col, RuleID: RuleCredential,
			Message: fmt.Sprintf(format, args...),
		})
	}

	if err := b.Config.Northbound.MQTT.Password.resolve(lookup); err != nil {
		add("config.yaml", fieldPath(fieldPath(fieldPath(nil, "northbound"), "mqtt"), "password"),
			"northbound.mqtt.password: %v", err)
	}
	for i := range b.Links {
		link := &b.Links[i]
		if link.Chirpstack == nil {
			continue
		}
		if err := link.Chirpstack.resolveToken(lookup); err != nil {
			id := link.ID
			if id == "" {
				id = fmt.Sprintf("[%d]", i)
			}
			add("links.yaml", fieldPath(fieldPath(indexPath(fieldPath(nil, "links"), i), "chirpstack"), "api_token_env"),
				"link %s: api_token_env: %v", id, err)
		}
	}
	return ds
}
