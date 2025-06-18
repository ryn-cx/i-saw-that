package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	cp "github.com/otiai10/copy"
)

func makeTestFiles(t *testing.T) (string, string, string, string, time.Time) {
	tmpDir, err := os.MkdirTemp("", "copyfs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	source := filepath.Join(tmpDir, "src")
	destination := filepath.Join(tmpDir, "dst")
	testFilePath := filepath.Join(source, "testfile.txt")
	copiedFilePath := filepath.Join(destination, "testfile.txt")

	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create a test file in the source directory
	if err := os.WriteFile(testFilePath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set a specific modification time
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(testFilePath, modTime, modTime); err != nil {
		t.Fatalf("Failed to change file times: %v", err)
	}
	return tmpDir, source, destination, copiedFilePath, modTime
}

// Can't use os.CopyFS because it does not preserve mtime
func TestOsCopyFSIgnoresMTime(t *testing.T) {
	tmpDir, source, destination, copiedFilePath, modTime := makeTestFiles(t)

	// Copy the source directory to the destination
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Check if the file exists in the destination and has the same modification time
	info, err := os.Stat(copiedFilePath)
	if err != nil {
		t.Fatalf("Failed to stat copied file: %v", err)
	}

	if info.ModTime().Equal(modTime) {
		t.Errorf("os.CopyFS preserves mtime now?")
	}
	os.RemoveAll(tmpDir)
}

// otiai10/copy supports preserving mtime
func TestOtai10CopyCopiesMTime(t *testing.T) {
	// Create temporary directories for source and destination
	tmpDir, source, destination, copiedFilePath, modTime := makeTestFiles(t)
	// Copy the source directory to the destination
	if err := cp.Copy(source, destination, cp.Options{PreserveTimes: true}); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Check if the file exists in the destination and has the same modification time
	info, err := os.Stat(copiedFilePath)
	if err != nil {
		t.Fatalf("Failed to stat copied file: %v", err)
	}

	if !info.ModTime().Equal(modTime) {
		t.Errorf("Expected mod time %v, got %v", modTime, info.ModTime())
	}
	os.RemoveAll(tmpDir)
}
