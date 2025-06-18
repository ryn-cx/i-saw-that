package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cp "github.com/otiai10/copy"
)

func TestInitialization(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)

	watcher, err := newWatcher(WatcherConfig)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	if watcher.Name != "Test Watcher" {
		t.Errorf("Expected name 'Test Watcher', got '%s'", watcher.Name)
	}
	if watcher.Source != WatcherConfig.Source {
		t.Errorf("Expected source '%s', got '%s'", WatcherConfig.Source, watcher.Source)
	}
	if watcher.Destination != WatcherConfig.Destination {
		t.Errorf("Expected destination '%s', got '%s'", WatcherConfig.Destination, watcher.Destination)
	}
	if !watcher.Enabled {
		t.Errorf("Expected enabled to be true")
	}
	if watcher.WaitTime != 1.0 {
		t.Errorf("Expected wait time 1.0, got %f", watcher.WaitTime)
	}
	if watcher.FolderFormat != "2006-01-02_15-04-05.000000" {
		t.Errorf("Expected folder format '2006-01-02_15-04-05.000000', got '%s'", watcher.FolderFormat)
	}
}

func TestDestinationInSource(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)

	WatcherConfig.Destination = filepath.Join(WatcherConfig.Source, "destination")
	expectedErrMsg := "destination path cannot be inside the source path"
	CheckForWatcherError(t, WatcherConfig, expectedErrMsg)
}

func TestSourceIsDestination(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)

	WatcherConfig.Destination = WatcherConfig.Source
	expectedErrMsg := "destination and source paths cannot be the same"
	CheckForWatcherError(t, WatcherConfig, expectedErrMsg)
}

func TestSourceNotDirectory(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)

	err := os.MkdirAll(filepath.Dir(WatcherConfig.Source), 0755)
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}
	_, err = os.Create(WatcherConfig.Source)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	expectedErrMsg := "source exists but is not a directory"
	CheckForWatcherError(t, WatcherConfig, expectedErrMsg)

}

func TestDestinationNotDirectory(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)

	err := os.MkdirAll(filepath.Dir(WatcherConfig.Destination), 0755)
	if err != nil {
		t.Fatalf("Failed to create parent directory: %v", err)
	}

	file, err := os.Create(WatcherConfig.Destination)
	if err != nil {
		t.Fatalf("Failed to create destination file: %v", err)
	}
	file.Close()

	expectedErrMsg := "destination exists but is not a directory"
	CheckForWatcherError(t, WatcherConfig, expectedErrMsg)

}

func TestInvalidName(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)
	WatcherConfig.Name = ""
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidNameV2, "name cannot be empty")

}

func TestInvalidWaitTime(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)
	WatcherConfig.WaitTime = 0
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidWaitTime, "wait time must be at least 0 seconds")
}

func TestImpreciseFolderFormat(t *testing.T) {
	WatcherConfig := DefaultTempWatcherConfig(t)
	WatcherConfig.FolderFormat = "2006-01-02"
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidFolderFormat, "folder format lacks adequate precision")
}

func TestInvalidFolderFormat(t *testing.T) {
	if os := os.Getenv("OS"); os != "Windows_NT" {
		t.Skip("Skipping Windows-specific test")
	}

	WatcherConfig := DefaultTempWatcherConfig(t)
	// Includes ":" in the format, which is invalid on Windows
	WatcherConfig.FolderFormat = "2006-01-02_15:04:05.000000"
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidFolderFormat, "invalid name")
}

func TestInvalidSourceName(t *testing.T) {
	if os := os.Getenv("OS"); os != "Windows_NT" {
		t.Skip("Skipping Windows-specific test")
	}

	WatcherConfig := DefaultTempWatcherConfig(t)
	WatcherConfig.Source = filepath.Join(WatcherConfig.Source, ":?<>")
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidSource, "invalid name:")
}

func TestInvalidDestinationName(t *testing.T) {
	if os := os.Getenv("OS"); os != "Windows_NT" {
		t.Skip("Skipping Windows-specific test")
	}

	WatcherConfig := DefaultTempWatcherConfig(t)
	WatcherConfig.Destination = filepath.Join(WatcherConfig.Destination, ":?<>")
	CheckForWatcherErrorV2(t, WatcherConfig, &ErrorInvalidDestination, "invalid name:")
}

