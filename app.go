package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SaveFileDialog 打开文件保存对话框
func (a *App) SaveFileDialog(title, defaultFilename string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultFilename,
	})
}

// OpenFileDialog 打开文件选择对话框
func (a *App) OpenFileDialog(title string, filters []runtime.FileFilter) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: filters,
	})
}
