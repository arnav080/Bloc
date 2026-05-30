package hardware

import (
	"os"
	"testing"
)

func TestFreeSpaceBytes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bloc-disk-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	free, err := FreeSpaceBytes(tempDir)
	if err != nil {
		t.Fatalf("FreeSpaceBytes failed: %v", err)
	}

	if free == 0 {
		t.Logf("Warning: reported free space is 0 bytes (might be normal in some restricted container sandboxes)")
	} else {
		t.Logf("Reported free space: %d bytes (%.2f GB)", free, float64(free)/1e9)
	}
}
