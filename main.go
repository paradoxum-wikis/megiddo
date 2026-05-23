package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatalf("megiddo: bootstrap: %v", err)
	}

	if err := wails.Run(&options.App{
		Title:     "Megiddo",
		Width:     1024,
		Height:    680,
		MinWidth:  640,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: frontendAssets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 22, B: 34, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
		},
	}); err != nil {
		log.Fatalf("megiddo: wails: %v", err)
	}
}
