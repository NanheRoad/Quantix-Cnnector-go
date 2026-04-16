# Windows 编译说明（无命令行窗口 + 高清图标）

## 前置要求

- 已安装 Go
- 已安装 `go-winres`
- 图标文件存在：`build/windows/quantix.ico`
  - 建议该 ico 内包含多尺寸（至少含 `256x256`）以保证高清显示

## 编译命令（PowerShell）

```powershell
go-winres simply --arch amd64 --in winres/winres.json --manifest
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/quantix-server.exe cmd/server/main.go
```

## Makefile 快捷命令

```powershell
make build-windows-gui
```

