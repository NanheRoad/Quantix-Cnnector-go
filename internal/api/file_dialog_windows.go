//go:build windows

package api

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
	ofnExplorer      = 0x00080000
)

type openFileNameW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        unsafe.Pointer
	DwReserved        uint32
	FlagsEx           uint32
}

func openLocalFileDialog(title, filter string) (string, error) {
	if strings.TrimSpace(title) == "" {
		title = "选择文件"
	}
	if strings.TrimSpace(filter) == "" {
		filter = "All files (*.*)|*.*"
	}

	filterBuf := toWindowsDialogFilterUTF16(filter)
	filterPtr := &filterBuf[0]
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", fmt.Errorf("invalid title: %w", err)
	}

	fileBuf := make([]uint16, 4096)
	ofn := openFileNameW{
		LStructSize: uint32(unsafe.Sizeof(openFileNameW{})),
		LpstrFilter: filterPtr,
		LpstrFile:   &fileBuf[0],
		NMaxFile:    uint32(len(fileBuf)),
		LpstrTitle:  titlePtr,
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir,
	}

	comdlg32 := windows.NewLazySystemDLL("comdlg32.dll")
	getOpenFileNameW := comdlg32.NewProc("GetOpenFileNameW")
	commDlgExtendedError := comdlg32.NewProc("CommDlgExtendedError")

	ret, _, callErr := getOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		errCode, _, _ := commDlgExtendedError.Call()
		if errCode == 0 {
			return "", fmt.Errorf("no file selected")
		}
		if callErr != windows.ERROR_SUCCESS && callErr != nil {
			return "", fmt.Errorf("open file dialog failed: %v", callErr)
		}
		return "", fmt.Errorf("open file dialog failed: commdlg error %d", errCode)
	}

	path := windows.UTF16ToString(fileBuf)
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no file selected")
	}
	return path, nil
}

func toWindowsDialogFilterUTF16(filter string) []uint16 {
	parts := strings.Split(strings.TrimSpace(filter), "|")
	out := make([]string, 0, len(parts)+1)
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		out = []string{"All files (*.*)", "*.*"}
	}
	if len(out)%2 != 0 {
		out = append(out, "*.*")
	}
	joined := strings.Join(out, "\x00") + "\x00\x00"
	return utf16.Encode([]rune(joined))
}
