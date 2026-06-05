package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Default watcher settings used when running from the command line.
const (
	cliDefaultWaitTime     = 1.0
	cliDefaultFolderFormat = "2006-01-02_15-04-05.000000"
)

// runCLI starts a single watcher headlessly (no GUI) for the given source and
// destination directories and blocks until the process is interrupted.
func runCLI(source, destination string) error {
	watcher, err := NewWatcher(
		"cli",
		source,
		destination,
		cliDefaultWaitTime,
		cliDefaultFolderFormat,
	)
	if err != nil {
		return fmt.Errorf("error creating watcher: %w", err)
	}

	if err := watcher.StartWatcher(); err != nil {
		return fmt.Errorf("error starting watcher: %w", err)
	}

	log.Printf("Watching %s -> %s. Press Ctrl+C to stop.", source, destination)

	// Block until an interrupt or termination signal is received.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := watcher.StopWatcher(); err != nil {
		return fmt.Errorf("error stopping watcher: %w", err)
	}
	return nil
}
