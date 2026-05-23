package dnsflush

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	dnsapi       = windows.NewLazyDLL("dnsapi.dll")
	procDNSFlush = dnsapi.NewProc("DnsFlushResolverCache")
)

func Flush() {
	if err := procDNSFlush.Find(); err != nil {
		flushIPC()
		return
	}
	r, _, _ := procDNSFlush.Call()
	if r == 0 {
		flushIPC()
		return
	}
}

func flushIPC() {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}
