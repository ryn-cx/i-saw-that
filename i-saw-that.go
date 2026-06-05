package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend
var assets embed.FS

func main() {
	// If a source and destination are passed as arguments, run in headless CLI
	// mode. With no arguments, launch the GUI.
	if len(os.Args) > 1 {
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: %s <source> <destination>\n", os.Args[0])
			os.Exit(1)
		}
		if err := runCLI(os.Args[1], os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "I Saw That",
		Width:  800,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
