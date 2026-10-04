package notification

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStateMissingFileStartsUninitialized(t *testing.T) {
	store, err := OpenState(filepath.Join(t.TempDir(), "state", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if store.Initialized() {
		t.Fatal("new state is initialized")
	}
}

func TestStateBootstrapPersistsWithoutSendingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap("pipeline:1:mr", "pipeline:2:ready"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Initialized() || !reopened.Contains("pipeline:1:mr") || !reopened.Contains("pipeline:2:ready") {
		t.Fatalf("reopened state lost bootstrap keys: %#v", reopened)
	}
}

func TestStateMarkSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mark("deployment:42"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Contains("deployment:42") {
		t.Fatal("marked event was not persisted")
	}
}

func TestOpenStateRejectsCorruptJSONWithoutOverwritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := []byte("{broken")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenState(path); err == nil {
		t.Fatal("OpenState() error = nil")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("corrupt state changed to %q", got)
	}
}

func TestStatePrunesOldestKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 2001)
	for i := range keys {
		keys[i] = fmt.Sprintf("event:%04d", i)
	}
	if err := store.Mark(keys...); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenState(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Contains("event:0000") {
		t.Fatal("oldest key was not pruned")
	}
	if !reopened.Contains("event:2000") {
		t.Fatal("newest key was pruned")
	}
}
