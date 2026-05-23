package hostsmanage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"megiddo/internal/megiddo"
	"megiddo/internal/paths"
)

var defaultHostsPreamble = "# Copyright Megiddo - hosts template\r\n#\r\n# 127.0.0.1       localhost\r\n"

func readOrDefault(hostsPath string) (string, error) {
	data, err := os.ReadFile(hostsPath)
	if os.IsNotExist(err) {
		return defaultHostsPreamble, nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func RemoveMegiddoEntries(interceptHosts []string) error {
	hf := paths.SystemHostsFile()
	existing, err := readOrDefault(hf)
	if err != nil {
		return err
	}
	clean := StripRedirects(existing, interceptHosts)
	return writeHostsAtomic(hf, eolTerminated(clean))
}

func eolTerminated(s string) string {
	t := strings.TrimRight(s, "\r\n\t ")
	if t == "" {
		return ""
	}
	return t + "\r\n"
}

func InstallRedirects(interceptHosts []string) error {
	hf := paths.SystemHostsFile()
	base, err := readOrDefault(hf)
	if err != nil {
		return err
	}
	stripped := filterMegiddoLines(base, interceptHosts)
	want := dedupeIntercept(interceptHosts)
	var sb strings.Builder
	body := strings.TrimRight(stripped, "\r\n\t ")
	if body != "" {
		sb.WriteString(body)
		sb.WriteString("\r\n")
	}
	for _, host := range want {
		sb.WriteString(fmt.Sprintf("127.0.0.1 %s %s\r\n", host, megiddo.HostMarker))
	}
	return writeHostsAtomic(hf, sb.String())
}

func dedupeIntercept(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, x := range in {
		key := strings.ToLower(x)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func StripRedirects(existing string, interceptHosts []string) string {
	return filterMegiddoLines(existing, interceptHosts)
}

func filterMegiddoLines(existing string, interceptHosts []string) string {
	var b strings.Builder
	for _, raw := range strings.Split(existing, "\n") {
		line := strings.TrimRight(raw, "\r")
		if shouldDropLine(line, interceptHosts) {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(line)
	}
	return b.String()
}

func shouldDropLine(line string, interceptHosts []string) bool {
	if strings.Contains(line, megiddo.HostMarker) {
		return true
	}
	low := strings.ToLower(strings.TrimSpace(line))
	for _, h := range interceptHosts {
		match := strings.ToLower(strings.TrimSpace(fmt.Sprintf("127.0.0.1 %s", h)))
		if low == match {
			return true
		}
	}
	return false
}

const writeRetries = 8

func writeHostsAtomic(dest, content string) error {
	dir := filepath.Dir(dest)
	if err := tryDirectWrite(dest, content); err == nil {
		return nil
	}

	tmpfile, err := os.CreateTemp(dir, ".megiddo_hosts_*")
	if err != nil {
		return err
	}
	tmpPath := tmpfile.Name()
	if _, err := tmpfile.WriteString(content); err != nil {
		tmpfile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpfile.Sync(); err != nil {
		tmpfile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpfile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dest)
}

func tryDirectWrite(dest, content string) error {
	if fi, err := os.Stat(dest); err == nil {
		mode := fi.Mode().Perm()
		if mode&0200 == 0 {
			_ = os.Chmod(dest, mode|0200)
		}
	}
	var lastErr error
	for i := 0; i < writeRetries; i++ {
		lastErr = os.WriteFile(dest, []byte(content), 0o644)
		if lastErr == nil {
			return nil
		}
		ls := strings.ToLower(lastErr.Error())
		retry := strings.Contains(ls, "being used") || strings.Contains(ls, "permission") ||
			strings.Contains(ls, "denied") || strings.Contains(ls, "access")
		if !retry {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return lastErr
}
