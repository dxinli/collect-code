package config

import (
	"errors"
	"fmt"
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

// Load reads and strictly parses the four configuration layers.
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

	// Parse errors stop at the first offending file (01b adds full
	// diagnostics); required-field errors are aggregated below instead.
	if err := loadFile(filepath.Join(dir, "config.yaml"), b.Config); err != nil {
		return nil, err
	}
	var links LinksFile
	if err := loadFile(filepath.Join(dir, "links.yaml"), &links); err != nil {
		return nil, err
	}
	var devices DevicesFile
	if err := loadFile(filepath.Join(dir, "devices.yaml"), &devices); err != nil {
		return nil, err
	}
	loaded, sources, err := loadProfiles(filepath.Join(dir, "profiles"))
	if err != nil {
		return nil, err
	}

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

	if ds := requiredDiagnostics(b, loaded); len(ds) > 0 {
		return nil, ds
	}
	if ds := resolveSecrets(b, o.lookup); len(ds) > 0 {
		return nil, ds
	}
	return b, nil
}

type loadedProfile struct {
	file    string
	profile *Profile
}

func loadProfiles(dir string) ([]loadedProfile, map[string]string, error) {
	sources := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, Diagnostic{File: "profiles", Message: err.Error()}
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
	for _, name := range names {
		var p Profile
		if err := loadFile(filepath.Join(dir, name), &p); err != nil {
			return nil, nil, err
		}
		if p.Profile != "" {
			if prev, ok := sources[p.Profile]; ok {
				return nil, nil, Diagnostic{
					File:    name,
					Message: fmt.Sprintf("duplicate profile %q (also defined in %s)", p.Profile, prev),
				}
			}
			sources[p.Profile] = name
		}
		loaded = append(loaded, loadedProfile{file: name, profile: &p})
	}
	return loaded, sources, nil
}

func loadFile(path string, out any) error {
	name := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return Diagnostic{File: name, Message: err.Error()}
	}

	// First pass only checks for an empty document; strict decoding is done
	// against the original bytes so reported line/column numbers stay true.
	var node yaml.Node
	if err := yaml.Load(data, &node); err != nil {
		return yamlError(name, err)
	}
	if emptyNode(&node) {
		return Diagnostic{File: name, Message: "empty document"}
	}

	if err := yaml.Load(data, out, yaml.WithKnownFields(), yaml.WithUniqueKeys()); err != nil {
		return yamlError(name, err)
	}
	return nil
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

func yamlError(file string, err error) error {
	if loadErrs, ok := errors.AsType[*yaml.LoadErrors](err); ok {
		ds := make(Diagnostics, 0, len(loadErrs.Errors))
		for _, le := range loadErrs.Errors {
			msg := sanitizeYAMLMessage(le.Message)
			if strings.Contains(msg, "no documents in stream") {
				msg = "empty document"
			}
			ds = append(ds, Diagnostic{
				File:    file,
				Line:    le.Mark.Line,
				Col:     le.Mark.Column,
				Message: msg,
			})
		}
		return ds
	}
	msg := sanitizeYAMLMessage(err.Error())
	if strings.Contains(msg, "no documents in stream") {
		msg = "empty document"
	}
	return Diagnostic{File: file, Message: msg}
}

func requiredDiagnostics(b *Bundle, loaded []loadedProfile) Diagnostics {
	var ds Diagnostics
	add := func(file, path string) {
		ds = append(ds, Diagnostic{File: file, Message: path + ": required"})
	}

	if b.Config.Node.ID == "" {
		add("config.yaml", "node.id")
	}
	if b.Config.Node.ProductKey == "" {
		add("config.yaml", "node.product_key")
	}
	if b.Config.Northbound.MQTT.Broker == "" {
		add("config.yaml", "northbound.mqtt.broker")
	}

	for i, l := range b.Links {
		p := fmt.Sprintf("links[%d]", i)
		if l.ID == "" {
			add("links.yaml", p+".id")
		}
		if l.Driver == "" {
			add("links.yaml", p+".driver")
		}
		if l.Serial == nil && l.TCP == nil && l.Chirpstack == nil {
			add("links.yaml", p+": one of serial/tcp/chirpstack")
		}
	}

	for i, d := range b.Devices {
		p := fmt.Sprintf("devices[%d]", i)
		if d.SN == "" {
			add("devices.yaml", p+".sn")
		}
		if d.DeviceType == "" {
			add("devices.yaml", p+".device_type")
		}
		if d.Profile == "" {
			add("devices.yaml", p+".profile")
		}
		if d.Link == "" {
			add("devices.yaml", p+".link")
		}
		if d.Address == nil {
			add("devices.yaml", p+".address")
		}
	}

	for _, lp := range loaded {
		if lp.profile.Profile == "" {
			add(lp.file, "profile")
		}
	}
	return ds
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

func resolveSecrets(b *Bundle, lookup func(string) (string, bool)) Diagnostics {
	var ds Diagnostics

	if err := b.Config.Northbound.MQTT.Password.resolve(lookup); err != nil {
		ds = append(ds, Diagnostic{
			File:    "config.yaml",
			Message: "northbound.mqtt.password: " + err.Error(),
		})
	}
	for i := range b.Links {
		link := &b.Links[i]
		if link.Chirpstack == nil {
			continue
		}
		if err := link.Chirpstack.resolveToken(lookup); err != nil {
			ds = append(ds, Diagnostic{
				File:    "links.yaml",
				Message: fmt.Sprintf("link %s: api_token_env: %v", link.ID, err),
			})
		}
	}
	return ds
}
