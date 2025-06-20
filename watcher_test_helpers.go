package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// tempWatcherConfig holds configuration for creating a watcher
type tempWatcherConfig struct {
	Name         string
	Source       string
	Destination  string
	TempPath     string
	WaitTime     float64
	FolderFormat string
	Enabled      bool
}

// DefaultTempWatcherConfig returns a configuration with sensible defaults
func DefaultTempWatcherConfig(t *testing.T) tempWatcherConfig {
	// Create temporary directory
	tempPath, err := os.MkdirTemp("", "watcher-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	log.Printf("Temporary directory created: %s", tempPath)

	t.Cleanup(func() {
		os.RemoveAll(tempPath)
	})

	return tempWatcherConfig{
		Name:         "Test Watcher",
		TempPath:     tempPath,
		Source:       filepath.Join(tempPath, "source"),
		Destination:  filepath.Join(tempPath, "destination"),
		WaitTime:     1.0,
		FolderFormat: "2006-01-02_15-04-05.000000",
		Enabled:      true,
	}
}

// Create a default watcher for testing
func newWatcher(config tempWatcherConfig) (*Watcher, error) {
	return NewWatcher(
		config.Name,
		config.Source,
		config.Destination,
		config.WaitTime,
		config.FolderFormat,
		config.Enabled,
	)
}

func CheckForWatcherError(t *testing.T, WatcherConfig tempWatcherConfig, expectedErrMsg string) {
	_, err := newWatcher(WatcherConfig)
	if err == nil || !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain:\n%s\nBut got:\n%v", expectedErrMsg, err)
	}
}

func CheckForWatcherErrorV2(t *testing.T, WatcherConfig tempWatcherConfig, expectedErrPtr any, expectedErrMsg string) {
	_, err := newWatcher(WatcherConfig)

	if err == nil {
		t.Errorf("Expected error of type:\n%T\nBut got no error", expectedErrPtr)
		return
	}
	if !errors.As(err, expectedErrPtr) {
		t.Errorf("Expected error type to be:\n%T\nBut got:\n%v", expectedErrPtr, err)
		return
	}
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain:\n%s\nBut got:\n%v", expectedErrMsg, err)
	}
}

func CheckForWatcherErrorV3(t *testing.T, WatcherConfig tempWatcherConfig, expectedErrPtr error, expectedErrMsg string) {
	_, err := newWatcher(WatcherConfig)

	if err == nil {
		t.Errorf("Expected error of type:\n%T\nBut got no error", expectedErrPtr)
		return
	}
	if !errors.Is(err, expectedErrPtr) {
		t.Errorf("Expected error type to be:\n%T\nBut got:\n%v", expectedErrPtr, err)
		return
	}
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error message to contain:\n%s\nBut got:\n%v", expectedErrMsg, err)
	}
}

func CompareSourceAndDestination(t *testing.T, source, destination string) {
	sourceEntries, err := os.ReadDir(source)
	if err != nil {
		t.Fatalf("Error reading source directory: %v", err)
	}
	destEntries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatalf("Error reading destination directory: %v", err)
	}

	if len(sourceEntries) != len(destEntries) {
		t.Fatalf("Directory entry counts don't match. Source: %d, Destination: %d", len(sourceEntries), len(destEntries))
	}

	for i := range sourceEntries {
		sourceEntry := sourceEntries[i]
		destinationEntry := destEntries[i]

		if sourceEntry.Name() != destinationEntry.Name() {
			t.Fatalf("Directory entries don't match at index %d. Source: %s, Destination: %s", i, sourceEntry.Name(), destinationEntry.Name())
		}

		sourceString := filepath.Join(source, sourceEntry.Name())
		destinationString := filepath.Join(destination, destinationEntry.Name())

		if sourceEntry.IsDir() && destinationEntry.IsDir() {
			CompareSourceAndDestination(t, sourceString, destinationString)
		} else if !sourceEntry.IsDir() && !destinationEntry.IsDir() {
			err := CompareFiles(sourceString, destinationString)
			if err != nil {
				t.Fatalf("Error comparing files: %v", err)
			}
		} else {
			t.Fatalf("Type mismatch for entry: %s", sourceEntry.Name())
		}
	}
}

