package main

import (
	"context"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"quantix-connector-go/internal/desktop"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	desk "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type desktopApp struct {
	fyneApp    fyne.App
	logBuffer  *desktop.LogBuffer
	runner     *desktop.BackendRunner
	cfg        desktop.AppConfig
	logWindow  fyne.Window
	apiKeyWind fyne.Window
	logData    binding.String
}

func main() {
	logBuffer := desktop.NewLogBuffer(6000)
	log.SetFlags(0)
	log.SetOutput(io.MultiWriter(os.Stdout, logBuffer))

	cfg, err := desktop.LoadConfig()
	if err != nil {
		log.Printf("load config warning: %v", err)
		cfg = desktop.DefaultConfig()
	}

	runner := desktop.NewBackendRunner(cfg.APIKey)
	if err := runner.Start(); err != nil {
		log.Printf("backend start failed: %v", err)
	}

	desktop.HideDockIcon()
	a := app.NewWithID("com.quantix.connector")
	a.Settings().SetTheme(theme.LightTheme())
	d := &desktopApp{
		fyneApp:   a,
		logBuffer: logBuffer,
		runner:    runner,
		cfg:       cfg,
		logData:   binding.NewString(),
	}
	_ = d.logData.Set(logBuffer.AllText())
	d.setupTray()

	a.Run()
}

func (d *desktopApp) setupTray() {
	desktopCap, ok := d.fyneApp.(desk.App)
	if !ok {
		log.Printf("system tray is not supported on this platform")
		return
	}
	openFrontend := fyne.NewMenuItem("打开前端", func() {
		if err := openSystemBrowser(d.frontendURL()); err != nil {
			log.Printf("open frontend failed: %v", err)
		}
	})
	openLogs := fyne.NewMenuItem("打开日志", func() {
		d.showLogWindow()
	})
	setAPIKey := fyne.NewMenuItem("设置 API Key", func() {
		d.showAPIKeyWindow()
	})
	menu := fyne.NewMenu("Quantix", openFrontend, openLogs, setAPIKey)
	desktopCap.SetSystemTrayMenu(menu)
	d.fyneApp.Lifecycle().SetOnStopped(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		if err := d.runner.Stop(ctx); err != nil {
			log.Printf("backend stop failed: %v", err)
		}
	})
}

func (d *desktopApp) showLogWindow() {
	if d.logWindow == nil {
		w := d.fyneApp.NewWindow("Quantix 日志")
		w.Resize(fyne.NewSize(860, 560))
		entry := widget.NewMultiLineEntry()
		entry.Wrapping = fyne.TextWrapOff
		entry.TextStyle = fyne.TextStyle{Monospace: true}
		entry.Bind(d.logData)
		clearBtn := widget.NewButton("清空视图", func() {
			_ = d.logData.Set("暂无日志")
		})
		copyBtn := widget.NewButton("复制全部", func() {
			if d.logWindow != nil {
				d.logWindow.Clipboard().SetContent(entry.Text)
			}
		})
		top := container.NewHBox(widget.NewLabel("完整日志（毫秒时间戳）"), clearBtn, copyBtn)
		w.SetContent(container.NewBorder(top, nil, nil, nil, entry))
		w.SetOnClosed(func() {
			d.logWindow = nil
		})
		d.logWindow = w
		go func() {
			ticker := time.NewTicker(600 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				if d.logWindow == nil {
					return
				}
				text := d.logBuffer.AllText()
				_ = d.logData.Set(text)
			}
		}()
	}
	d.logWindow.Show()
	d.logWindow.RequestFocus()
}

func (d *desktopApp) showAPIKeyWindow() {
	if d.apiKeyWind == nil {
		w := d.fyneApp.NewWindow("设置 API Key")
		w.Resize(fyne.NewSize(480, 200))
		entry := widget.NewPasswordEntry()
		entry.SetText(d.cfg.APIKey)
		entry.PlaceHolder = "请输入 API Key"
		hint := widget.NewLabel("保存后将重启本地服务（127.0.0.1:8000）")
		saveBtn := widget.NewButton("保存", func() {
			next := strings.TrimSpace(entry.Text)
			if next == "" {
				dialog.ShowError(errInvalid("API Key 不能为空"), w)
				return
			}
			d.cfg.APIKey = next
			if err := desktop.SaveConfig(d.cfg); err != nil {
				dialog.ShowError(err, w)
				return
			}
			if err := d.runner.Restart(next); err != nil {
				dialog.ShowError(err, w)
				return
			}
			dialog.ShowInformation("成功", "API Key 已保存并生效", w)
		})
		openBtn := widget.NewButton("打开前端", func() {
			if err := openSystemBrowser(d.frontendURL()); err != nil {
				dialog.ShowError(err, w)
			}
		})
		w.SetContent(container.NewVBox(
			widget.NewLabel("当前 API Key"),
			entry,
			hint,
			container.NewHBox(saveBtn, openBtn),
		))
		w.SetOnClosed(func() {
			d.apiKeyWind = nil
		})
		d.apiKeyWind = w
	}
	d.apiKeyWind.Show()
	d.apiKeyWind.RequestFocus()
}

func openSystemBrowser(rawURL string) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}

func (d *desktopApp) frontendURL() string {
	base := d.runner.Address() + "/"
	key := strings.TrimSpace(d.runner.APIKey())
	if key == "" {
		return base
	}
	return base + "?api_key=" + url.QueryEscape(key)
}

type invalidErr struct{ msg string }

func (e invalidErr) Error() string { return e.msg }

func errInvalid(msg string) error { return invalidErr{msg: msg} }