func TestInitialBackupWithExistingContent(t *testing.T) {
	// This code cannot use getWatcherWithObserver because it starts the watcher with
	// empty directories.
	WatcherConfig := DefaultTempWatcherConfig(t)
	watcher, err := newWatcher(WatcherConfig)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	observer := NewSimplifiedObserver()

	watcher.AddObserver(observer)

	for i := range 5 {
		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("file%d.txt", i), 1024)
		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("subfolder%d/file%d.txt", i, i), 1024)
		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("subfolder%d/subsubfolder%d/file%d.txt", i, i, i), 1024)
	}

	err = watcher.StartWatcher()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	if !observer.WaitUntilCount(1, 5*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[0].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)

	// Make sure an additional backup is not accidentally created after the initial
	// backup.
	time.Sleep(10 * time.Second)
	if observer.CurrentCount != 1 {
		t.Fatalf("Expected 1 backup, got %d", observer.CurrentCount)
	}

}

func TestEmptyInitialBackup(t *testing.T) {
	WatcherConfig, watcher, _ := getWatcherWithObserver(t)
	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[0].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
}

func TestAddingMultipleGroupedFiles(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)

	numberOfLoops := 5
	for i := range numberOfLoops {
		// Offset the number by the number of loops to ensure unique file names
		i2 := i + numberOfLoops

		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("file%d.txt", i), 1024)
		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("file%d.txt", i2), 1024)

		// Wait for the new files to be backed up
		if !observer.WaitUntilCount(i+1, 10*time.Second) {
			t.Fatalf("Timeout waiting for backup completion")
		}

		backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[i+1].Path)

		CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
	}
}
func TestAddingFilesSlowly(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)

	for i := range 5 {
		CreateDummyFile(t, WatcherConfig.Source, fmt.Sprintf("file%d.txt", i), 1024)
		time.Sleep(500 * time.Millisecond)
	}

	if !observer.WaitUntilCount(1, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[1].Path)

	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
}

func TestAddFileDuringBackups(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)
	testPath := filepath.Dir(WatcherConfig.Source)
	tempFolderPath := filepath.Join(testPath, "temp")

	// Create a large file in a temporary folder. This is not in the source folder
	// because it is used later on to make sure the first backup matches the source at
	// the time that the backup started.
	CreateDummyFile(t, tempFolderPath, "file1.txt", 1024*1024*1024)

	// Copy the large file to the source folder
	if err := cp.Copy(tempFolderPath, WatcherConfig.Source, cp.Options{PreserveTimes: true}); err != nil {
		t.Fatalf("Failed to copy source file to temp file: %v", err)
	}

	// Wait for the backup to start
	time.Sleep((time.Duration(watcher.WaitTime)*1000 + 100) * time.Millisecond)

	// Create another file to can a second backup to trigger immediately after the first
	// one completes.
	CreateDummyFile(t, WatcherConfig.Source, "file2.txt", 1024)

	// Make sure the first backup is still running after the file was copied.
	if !observer.WaitUntilCount(0, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	// Wait for the first backup to complete
	if !observer.WaitUntilCount(1, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	// Check that the first backup has just a single file
	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[1].Path)
	CompareSourceAndDestination(t, tempFolderPath, backupPath)

	// Wait for the second backup to complete
	if !observer.WaitUntilCount(2, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath = filepath.Join(WatcherConfig.Destination, watcher.Metadata[2].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
}

func TestAddingFilesInNewSubfolder(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)

	CreateDummyFile(t, WatcherConfig.Source, "subfolder/file1.txt", 1024)
	if !observer.WaitUntilCount(1, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[1].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
}

func TestAddingFilesInExistingSubfolder(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)

	// Create a subfolder in the source directory
	CreateDummyFile(t, WatcherConfig.Source, "subfolder/file1.txt", 1024)

	// Wait for completion
	if !observer.WaitUntilCount(1, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	CreateDummyFile(t, WatcherConfig.Source, "subfolder/file2.txt", 1024)

	// Wait for completion
	if !observer.WaitUntilCount(2, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[2].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)

}
func TestAddingEmptyFolder(t *testing.T) {
	WatcherConfig, watcher, observer := getWatcherWithObserver(t)

	// Create a subfolder in the source directory
	subFolder := filepath.Join(WatcherConfig.Source, "subfolder")
	err := os.MkdirAll(subFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create subfolder: %v", err)
	}

	// Wait for completion
	if !observer.WaitUntilCount(1, 10*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[1].Path)
	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
}

// TODO:
// Test replacing the entire source directory with a new one to see what happpens to the
// recursive watcher .
// Test removing the entire source directory to see what happpens to the recursive
// watcher .
// Test renaming the source directory to see what happpens to the recursive watcher.
// Test starting an existing watcher after it has been started
// Test stopping an existing watcher
// Test stopping an existing watcher after it has been stopped
// Test restarting an existing watcher after it has been stopped
