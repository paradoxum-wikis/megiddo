package winutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shell32 = windows.NewLazySystemDLL("shell32.dll")

const (
	SW_HIDE        = 0
	SW_SHOWDEFAULT = 10
)

func ProcessIsElevated() bool {
	r, _, _ := shell32.NewProc("IsUserAnAdmin").Call()
	return r != 0
}

func RelaunchElevatedSelf(skip int, extras []string, showCmd int32) error {
	proc := shell32.NewProc("ShellExecuteW")
	ex, err := os.Executable()
	if err != nil {
		return err
	}
	ex = filepath.Clean(ex)

	exW, err := windows.UTF16PtrFromString(ex)
	if err != nil {
		return err
	}

	var tail []string
	if skip < len(os.Args) {
		tail = append(tail, os.Args[skip:]...)
	}
	tail = append(tail, extras...)
	argLine := composeArgLine(tail)
	params := (*uint16)(nil)
	if argLine != "" {
		ptr, err := windows.UTF16PtrFromString(argLine)
		if err != nil {
			return err
		}
		params = ptr
	}

	runasVerb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}

	r, _, last := syscall.SyscallN(
		proc.Addr(),
		0,
		uintptr(unsafe.Pointer(runasVerb)),
		uintptr(unsafe.Pointer(exW)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(showCmd),
	)
	if int32(r) <= 32 {
		return last
	}
	return nil
}

func composeArgLine(args []string) string {
	out := ""
	for _, a := range args {
		if out != "" {
			out += " "
		}
		if strings.ContainsAny(a, " \t\"") || a == "" {
			out += `"` + strings.ReplaceAll(strings.ReplaceAll(a, `\`, `\\`), `"`, `\"`) + `"`
			continue
		}
		out += a
	}
	return out
}
