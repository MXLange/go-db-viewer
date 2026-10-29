package main

import (
	"embed"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := NewApp()
	if err != nil {
		println("Não foi possível iniciar o armazenamento de conexões.")
		return
	}
	err = wails.Run(&options.App{
		Title: "Go DB Viewer", Width: 1280, Height: 820, MinWidth: 900, MinHeight: 620,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets}, OnStartup: app.startup, OnBeforeClose: app.beforeClose, OnShutdown: app.shutdown, Bind: []any{app},
	})
	if err != nil {
		println("Não foi possível iniciar a janela do aplicativo.")
	}
}
