package main

import (
	"embed"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed frontend/*
var ui embed.FS

func main() {
	exe, _ := os.Executable()
	app := NewStudio(filepath.Dir(exe))
	assets, _ := fs.Sub(ui, "frontend")
	err := wails.Run(&options.App{Title: "Nawa · نَوى", Width: 1380, Height: 900, MinWidth: 960, MinHeight: 680, Frameless: true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets, Handler: app.MediaHandler()},
		Windows:          &windows.Options{WebviewIsTransparent: true, WindowIsTranslucent: true, DisableFramelessWindowDecorations: true, BackdropType: windows.Acrylic, Theme: windows.Dark, WebviewUserDataPath: filepath.Join(app.root, "data", "webview"), WindowClassName: "MadarMusicStudio"},
		OnStartup:        app.start, OnShutdown: app.stop, Bind: []interface{}{app}, SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "42a7b6bf-b738-428d-a527-27d2ac114360"}})
	if err != nil {
		println(err.Error())
	}
}
