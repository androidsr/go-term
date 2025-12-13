package main

import (
	"context"
	"embed"

	"go-term/controllers"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	sshController := controllers.NewSSHController()

	err := wails.Run(&options.App{
		Title:  "那个谁SSH终端",
		Width:  1100,
		Height: 750,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			sshController.Startup(ctx)
		},
		Bind: []interface{}{
			app,
			sshController,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			BackdropType:                      windows.Mica,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               "",
			WebviewBrowserPath:                "",
			Theme:                             windows.Light,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
