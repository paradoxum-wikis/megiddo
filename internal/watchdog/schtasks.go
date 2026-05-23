package watchdog

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"megiddo/internal/megiddo"
)

const lookaheadSecs = 15

func UpsertMegiddoTask() error {
	pidPath := filepath.ToSlash(ownerPIDAbs())
	runAtLocal := time.Now().Add(time.Duration(lookaheadSecs) * time.Second)
	ps := watchdogPowerShell(pidPath)
	boundary := runAtLocal.Format("2006-01-02T15:04:05")

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <TimeTrigger>
      <StartBoundary>%s</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <ExecutionTimeLimit>PT1M</ExecutionTimeLimit>
    <StartWhenAvailable>false</StartWhenAvailable>
    <Hidden>true</Hidden>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>-NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -EncodedCommand %s</Arguments>
    </Exec>
  </Actions>
</Task>`, boundary, encodeUTF16Base64(ps))

	tmp := filepath.Join(os.TempDir(), "megiddo_watchdog_task.xml")
	utf16Payload := encodeUTF16LE([]byte(xml))
	final := append([]byte{0xff, 0xfe}, utf16Payload...)
	if err := os.WriteFile(tmp, final, 0o600); err != nil {
		return err
	}

	cmd := exec.Command(
		"schtasks.exe", "/Create",
		"/TN", megiddo.WatchdogTaskName,
		"/XML", tmp,
		"/RU", "SYSTEM",
		"/F",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func DeleteMegiddoTask() {
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", megiddo.WatchdogTaskName, "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
	_ = os.Remove(filepath.Join(os.TempDir(), "megiddo_watchdog_task.xml"))
}

func ProxyOwnerAbs() string {
	return ownerPIDAbs()
}

func ownerPIDAbs() string {
	return filepath.Join(os.TempDir(), megiddo.ProxyOwnerPIDFileName)
}

func watchdogPowerShell(pidFile string) string {
	marker := "'" + megiddo.HostMarker + "'"
	ps := fmt.Sprintf(`$pp=%q
$alive=$false
if(Test-Path $pp){
	try{
		$fpid=[int](Get-Content $pp -Raw)
		if(Get-Process -Id $fpid -ErrorAction SilentlyContinue){$alive=$true}
	}catch{}
}
if(-not $alive){
	$f=[System.IO.Path]::Combine($env:SystemRoot,'System32','drivers','etc','hosts')
	if(Test-Path $f){
		$lines=[System.IO.File]::ReadLines($f)
		$clean=@()
		foreach($ln in $lines){
			if($ln.IndexOf(%s,[System.Globalization.StringComparison]::OrdinalIgnoreCase) -lt 0){$clean+=$ln}
		}
		[System.IO.File]::WriteAllLines($f,[string[]]$clean)
	}
	Start-Process 'ipconfig.exe' '/flushdns' -NoNewWindow -Wait
}`, pidFile, marker)
	return strings.TrimSpace(ps)
}

func encodeUTF16Base64(script string) string {
	u := utf16.Encode([]rune(script))
	raw := make([]byte, len(u)*2)
	for i, v := range u {
		raw[2*i] = byte(v)
		raw[2*i+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func encodeUTF16LE(utf8 []byte) []byte {
	u := utf16.Encode([]rune(string(utf8)))
	raw := make([]byte, len(u)*2)
	for i, v := range u {
		raw[2*i] = byte(v)
		raw[2*i+1] = byte(v >> 8)
	}
	return raw
}

func WriteOwnerPID() error {
	return os.WriteFile(ownerPIDAbs(), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

func RemoveOwnerPID() {
	_ = os.Remove(ownerPIDAbs())
}
