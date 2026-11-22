package bbolt

import (
	stderrors "errors"
	"path/filepath"
	"testing"
	"time"

	bolterrors "go.etcd.io/bbolt/errors"
)

func TestBboltSecondOpenTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := open(path, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Errorf("first Close: %v", err)
		}
	}()

	if _, err := open(path, 100*time.Millisecond); !stderrors.Is(err, bolterrors.ErrTimeout) {
		t.Fatalf("second open error = %v, want bolt.ErrTimeout", err)
	}
}
