package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

	latestFileActivity time.Time
	mu                 sync.Mutex
	saveMetadataLock   sync.Mutex
	fsnotifyWatcher    *fsnotify.Watcher
	customObservers    []BackupCompleteObserver
	stopChan           chan struct{}
	backupRequestChan  chan struct{}
}

var ErrorInvalidNameV2 = fmt.Errorf("error validating name")
var ErrorInvalidWaitTime = fmt.Errorf("error validating wait time")
var ErrorInvalidSource = fmt.Errorf("error validating source")
var ErrorInvalidDestination = fmt.Errorf("error validating destination")
var ErrorInvalidFolderFormat = fmt.Errorf("error validating folder format")

func NewWatcher(name, source, destination string, waitTime float64, folderFormat string, enabled bool) (*Watcher, error) {
	var errs error
	// Validate name
	if name == "" {
		err := fmt.Errorf("%w: name cannot be empty", ErrorInvalidNameV2)
		errs = errors.Join(errs, err)
	}

	// Validate wait time
	if waitTime <= 0 {
		err := fmt.Errorf("%w: wait time must be at least 0 seconds", ErrorInvalidWaitTime)
		errs = errors.Join(errs, err)
	}

	if err := validateSourceAndDestination(source, destination); err != nil {
		// These errors already include the "error validating"" prefix because it
		// validates multiple values at once.
		errs = errors.Join(errs, err)
	}

	if err := validateFolderFormat(waitTime, folderFormat); err != nil {
		err := fmt.Errorf("%w: %s", ErrorInvalidFolderFormat, err)
		errs = errors.Join(errs, err)
	}

	w := &Watcher{
		Name:               name,
		Source:             source,
		Destination:        destination,
		Enabled:            enabled,
		WaitTime:           waitTime,
		FolderFormat:       folderFormat,
		Metadata:           []Backup{},
		latestFileActivity: time.Now(),
		stopChan:           make(chan struct{}),
		backupRequestChan:  make(chan struct{}, 1),
	}

	// Loading metadata relies on metadataJSONPath so it is easier to load the metadata
	// after the struct is created.
	if err := w.loadMetadata(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("error loading metadata: %w", err))
	}

	return w, errs
}

// Validate the folder format.
// Make sure that file names cannot overlap.
// Make sure the format is supported by the filesystem.
func validateFolderFormat(waitTime float64, folderFormat string) error {
	var errs error

	// Attempt to create two different times exactly one waitTime apart and make sure
	// that the names are different to avoid potential collisions
	seconds := int64(waitTime)
	nanoseconds := int64((waitTime - float64(seconds)) * 1e9)
	format1 := time.Unix(0, 0).Format(folderFormat)
	format2 := time.Unix(seconds, nanoseconds).Format(folderFormat)
	if format1 == format2 {
		err := fmt.Errorf("folder format lacks adequate precision for wait time")
		errs = errors.Join(errs, err)
	}

	return errors.Join(errs, validateDir(folderFormat, ErrorInvalidFolderFormat))
}

// Validate a path is a directory.
// The path must be supported by the filesystem.
// The path must not be a file.
// If the path does not exist, it will be created.
func validateDir(path string, invalidNameError error) error {
	var errs error
	var pathErr *os.PathError

	info, err := os.Stat(path)

	// errors.As(err, &pathErr) returns true if the file does not exist, so it must be
	// checked after checking if the file exists
	// os.IsNotExist(err) returns false if the name is invalid so it can be checked
	// before checking if the name is invalid
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			errs = errors.Join(errs, err)
		}
	} else if errors.As(err, &pathErr) {
		errs = errors.Join(errs, fmt.Errorf("%w: invalid name: %w", invalidNameError, err))
	} else if err == nil && !info.IsDir() {
		errs = errors.Join(errs, fmt.Errorf("%w: %s exists but is not a directory", invalidNameError, path))
	} else if err != nil {
		errs = errors.Join(errs, fmt.Errorf("%w: %w", invalidNameError, err))
	}

	return errs
}

// Validate source and destination directories.
// The values rely on one another so both must be validated at the same time.
// The paths must be supported by the filesystem.
// The paths must not be a file.
// If the paths do not exist, they will be created.
// The paths must not be the same.
// The destination must not be inside the source.
func validateSourceAndDestination(source string, destination string) error {
	var errs error

	// Generic directory validation
	errs = errors.Join(errs, validateDir(source, ErrorInvalidSource))
	errs = errors.Join(errs, validateDir(destination, ErrorInvalidDestination))

	// Get absolute paths so validation cannot be bypassed by using relative paths
	absSource, err := filepath.Abs(source)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("%w: error getting absolute path: %w", ErrorInvalidSource, err))
	}
	absDest, err := filepath.Abs(destination)
	if err != nil {
		err = fmt.Errorf("%w: error getting absolute path: %w", ErrorInvalidDestination, err)
		errs = errors.Join(errs, err)
	}

	// Make sure source and destination are different
	if absSource == absDest {
		err = fmt.Errorf("%w: source and destination paths cannot be the same", ErrorInvalidSource)
		errs = errors.Join(errs, err)
		err = fmt.Errorf("%w: destination and source paths cannot be the same", ErrorInvalidDestination)
		errs = errors.Join(errs, err)
	}

	// Make sure destination is not inside of source
	relPath, err := filepath.Rel(absSource, absDest)
	if err != nil {
		err := fmt.Errorf("%w: error checking relative path from source to destination: %w", ErrorInvalidDestination, err)
		errs = errors.Join(errs, err)
	}
	if !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, "..") && relPath != "." {
		err := fmt.Errorf("%w: destination path cannot be inside the source path", ErrorInvalidDestination)
		errs = errors.Join(errs, err)
	}

	return errs
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
	// TODO: What happens if metadata is a folder?
	w.saveMetadataLock.Lock()
	defer w.saveMetadataLock.Unlock()

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

// TODO: Go through everything below this
// TODO: Make more tests before mucking around in this code
func (w *Watcher) StartWatcher() error {
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

	// Create an initial backup if no backups are present.
	if len(w.Metadata) == 0 {
		w.backupRequestChan <- struct{}{}
	}

	fmt.Printf("%s: Starting watcher\n", w.Name)
	return nil
}

// StopWatcher stops watching the source directory
func (w *Watcher) StopWatcher() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.fsnotifyWatcher == nil && len(w.stopChan) == 0 {
		return nil // Already stopped
	}

	close(w.stopChan)
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
	// TODO: Look into Go packaging for how this can be compiled by users.
	w.fsnotifyWatcher.Add(filepath.Join(w.Source, "..."))

	for {
		select {
		case event, ok := <-w.fsnotifyWatcher.Events:
			// TODO What does it mean if ok is false?
			if !ok {
				return nil
			}
			// event.Op is a bitmask depending on the type of event, for now just
			// run the backup for any file event, but this is here in case some
			// events should not trigger a backup.
			if event.Op != 0 {
				w.handleFileEvent(event)
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
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(time.Duration(w.WaitTime * float64(time.Second)))
			timerChan = timer.C

		// The timer has expired, which means the changes have settled and it's time to
		// create a backup.
		case <-timerChan:
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

// handleFileEvent handles a file system event
func (w *Watcher) handleFileEvent(event fsnotify.Event) {
	// Add more information to Printf from the event
	log.Printf("%s: File event detected: %s, Op: %s", w.Name, event.Name, event.Op)
	w.mu.Lock()
	w.latestFileActivity = time.Now()
	w.mu.Unlock()
	w.backupRequestChan <- struct{}{}
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
