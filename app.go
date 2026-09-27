package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

type App struct {
	app *application.App
}

func NewApp(app *application.App) *App {
	return &App{app: app}
}

// SaveFileDialog 打开文件保存对话框
func (a *App) SaveFileDialog(title, defaultFilename string) (string, error) {
	return a.app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:    title,
		Filename: defaultFilename,
	}).PromptForSingleSelection()
}

// OpenFileDialog 打开文件选择对话框
func (a *App) OpenFileDialog(title string, filters []application.FileFilter) (string, error) {
	dialog := a.app.Dialog.OpenFile().SetTitle(title)
	for _, f := range filters {
		dialog.AddFilter(f.DisplayName, f.Pattern)
	}
	return dialog.PromptForSingleSelection()
}
