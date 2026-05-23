package winutil

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
)

func ExplainListenError(bindAddr string, err error) error {
	if err == nil {
		return nil
	}
	if hint := listenErrorHint(bindAddr, err); hint != "" {
		return fmt.Errorf("%s (%w)", hint, err)
	}
	return fmt.Errorf("bind %s: %w", bindAddr, err)
}

func listenErrorHint(bindAddr string, err error) string {
	lower := strings.ToLower(err.Error())
	if portBusy(err, lower) {
		return fmt.Sprintf(
			"bind %s: port already in use - close other Megiddo instances, IIS, nginx, or any app listening on %s (PowerShell: netstat -ano | findstr :443)",
			bindAddr,
			bindAddr,
		)
	}
	if accessDenied(err, lower) {
		return fmt.Sprintf(
			"bind %s: access denied - Megiddo must run as administrator to listen on port 443",
			bindAddr,
		)
	}
	return ""
}

func portBusy(err error, lower string) bool {
	if strings.Contains(lower, "only one usage of each socket address") ||
		strings.Contains(lower, "address already in use") {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return true
		}
		if errno, ok := opErr.Err.(syscall.Errno); ok && errno == syscall.EADDRINUSE {
			return true
		}
	}
	return false
}

func accessDenied(err error, lower string) bool {
	if strings.Contains(lower, "access is denied") {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EACCES) {
		return true
	}
	return false
}
