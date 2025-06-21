package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	cp "github.com/otiai10/copy"
)

// Interface used for tests and potential GUI in the future
type BackupCompleteObserver interface {
	OnBackupCompletion(watcher *Watcher)
}

type Backup struct {
	Name       string  `json:"name,omitempty"`
	Timestamp  float64 `json:"timestamp"`
	Path       string  `json:"path"`
	Compressed bool    `json:"compressed,omitempty"`
}

type Watcher struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Destination  string   `json:"destination"`
	Enabled      bool     `json:"enabled"`
	WaitTime     float64  `json:"wait_time"`
	FolderFormat string   `json:"folder_format"`
	Metadata     []Backup `json:"metadata"`

	mu                sync.Mutex
	fsnotifyWatcher   *fsnotify.Watcher
	customObservers   []BackupCompleteObserver
	stopChan          chan struct{}
	backupRequestChan chan struct{}
}

func NewWatcher(name, source, destination string, waitTime float64, folderFormat string, enabled bool) (*Watcher, error) {
	var errs error
	validateName(name, &errs)
	validateWaitTime(waitTime, &errs)
	validateFolderFormat(waitTime, folderFormat, &errs)
	validateSourceAndDestination(source, destination, &errs)

	w := &Watcher{
		Name:              name,
		Source:            source,
		Destination:       destination,
		Enabled:           enabled,
		WaitTime:          waitTime,
		FolderFormat:      folderFormat,
		Metadata:          []Backup{},
		stopChan:          make(chan struct{}),
		backupRequestChan: make(chan struct{}, 1),
	}

	// Loading metadata relies on metadataJSONPath so it is easier to load the metadata
	// after the struct is created.
	if err := w.loadMetadata(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("error loading metadata: %w", err))
	}

	return w, errs
}

func (w *Watcher) metadataJSONPath() string {
	return filepath.Join(w.Destination, "metadata.json")
}

func (w *Watcher) loadMetadata() error {
	// TODO: What happens if metadata is a folder?
	data, err := os.ReadFile(w.metadataJSONPath())
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("error reading metadata file: %w", err)
	}

	var metadata []Backup
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("error parsing metadata JSON: %w", err)
	}

	w.Metadata = metadata
	return nil
}

func (w *Watcher) saveMetadata() error {
	data, err := json.MarshalIndent(w.Metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling metadata: %w", err)
	}

	metadataPath := w.metadataJSONPath()

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("error writing metadata file: %w", err)
	}

	return nil
}

func (w *Watcher) StartWatcher() error {
	log.Printf("%s: Starting watcher\n", w.Name)
	// Easiest to lock the thread for the whole function since StartWatcher isn't a
	// function that will be called frequently.
	w.mu.Lock()
	defer w.mu.Unlock()

	// This is an error to support having a GUI in the future
	if !w.Enabled {
		return errors.New("watcher is disabled")
	}

	if w.fsnotifyWatcher != nil {
		return errors.New("watcher is already running")
	}

	go w.startFSNotifyWatcher()
	go w.backupLoop()

	log.Printf("%s: Watcher Started\n", w.Name)

	// Create an initial backup if no backups are present.
	err := w.createBackupIfBackupIsOutdated()
	if err != nil {
		return fmt.Errorf("error checking if backup is up to date: %w", err)
	}
	return nil
}

// StopWatcher stops watching the source directory
func (w *Watcher) StopWatcher() error {
	// Easiest to lock the thread for the whole function since StopWatcher isn't a
	// function that will be called frequently.
	log.Printf("%s: Stopping watcher\n", w.Name)
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.fsnotifyWatcher == nil {
		return nil // Already stopped
	}

	err := w.fsnotifyWatcher.Close()
	w.fsnotifyWatcher = nil

	return err
}

func (w *Watcher) startFSNotifyWatcher() error {
	var err error
	w.fsnotifyWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating file watcher: %w", err)
	}

	// The current version of fsnotify unofficially supports recursive watching by
	// appending ... to the path and modifying a single line in the fsnotify code.
	// TODO: Decide how this program should be built and distributed.
	w.fsnotifyWatcher.Add(filepath.Join(w.Source, "..."))

	for {
		select {
		case event, ok := <-w.fsnotifyWatcher.Events:
			// TODO: Under what conditions does ok become false?
			if !ok {
				return nil
			}
			// event.Op is a bitmask depending on the type of event, for now just
			// run the backup for any file event, but this is here in case some
			// events should not trigger a backup.
			if event.Op != 0 {
				log.Printf("%s: File event detected: %s, Op: %s", w.Name, event.Name, event.Op)
				w.backupRequestChan <- struct{}{}
			}
		case err, ok := <-w.fsnotifyWatcher.Errors:
			if !ok {
				return err
			}
			log.Printf("Error watching files: %v", err)
		case <-w.stopChan:
			return nil
		}
	}
}

// Thread responsible for creating backups.
func (w *Watcher) backupLoop() {
	var timer *time.Timer
	var timerChan <-chan time.Time

	for {
		select {
		case <-w.stopChan:
			return

		// An file was changed, start a timer to wait for all file changes to settle
		// before creating a backup.
		case <-w.backupRequestChan:
			log.Printf("File change detected, starting timer for %f seconds", w.WaitTime)
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(time.Duration(w.WaitTime * float64(time.Second)))
			timerChan = timer.C

		// The timer has expired, which means the changes have settled and it's time to
		// create a backup.
		case <-timerChan:
			log.Printf("%s: Timer expired, creating backup", w.Name)
			w.createBackup()

			// Reset timer
			timer = nil
			timerChan = nil
		}
	}
}

