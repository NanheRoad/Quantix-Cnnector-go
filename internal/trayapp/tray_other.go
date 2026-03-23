//go:build !windows

package trayapp

import "fmt"

type Options struct {
	FrontendURL  string
	LogPath      string
	GetAPIKey    func() string
	UpdateAPIKey func(string) error
	OnQuit       func()
}

func Run(opts Options) error {
	_ = opts
	return fmt.Errorf("system tray is only enabled on windows build")
}

func RequestQuit() {}
