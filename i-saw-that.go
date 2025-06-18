// main.go - Example usage of the Watcher

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Simplified command-line arguments until proper interface is implemented.
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run main.go <source_path> <destination_path>")
		fmt.Println("Example: go run main.go /path/to/source /path/to/destination")
		os.Exit(1)
	}

	sourcePath := os.Args[1]
	destPath := os.Args[2]

	watcher, err := NewWatcher(
		"Main Watcher",
		sourcePath,
		destPath,
		1.0,
		"2006-01-02_15-04-05.000000",
		true,
	)
	if err != nil {
		log.Fatalf("Error creating watcher: %v", err)
	}

	if err := watcher.StartWatcher(); err != nil {
		log.Fatalf("Error starting watcher: %v", err)
	}

	fmt.Printf("Watcher started.\nWatching: %s\nBackups: %s\n", watcher.Source, watcher.Destination)
	fmt.Printf("Watching: %s\n", watcher.Source)
	fmt.Printf("Backups: %s\n", watcher.Destination)

	// Keep the program running until interrupted
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	// Stopping the watcher is not required because it doesn't do anything different
	// than just killing the process, but having this makes it where cleanupp can be
	// added in the future if needed.
	fmt.Println("\nStopping watcher...")
	if err := watcher.StopWatcher(); err != nil {
		log.Printf("Error stopping watcher: %v", err)
	}
	fmt.Println("Watcher stopped.")
}