func CompareFiles(source, destination string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("error stating source file: %v", err)
	}
	destInfo, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("error stating destination file: %v", err)
	}

	sourceContent, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("error reading source file: %v", err)
	}

	destContent, err := os.ReadFile(destination)
	if err != nil {
		return fmt.Errorf("error reading destination file: %v", err)
	}

	if string(sourceContent) != string(destContent) {
		// Truncate the content to avoid large diffs in error messages
		if len(sourceContent) > 100 {
			sourceContent = sourceContent[:100]
		}
		if len(destContent) > 100 {
			destContent = destContent[:100]
		}
		// Include file names in this error message
		return fmt.Errorf("file contents don't match.\nSource: %s: %s\nDestination: %s: %s", source, string(sourceContent), destination, string(destContent))
	}

	if !sourceInfo.ModTime().Equal(destInfo.ModTime()) {
		return fmt.Errorf("file modification times don't match.\nSource: %v\nDestination: %v", sourceInfo.ModTime(), destInfo.ModTime())
	}
	return nil
}

func NewSimplifiedObserver() *SimplifiedObserver {
	o := &SimplifiedObserver{}
	o.cond = sync.NewCond(&o.mu)
	return o
}

type SimplifiedObserver struct {
	CurrentCount int
	mu           sync.Mutex
	cond         *sync.Cond
}

func (o *SimplifiedObserver) OnBackupCompletion(watcher *Watcher) {
	o.incrementCounter()
	o.cond.Signal()
}

func (o *SimplifiedObserver) getCurrentCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.CurrentCount
}
func (o *SimplifiedObserver) incrementCounter() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.CurrentCount++
}

// WaitUntilCount waits for the observer's CurrentCount to reach a specific value.
func (o *SimplifiedObserver) WaitUntilCount(targetCount int, timeout time.Duration) bool {
	if o.getCurrentCount() == targetCount {
		return true
	}

	outOfTime := false
	timer := time.AfterFunc(timeout, func() {
		outOfTime = true
		o.cond.Signal()
	})
	defer timer.Stop()

	o.mu.Lock()
	for o.CurrentCount < targetCount && !outOfTime {
		o.cond.Wait()
	}
	o.mu.Unlock()

	return o.getCurrentCount() >= targetCount
}

func getWatcherWithObserver(t *testing.T) (tempWatcherConfig, *Watcher, *SimplifiedObserver) {
	WatcherConfig := DefaultTempWatcherConfig(t)
	watcher, err := newWatcher(WatcherConfig)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	observer := NewSimplifiedObserver()

	watcher.AddObserver(observer)

	// Start the watcher
	err = watcher.StartWatcher()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	t.Cleanup(func() {
		if err := watcher.StopWatcher(); err != nil {
			t.Fatalf("Failed to stop watcher: %v", err)
		}
	})

	// Wait for completion
	if !observer.WaitUntilCount(1, 5*time.Second) {
		t.Fatalf("Timeout waiting for backup completion")
	}

	backupPath := filepath.Join(WatcherConfig.Destination, watcher.Metadata[0].Path)

	CompareSourceAndDestination(t, WatcherConfig.Source, backupPath)
	observer.CurrentCount = 0 // Reset observer count for the tests

	return WatcherConfig, watcher, observer
}

func CreateDummyFile(t *testing.T, directoryPath string, filePath string, fileSize int) {
	fullPath := filepath.Join(directoryPath, filePath)
	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, createRandomFileContent(fileSize), 0644); err != nil {
		t.Fatalf("Failed to write to source file 2: %v", err)
	}
}

func createRandomFileContent(size int) []byte {
	// For test purposes, a simple pseudo-random generator is sufficient.
	// Using crypto/rand is unnecessary overhead here.
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return b
}