func (w *Watcher) createBackup() {
	// Snapshot the values for this backup operation to avoid them being incorrect if
	// the watcher is modified while the backup is being created.
	w.mu.Lock()
	sourceSnapshot := w.Source
	destinationSnapshot := w.Destination
	folderFormatSnapshot := w.FolderFormat
	w.mu.Unlock()

	timestamp := time.Now()
	timestampFolder := timestamp.Format(folderFormatSnapshot)
	destinationPath := filepath.Join(destinationSnapshot, timestampFolder)

	// Check if destination path already exists
	if _, err := os.Stat(destinationPath); err == nil {
		log.Printf("Destination path %s already exists", destinationPath)
		return
	}

	log.Printf("Creating backup at %s", destinationPath)
	// Try copying files 100 times waiting 0.1 second between attempt to bypass locked files
	// TODO: A more reasonable appproach to handling locked files
	for range 100 {
		if err := cp.Copy(sourceSnapshot, destinationPath, cp.Options{PreserveTimes: true}); err != nil {
			log.Printf("Error copying source to destination: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}

	// Add the backup to metadata
	backup := Backup{
		Timestamp: float64(timestamp.Unix()) + float64(timestamp.Nanosecond())/1e9,
		Path:      timestampFolder,
	}

	w.mu.Lock()
	w.Metadata = append(w.Metadata, backup)
	w.mu.Unlock()

	// This is only ever called by the single backup thread and the file is only
	// accessed during initialization (before threads are started) and when writing it
	// here so no locking is needed.
	if err := w.saveMetadata(); err != nil {
		log.Printf("Error saving metadata: %v", err)
	}
	log.Printf("Backup created successfully at %s", destinationPath)

	w.notifyObservers()
}

func (w *Watcher) AddObserver(observer BackupCompleteObserver) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, obs := range w.customObservers {
		if obs == observer {
			return // Already added
		}
	}
	w.customObservers = append(w.customObservers, observer)
}

func (w *Watcher) RemoveObserver(observer BackupCompleteObserver) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, obs := range w.customObservers {
		if obs == observer {
			w.customObservers = append(w.customObservers[:i], w.customObservers[i+1:]...)
			return
		}
	}
}

// Notify observers that a backup has been completed
func (w *Watcher) notifyObservers() {
	w.mu.Lock()
	defer w.mu.Unlock()

	observers := make([]BackupCompleteObserver, len(w.customObservers))
	copy(observers, w.customObservers)

	for _, observer := range observers {
		observer.OnBackupCompletion(w)
	}
}

func (w *Watcher) createBackupIfBackupIsOutdated() error {
	// If no backups have been made it has to be outdated
	if len(w.Metadata) == 0 {
		log.Printf("No backups found, creating initial backup")
		w.backupRequestChan <- struct{}{}
		return nil
	}

	latestBackupPath := filepath.Join(w.Destination, w.Metadata[len(w.Metadata)-1].Path)

	foldersMatch, err := doFoldersMatch(w.Source, latestBackupPath)
	if err != nil {
		return fmt.Errorf("error comparing source and latest backup: %w", err)
	}

	if !foldersMatch {
		log.Printf("Source and latest backup do not match, creating new backup")
		w.backupRequestChan <- struct{}{}
	}

	return nil
}

func doFoldersMatch(source, destination string) (bool, error) {
	sourceEntries, err := os.ReadDir(source)
	if err != nil {
		return false, fmt.Errorf("error reading source directory: %w", err)
	}
	destEntries, err := os.ReadDir(destination)
	if err != nil {
		return false, fmt.Errorf("error reading destination directory: %w", err)
	}

	if len(sourceEntries) != len(destEntries) {
		return false, nil
	}

	for i := range sourceEntries {
		sourceEntry := sourceEntries[i]
		destinationEntry := destEntries[i]

		if sourceEntry.Name() != destinationEntry.Name() {
			return false, nil
		}

		sourceString := filepath.Join(source, sourceEntry.Name())
		destinationString := filepath.Join(destination, destinationEntry.Name())

		if sourceEntry.IsDir() && destinationEntry.IsDir() {
			subfolderMatch, err := doFoldersMatch(sourceString, destinationString)
			if err != nil {
				return false, fmt.Errorf("error comparing directories: %w", err)
			}
			if !subfolderMatch {
				return false, nil
			}
		} else if !sourceEntry.IsDir() && !destinationEntry.IsDir() {
			fileMatch, err := doFilesMatch(sourceString, destinationString)
			if err != nil {
				return false, fmt.Errorf("error comparing files: %w", err)
			}

			if !fileMatch {
				return false, nil
			}

		} else {
			return false, nil
		}
	}
	return true, nil
}

func doFilesMatch(source, destination string) (bool, error) {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false, fmt.Errorf("error stating source file: %v", err)
	}
	destInfo, err := os.Stat(destination)
	if err != nil {
		return false, fmt.Errorf("error stating destination file: %v", err)
	}

	sourceContent, err := os.ReadFile(source)
	if err != nil {
		return false, fmt.Errorf("error reading source file: %v", err)
	}

	destContent, err := os.ReadFile(destination)
	if err != nil {
		return false, fmt.Errorf("error reading destination file: %v", err)
	}

	if string(sourceContent) != string(destContent) {
		return false, nil
	}

	if !sourceInfo.ModTime().Equal(destInfo.ModTime()) {
		return false, nil
	}
	return true, nil
}
