package presets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresetsManager(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_presets.json")

	mgr := NewManager(filePath)
	p := Preset{
		Host:    "example.com",
		Port:    "8000",
		SID:     "1",
		User:    "source",
		Pass:    "secret",
		Bitrate: "192",
	}

	if err := mgr.Set("Station 1", p); err != nil {
		t.Fatalf("failed to save preset: %v", err)
	}

	loaded, ok := mgr.Get("Station 1")
	if !ok || loaded.Host != "example.com" || loaded.Bitrate != "192" {
		t.Fatalf("loaded preset mismatch: %+v", loaded)
	}

	mgr2 := NewManager(filePath)
	loaded2, ok := mgr2.Get("Station 1")
	if !ok || loaded2.Pass != "secret" {
		t.Fatalf("mgr2 reload mismatch: %+v", loaded2)
	}

	if err := mgr.Delete("Station 1"); err != nil {
		t.Fatalf("failed to delete preset: %v", err)
	}

	if _, ok := mgr.Get("Station 1"); ok {
		t.Fatalf("expected preset to be deleted")
	}

	_ = os.Remove(filePath)
}
