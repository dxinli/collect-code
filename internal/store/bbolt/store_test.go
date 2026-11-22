package bbolt_test

import (
	"os"
	"path/filepath"
	"testing"

	storebbolt "github.com/dxinli/collect-code/internal/store/bbolt"
	bolt "go.etcd.io/bbolt"
)

// rawOpen opens the database with the upstream library so a test can build or
// inspect a specific on-disk state.
func rawOpen(t *testing.T, path string) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("raw bolt.Open(%q): %v", path, err)
	}
	return db
}

// rawSchema opens path, applies fn inside one update transaction, and closes.
func rawSchema(t *testing.T, path string, fn func(tx *bolt.Tx) error) {
	t.Helper()
	db := rawOpen(t, path)
	if err := db.Update(fn); err != nil {
		t.Fatalf("raw schema update: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}
}

func TestBboltCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "state.db")
	st, err := storebbolt.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state.db was not created: %v", err)
	}
}

func TestBboltFileAndDirPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(dir, "state.db")
	st, err := storebbolt.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o750 {
		t.Fatalf("dir mode = %o, want 750", perm)
	}
}

func TestBboltRejectsEmptyPath(t *testing.T) {
	if st, err := storebbolt.Open(""); err == nil {
		_ = st.Close()
		t.Fatal("Open(\"\") succeeded, want error")
	}
}

func TestBboltRejectsIncompatibleSchema(t *testing.T) {
	version2 := []byte{0, 0, 0, 0, 0, 0, 0, 2}
	cases := []struct {
		name string
		fn   func(tx *bolt.Tx) error
	}{
		{
			name: "unsupported version",
			fn: func(tx *bolt.Tx) error {
				meta, err := tx.CreateBucket([]byte("meta"))
				if err != nil {
					return err
				}
				return meta.Put([]byte("schema_version"), version2)
			},
		},
		{
			name: "missing version",
			fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucket([]byte("meta"))
				return err
			},
		},
		{
			name: "short version",
			fn: func(tx *bolt.Tx) error {
				meta, err := tx.CreateBucket([]byte("meta"))
				if err != nil {
					return err
				}
				return meta.Put([]byte("schema_version"), []byte{1, 2, 3, 4})
			},
		},
		{
			name: "business bucket without meta",
			fn: func(tx *bolt.Tx) error {
				_, err := tx.CreateBucket([]byte("records"))
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			rawSchema(t, path, tc.fn)
			st, err := storebbolt.Open(path)
			if err == nil {
				_ = st.Close()
				t.Fatal("Open succeeded on an incompatible schema, want error")
			}
		})
	}
}

func TestBboltOpenFailureLeavesNoHalfSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	rawSchema(t, path, func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket([]byte("meta"))
		if err != nil {
			return err
		}
		return meta.Put([]byte("schema_version"), []byte{0, 0, 0, 0, 0, 0, 0, 2})
	})

	if st, err := storebbolt.Open(path); err == nil {
		_ = st.Close()
		t.Fatal("Open succeeded on schema version 2, want error")
	}

	db := rawOpen(t, path)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close raw db: %v", err)
		}
	}()
	if err := db.View(func(tx *bolt.Tx) error {
		for _, name := range []string{"records", "last_seq", "devices", "snapshots"} {
			if tx.Bucket([]byte(name)) != nil {
				t.Errorf("bucket %q was created by a failed Open", name)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("view raw db: %v", err)
	}
}
