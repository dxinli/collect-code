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

	var parseDiags Diagnostics
	if err := loadFile(filepath.Join(dir, "config.yaml"), b.Config); err != nil {
		parseDiags = append(parseDiags, flatten("config.yaml", err)...)
	}
	var links LinksFile
	if err := loadFile(filepath.Join(dir, "links.yaml"), &links); err != nil {
		parseDiags = append(parseDiags, flatten("links.yaml", err)...)
	}
	var devices DevicesFile
	if err := loadFile(filepath.Join(dir, "devices.yaml"), &devices); err != nil {
		parseDiags = append(parseDiags, flatten("devices.yaml", err)...)
	}
	profileDiags, profiles, sources := loadProfiles(filepath.Join(dir, "profiles"))
	parseDiags = append(parseDiags, profileDiags...)
	if len(parseDiags) > 0 {
		return nil, parseDiags
	}

	b.Links = links.Links
	b.Devices = devices.Devices
	b.Profiles = profiles
	b.ProfileSources = sources

	if ds := requiredDiagnostics(b); len(ds) > 0 {
		return nil, ds
	}
	if ds := resolveSecrets(b, o.lookup); len(ds) > 0 {
		return nil, ds
	}
	return b, nil
}

func loadProfiles(dir string) (Diagnostics, map[string]*Profile, map[string]string) {
	profiles := map[string]*Profile{}
	sources := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return Diagnostics{{File: "profiles", Message: err.Error()}}, profiles, sources
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

	var ds Diagnostics
	for _, name := range names {
		var p Profile
		if err := loadFile(filepath.Join(dir, name), &p); err != nil {
			ds = append(ds, flatten(name, err)...)
			continue
		}
		if p.Profile == "" {
			ds = append(ds, Diagnostic{File: name, Message: "profile: required"})
			continue
		}
		if prev, ok := sources[p.Profile]; ok {
			ds = append(ds, Diagnostic{
				File:    name,
				Message: fmt.Sprintf("duplicate profile %q (also defined in %s)", p.Profile, prev),
			})
			continue
		}
		profiles[p.Profile] = &p
		sources[p.Profile] = name
	}
	return ds, profiles, sources
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

func flatten(file string, err error) Diagnostics {
	if ds, ok := err.(Diagnostics); ok {
		return ds
	}
	if d, ok := err.(Diagnostic); ok {
		if d.File == "" {
			d.File = file
		}
		return Diagnostics{d}
	}
	return Diagnostics{{File: file, Message: err.Error()}}
}

func requiredDiagnostics(b *Bundle) Diagnostics {
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
	return ds
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
				Message: fmt.Sprintf("link %s: %v", link.ID, err),
			})
		}
	}
	return ds
}
