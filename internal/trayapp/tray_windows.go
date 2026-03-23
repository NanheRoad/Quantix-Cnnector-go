//go:build windows

package trayapp

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Options struct {
	FrontendURL  string
	LogPath      string
	GetAPIKey    func() string
	UpdateAPIKey func(string) error
	OnQuit       func()
}

var (
	procMu sync.Mutex
	proc   *exec.Cmd
)

func Run(opts Options) error {
	script := buildPowerShellScript(opts)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	procMu.Lock()
	proc = cmd
	procMu.Unlock()

	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			fmt.Printf("[tray] %s\n", line)
		}
	}()

	s := bufio.NewScanner(stdout)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "APIKEY:") {
			if opts.UpdateAPIKey != nil {
				_ = opts.UpdateAPIKey(strings.TrimPrefix(line, "APIKEY:"))
			}
			continue
		}
		if line == "QUIT" {
			if opts.OnQuit != nil {
				opts.OnQuit()
			}
		}
	}

	err = cmd.Wait()
	procMu.Lock()
	if proc == cmd {
		proc = nil
	}
	procMu.Unlock()
	if err != nil {
		return err
	}
	return nil
}

func RequestQuit() {
	procMu.Lock()
	p := proc
	procMu.Unlock()
	if p != nil && p.Process != nil {
		_ = p.Process.Kill()
	}
}

func buildPowerShellScript(opts Options) string {
	baseURL := psQuote(strings.TrimSpace(opts.FrontendURL))
	logPath := psQuote(strings.TrimSpace(opts.LogPath))
	currentKey := ""
	if opts.GetAPIKey != nil {
		currentKey = opts.GetAPIKey()
	}
	key := psQuote(currentKey)
	cfgPath := psQuote("./quantix.local.json")

	return fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName Microsoft.VisualBasic

$frontendBase = %s
$apiKey = %s

function New-QuantixIcon {
  $bmp = New-Object System.Drawing.Bitmap 64,64
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
  $g.Clear([System.Drawing.Color]::Transparent)

  $rect = New-Object System.Drawing.RectangleF(3,3,58,58)
  $r = 13.0
  $bgPath = New-Object System.Drawing.Drawing2D.GraphicsPath
  $bgPath.AddArc($rect.X, $rect.Y, $r*2, $r*2, 180, 90)
  $bgPath.AddArc($rect.Right-$r*2, $rect.Y, $r*2, $r*2, 270, 90)
  $bgPath.AddArc($rect.Right-$r*2, $rect.Bottom-$r*2, $r*2, $r*2, 0, 90)
  $bgPath.AddArc($rect.X, $rect.Bottom-$r*2, $r*2, $r*2, 90, 90)
  $bgPath.CloseFigure()

  $bgBrush = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(15,91,216))
  $g.FillPath($bgBrush, $bgPath)

  $white = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::White)
  $blue = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(10,63,158))
  $penQ = New-Object System.Drawing.Pen ([System.Drawing.Color]::White, 7)
  $penQ.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
  $penQ.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
  $penQ.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round

  $g.DrawEllipse($penQ, 10, 12, 30, 30)
  $g.DrawLine($penQ, 31, 33, 43, 45)
  $g.FillRectangle($white, 41, 14, 14, 20)
  $g.FillRectangle($blue, 44, 18, 2, 5)
  $g.FillRectangle($blue, 48, 18, 2, 5)
  $g.FillRectangle($blue, 52, 18, 2, 5)
  $g.FillRectangle($white, 13, 47, 28, 4)
  $g.FillRectangle($white, 18, 53, 18, 4)

  $hIcon = $bmp.GetHicon()
  return [System.Drawing.Icon]::FromHandle($hIcon)
}

$notify = New-Object System.Windows.Forms.NotifyIcon
$notify.Icon = New-QuantixIcon
$notify.Text = 'Quantix Connector'
$notify.Visible = $true

$menu = New-Object System.Windows.Forms.ContextMenuStrip
$itemShow = $menu.Items.Add('显示前端')
$itemSet = $menu.Items.Add('设置 API Key')
$itemLog = $menu.Items.Add('查看日志')
$menu.Items.Add('-') | Out-Null
$itemQuit = $menu.Items.Add('退出')
$notify.ContextMenuStrip = $menu

$itemShow.add_Click({
  $target = $frontendBase
  if ($apiKey -ne '') {
    if ($target.Contains('?')) {
      $target = $target + '&api_key=' + [System.Uri]::EscapeDataString($apiKey)
    } else {
      $target = $target + '?api_key=' + [System.Uri]::EscapeDataString($apiKey)
    }
  }
  Start-Process $target
})

$itemSet.add_Click({
  $input = [Microsoft.VisualBasic.Interaction]::InputBox('输入新的 API Key（留空=关闭鉴权）', '设置 API Key', %s)
  if ($input -ne $null) {
    $apiKey = $input
    $obj = @{ api_key = $input } | ConvertTo-Json
    Set-Content -Path %s -Value $obj -Encoding UTF8
    Write-Output ('APIKEY:' + $input)
  }
})

$itemLog.add_Click({
  if (Test-Path %s) {
    Start-Process %s
  } else {
    [System.Windows.Forms.MessageBox]::Show('日志文件不存在', 'Quantix') | Out-Null
  }
})

$itemQuit.add_Click({
  Write-Output 'QUIT'
  $notify.Visible = $false
  [System.Windows.Forms.Application]::Exit()
})

[System.Windows.Forms.Application]::Run()
`, baseURL, key, key, cfgPath, logPath, logPath)
}

func psQuote(v string) string {
	s := strings.ReplaceAll(v, "'", "''")
	return "'" + s + "'"
}
