package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := &App{cfg: loadConfig()}
	// Initial size is a placeholder — gapLoop positions the window from the
	// live probe within seconds (physical pixels via SetWindowPos).
	err := wails.Run(&options.App{
		Title:             "win-taskbar-pet-overlay",
		Width:             1536,
		Height:            36,
		Frameless:         true,
		AlwaysOnTop:       true,
		StartHidden:       true,
		DisableResize:     true,
		HideWindowOnClose: true,
		// NOTE: do NOT set BackgroundColour — wails paints it as an OPAQUE
		// class brush, which the transparent webview then reveals instead
		// of the desktop (black/white bar bug).
		Windows: &windows.Options{
			WebviewIsTransparent: true,
		},
		Bind:              []interface{}{app},
		OnStartup:         app.startup,
		OnDomReady:        app.domready,
		OnShutdown:        app.shutdown,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
	})
	if err != nil {
		println("wails fatal:", err.Error())
	}
}
