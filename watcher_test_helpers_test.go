// Tests for the helper test functions

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareIdenticalFiles(t *testing.T) {
	// Create temporary source and destination files
	tmpDir, err := os.MkdirTemp("", "comparefiles-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	firstFile := filepath.Join(tmpDir, "first.txt")
	secondFile := filepath.Join(tmpDir, "second.txt")

	content := "File content"
	if err := os.WriteFile(firstFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write dest file: %v", err)
	}

	// Set the same modification time
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(firstFile, modTime, modTime); err != nil {
		t.Fatalf("Failed to change source file times: %v", err)
	}
	if err := os.Chtimes(secondFile, modTime, modTime); err != nil {
		t.Fatalf("Failed to change dest file times: %v", err)
	}

	// Compare the files
	CompareFiles(firstFile, secondFile)
}

func TestCompareFilesWithDifferentContent(t *testing.T) {
	// Create temporary source and destination files
	tmpDir, err := os.MkdirTemp("", "comparefiles-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	firstFile := filepath.Join(tmpDir, "first.txt")
	secondFile := filepath.Join(tmpDir, "second.txt")

	if err := os.WriteFile(firstFile, []byte("First file"), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte("Second file"), 0644); err != nil {
		t.Fatalf("Failed to write dest file: %v", err)
	}

	// Set the same modification time
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(firstFile, modTime, modTime); err != nil {
		t.Fatalf("Failed to change source file times: %v", err)
	}
	if err := os.Chtimes(secondFile, modTime, modTime); err != nil {
		t.Fatalf("Failed to change dest file times: %v", err)
	}

	err = CompareFiles(firstFile, secondFile)
	if err == nil {
		t.Fatalf("Expected error due to different file contents, but got none")
	}
}

func TestCompareFilesWithDifferentTimestamps(t *testing.T) {
	// Create temporary source and destination files
	tmpDir, err := os.MkdirTemp("", "comparefiles-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	firstFile := filepath.Join(tmpDir, "first.txt")
	secondFile := filepath.Join(tmpDir, "second.txt")

	content := "File content"
	if err := os.WriteFile(firstFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write dest file: %v", err)
	}

	// Set different modification times
	modTime1 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	modTime2 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(firstFile, modTime1, modTime1); err != nil {
		t.Fatalf("Failed to change source file times: %v", err)
	}
	if err := os.Chtimes(secondFile, modTime2, modTime2); err != nil {
		t.Fatalf("Failed to change dest file times: %v", err)
	}

	err = CompareFiles(firstFile, secondFile)
	if err == nil {
		t.Fatalf("Expected error due to different file timestamps, but got none")
	}
}
