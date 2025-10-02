package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	// List of folder pairs from the config file.
	config []*WatcherConfig
	// Map of active watchers by their ID.
	watchers map[string]*Watcher
	// Path to the config file that saves the folders being watched.
	configPath string
}

type WatcherConfig struct {
	ID           string  `json:"id"`
	Source       string  `json:"source"`
	Destination  string  `json:"destination"`
	Enabled      bool    `json:"enabled"`
	WaitTime     float64 `json:"wait_time"`
	FolderFormat string  `json:"folder_format"`
}

func NewApp() *App {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Error getting config dir: %v", err)
		configDir = "."
	}

	appConfigDir := filepath.Join(configDir, "i-saw-that")
	os.MkdirAll(appConfigDir, 0755)

	return &App{
		watchers:   make(map[string]*Watcher),
		configPath: filepath.Join(appConfigDir, "config.json"),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.loadConfig(); err != nil {
		log.Printf("Error loading config: %v", err)
	}
}

// GetFolderPairs returns all folder pairs
func (a *App) GetFolderPairs() []*WatcherConfig {
	return a.config
}

func (a *App) SelectFolder() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// ToggleFolderPair enables or disables a folder pair
func (a *App) ToggleFolderPair(id string, enabled bool) error {
	for i, pair := range a.config {
		if pair.ID == id {
			if enabled {
				// Start watcher
				watcher, err := NewWatcher(
					pair.ID,
					pair.Source,
					pair.Destination,
					pair.WaitTime,
					pair.FolderFormat,
				)
				if err != nil {
					return fmt.Errorf("error creating watcher: %w", err)
				}

				if err := watcher.StartWatcher(); err != nil {
					return fmt.Errorf("error starting watcher: %w", err)
				}

				a.watchers[id] = watcher
				log.Printf("Enabled folder pair: %s -> %s", pair.Source, pair.Destination)
			} else {
				// Stop watcher
				if watcher, exists := a.watchers[id]; exists {
					if err := watcher.StopWatcher(); err != nil {
						log.Printf("Error stopping watcher: %v", err)
					}
					delete(a.watchers, id)
				}
				log.Printf("Disabled folder pair: %s -> %s", pair.Source, pair.Destination)
			}

			a.config[i].Enabled = enabled
			a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("folder pair not found")
}

// AddFolderPair adds a new folder pair
func (a *App) AddFolderPair(source, destination string, waitTime float64, folderFormat string) error {
	id := fmt.Sprintf("watcher-%d", len(a.config))

	// Use defaults if not provided
	if waitTime <= 0 {
		waitTime = 1.0
	}
	if folderFormat == "" {
		folderFormat = "2006-01-02_15-04-05.000000"
	}

	watcher, err := NewWatcher(
		id,
		source,
		destination,
		waitTime,
		folderFormat,
	)
	if err != nil {
		return fmt.Errorf("error creating watcher: %w", err)
	}

	if err := watcher.StartWatcher(); err != nil {
		return fmt.Errorf("error starting watcher: %w", err)
	}

	pair := &WatcherConfig{
		ID:           id,
		Source:       source,
		Destination:  destination,
		Enabled:      true,
		WaitTime:     waitTime,
		FolderFormat: folderFormat,
	}

	a.config = append(a.config, pair)
	a.watchers[id] = watcher

	log.Printf("Added folder pair: %s -> %s\n", source, destination)
	a.saveConfig()
	return nil
}

// UpdateFolderPair updates an existing folder pair
func (a *App) UpdateFolderPair(id, source, destination string, waitTime float64, folderFormat string) error {
	for i, pair := range a.config {
		if pair.ID == id {
			// Use existing values if not provided
			if waitTime <= 0 {
				waitTime = pair.WaitTime
			}
			if folderFormat == "" {
				folderFormat = pair.FolderFormat
			}

			// Stop old watcher if enabled
			if watcher, exists := a.watchers[id]; exists {
				if err := watcher.StopWatcher(); err != nil {
					log.Printf("Error stopping watcher: %v", err)
				}
				delete(a.watchers, id)
			}

			// Create new watcher if enabled
			if pair.Enabled {
				watcher, err := NewWatcher(
					id,
					source,
					destination,
					waitTime,
					folderFormat,
				)
				if err != nil {
					return fmt.Errorf("error creating watcher: %w", err)
				}

				if err := watcher.StartWatcher(); err != nil {
					return fmt.Errorf("error starting watcher: %w", err)
				}

				a.watchers[id] = watcher
			}

			// Update pair
			a.config[i].Source = source
			a.config[i].Destination = destination
			a.config[i].WaitTime = waitTime
			a.config[i].FolderFormat = folderFormat

			log.Printf("Updated folder pair: %s -> %s\n", source, destination)
			a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("folder pair not found")
}

// RemoveFolderPair removes a folder pair by ID
func (a *App) RemoveFolderPair(id string) error {
	for i, pair := range a.config {
		if pair.ID == id {
			// Stop the watcher
			if watcher, exists := a.watchers[id]; exists {
				if err := watcher.StopWatcher(); err != nil {
					log.Printf("Error stopping watcher: %v", err)
				}
				delete(a.watchers, id)
			}

			// Remove from slice
			a.config = append(a.config[:i], a.config[i+1:]...)
			a.saveConfig()
			return nil
		}
	}
	return fmt.Errorf("folder pair not found")
}

// loadConfig loads folder pairs from config file
func (a *App) loadConfig() error {
	data, err := os.ReadFile(a.configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	var pairs []*WatcherConfig
	if err := json.Unmarshal(data, &pairs); err != nil {
		return fmt.Errorf("error parsing config: %w", err)
	}

	// Start watchers for each pair
	for _, pair := range pairs {
		// Set defaults if missing
		if pair.WaitTime <= 0 {
			pair.WaitTime = 1.0
		}
		if pair.FolderFormat == "" {
			pair.FolderFormat = "2006-01-02_15-04-05.000000"
		}

		// Only start watcher if enabled
		if pair.Enabled {
			watcher, err := NewWatcher(
				pair.ID,
				pair.Source,
				pair.Destination,
				pair.WaitTime,
				pair.FolderFormat,
			)
			if err != nil {
				log.Printf("Error creating watcher for %s: %v", pair.ID, err)
				a.config = append(a.config, pair)
				continue
			}

			if err := watcher.StartWatcher(); err != nil {
				log.Printf("Error starting watcher for %s: %v", pair.ID, err)
				a.config = append(a.config, pair)
				continue
			}

			a.watchers[pair.ID] = watcher
		}

		a.config = append(a.config, pair)
		log.Printf("Loaded folder pair: %s -> %s", pair.Source, pair.Destination)
	}

	return nil
}

// saveConfig saves folder pairs to config file
func (a *App) saveConfig() error {
	data, err := json.MarshalIndent(a.config, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	if err := os.WriteFile(a.configPath, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	log.Printf("Config saved to %s", a.configPath)
	return nil
}
