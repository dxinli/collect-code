package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const manifestSchemaVersion = 1

// reservedSections are manifest keys owned by the runtime or the manifest
// format itself. Modules may not replace them via SetSection.
var reservedSections = map[string]struct{}{
	"schema_version":  {},
	"generated_at":    {},
	"pid":             {},
	"http":            {},
	"endpoints":       {},
	"control_actions": {},
	"modules":         {},
}

// writeManifestAtomic writes data to path through a same-directory temporary
// file, fsync and rename. The directory must already exist; it is not created.
func writeManifestAtomic(path string, doc map[string]any) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create manifest temp file: %w", err)
	}
	tmp := f.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync manifest: %w", err)
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	remove = false
	return nil
}
