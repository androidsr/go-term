package main

import (
	"embed"
	"fmt"

	"go-term/controllers"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

func main() {
	app := application.New(application.Options{
		Name:        "go-term",
		Description: "智能SSH终端管理器 - 服务器管理、终端操作、文件管理、批量脚本执行",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	// 注册服务（依赖注入 app 引用，供对话框/事件使用）
	app.RegisterService(application.NewService(NewApp(app)))
	app.RegisterService(application.NewService(controllers.NewSSHController(app)))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "那个谁SSH终端",
		Width:            1100,
		Height:           750,
		BackgroundColour: application.NewRGB(20, 20, 20),
		Windows: application.WindowsWindow{
			Theme: application.Dark,
		},
	})

	// 系统托盘：SSH 终端工具常驻托盘，提供快速显示/退出入口
	systray := app.SystemTray.New()
	systray.SetIcon(appIcon)
	systray.SetLabel("那个谁SSH终端")

	trayMenu := app.NewMenu()
	trayMenu.Add("显示主窗口").OnClick(func(ctx *application.Context) {
		app.Window.Current().Show()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("退出").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	systray.SetMenu(trayMenu)

	err := app.Run()
	if err != nil {
		fmt.Println("Error:", err.Error())
	}
}
