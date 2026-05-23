package reboothosts

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"megiddo/internal/hostsmanage"
	"megiddo/internal/paths"
)

const pendingKey = `SYSTEM\CurrentControlSet\Control\Session Manager`
const pendingVal = `PendingFileRenameOperations`

func ntPath(abs string) string {
	ap, err := filepath.Abs(abs)
	if err != nil {
		ap = abs
	}
	return `\??\` + filepath.Clean(ap)
}

func ScheduleMegiddoPendingRename(interceptHosts []string, tempCleanupFile string) error {
	hostsFile := paths.SystemHostsFile()

	var payload string
	if data, err := os.ReadFile(hostsFile); err == nil {
		payload = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}

	clean := hostsmanage.StripRedirects(payload, interceptHosts)
	if strings.TrimSpace(clean) == "" {
		clean = ""
	} else {
		clean = strings.TrimRight(clean, "\r\n\t ") + "\r\n"
	}
	if err := os.WriteFile(tempCleanupFile, []byte(clean), 0o644); err != nil {
		return err
	}

	src := ntPath(tempCleanupFile)
	dst := ntPath(hostsFile)

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, pendingKey,
		registry.SET_VALUE|registry.QUERY_VALUE|registry.READ)
	if err != nil {
		return err
	}
	defer key.Close()

	current, _, err := key.GetStringsValue(pendingVal)
	if err != nil && err != registry.ErrNotExist {
		return err
	}

	filtered := filterDuplicateSrc(current, src)
	filtered = append(filtered, src, dst)

	return key.SetStringsValue(pendingVal, filtered)
}

func CancelMegiddoPendingRename(tempCleanupFile string) error {
	src := ntPath(tempCleanupFile)

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, pendingKey,
		registry.SET_VALUE|registry.QUERY_VALUE|registry.READ)
	if err != nil {
		return err
	}
	defer key.Close()

	current, _, err := key.GetStringsValue(pendingVal)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}

	var next []string
	for i := 0; i+1 < len(current); i += 2 {
		if strings.EqualFold(current[i], src) {
			continue
		}
		next = append(next, current[i], current[i+1])
	}

	if len(next) == 0 {
		return key.DeleteValue(pendingVal)
	}
	return key.SetStringsValue(pendingVal, next)
}

func filterDuplicateSrc(pairs []string, src string) []string {
	if len(pairs) == 0 {
		return pairs
	}
	out := pairs[:0]
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.EqualFold(pairs[i], src) {
			continue
		}
		out = append(out, pairs[i], pairs[i+1])
	}
	return out
}
