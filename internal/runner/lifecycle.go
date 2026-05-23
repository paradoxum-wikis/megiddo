package runner

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"megiddo/internal/certmgr"
	"megiddo/internal/dnsflush"
	"megiddo/internal/hostsmanage"
	"megiddo/internal/localserve"
	"megiddo/internal/megiddo"
	"megiddo/internal/mitm"
	"megiddo/internal/paths"
	"megiddo/internal/processwin"
	"megiddo/internal/reboothosts"
	"megiddo/internal/replacement"
	"megiddo/internal/resolving"
	"megiddo/internal/texpacklookup"
	"megiddo/internal/watchdog"
	"megiddo/internal/winutil"
)

func Lifecycle(ctx context.Context, logSink func(format string, args ...any), rep *replacement.Map, files *localserve.Store, tpl *texpacklookup.Store) error {
	if logSink == nil {
		logSink = func(string, ...any) {}
	}

	pidPath := watchdog.ProxyOwnerAbs()
	if !elevatedNeighbor(pidPath, os.Getpid()) {
		watchdog.DeleteMegiddoTask()
		if err := hostsmanage.RemoveMegiddoEntries(megiddo.InterceptHosts); err != nil {
			return fmt.Errorf("remove stale Megiddo hosts redirects: %w (hosts file=%s)", err, paths.SystemHostsFile())
		}
		dnsflush.Flush()
	} else {
		logSink("megiddo: another elevated PID owns %s - skipping startup hosts/watchdog teardown", pidPath)
	}

	caDir, err := paths.ProxyCADir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(caDir, 0o755); err != nil {
		return err
	}
	if err := certmgr.EnsureCA(caDir); err != nil {
		return fmt.Errorf("bootstrap CA dir: %w", err)
	}
	leafMap := map[string]*tls.Certificate{}
	var defaultLeaf *tls.Certificate
	for _, host := range megiddo.InterceptHosts {
		tlsc, err := certmgr.TLSPair(host, caDir)
		if err != nil {
			return fmt.Errorf("leaf tls material for %s: %w", host, err)
		}
		key := strings.ToLower(host)
		leafMap[key] = tlsc
		defaultLeaf = tlsc
	}

	upstream := map[string][]string{}
	ctxLookup, cancelLookup := context.WithTimeout(ctx, 25*time.Second)
	defer cancelLookup()
	for _, host := range megiddo.InterceptHosts {
		ips, err := resolving.RealIPs(ctxLookup, host)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("resolve real CDN IPs for %s: %w", host, err)
		}
		upstream[strings.ToLower(host)] = ips
		logSink("megiddo: %s -> upstream %s", host, ips[0])
	}

	srv := &mitm.Server{
		UpstreamIPs:   upstream,
		Leaves:        leafMap,
		DefaultLeaf:   defaultLeaf,
		Logf:          logSink,
		Replacements:  rep,
		LocalFiles:    files,
		TexpackLookup: tpl,
	}

	bindAddr := fmt.Sprintf("127.0.0.1:%d", megiddo.ProxyPort)
	if err := srv.Listen(bindAddr); err != nil {
		return winutil.ExplainListenError(bindAddr, err)
	}
	defer srv.Close()

	ctxMitm, stopMitm := context.WithCancel(ctx)
	defer stopMitm()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctxMitm) }()

	if err := hostsmanage.InstallRedirects(megiddo.InterceptHosts); err != nil {
		stopMitm()
		return fmt.Errorf("write hosts redirection: %w", err)
	}
	dnsflush.Flush()

	tempGuard := filepath.Join(os.TempDir(), megiddo.PendingRenameTempFile)
	if err := reboothosts.ScheduleMegiddoPendingRename(megiddo.InterceptHosts, tempGuard); err != nil {
		logSink("megiddo: PendingFileRename guard failed (%v) - relying on watchdog only", err)
	}
	if err := watchdog.WriteOwnerPID(); err != nil {
		logSink("megiddo: PID sentinel write failed (%v)", err)
	}
	if err := watchdog.UpsertMegiddoTask(); err != nil {
		stopMitm()
		return fmt.Errorf("create watchdog scheduled task: %w", err)
	}

	go refreshWatchdog(ctx, logSink)

	logSink("megiddo: proxy active on %s (targets: %s)", bindAddr, strings.Join(megiddo.InterceptHosts, ", "))

	var shutdownErr error
	mitmExited := false
	select {
	case <-ctx.Done():
		shutdownErr = ctx.Err()
	case err := <-errCh:
		shutdownErr = err
		mitmExited = true
	}

	stopMitm()
	if !mitmExited {
		_ = <-errCh
	}

	watchdog.DeleteMegiddoTask()
	if herr := hostsmanage.RemoveMegiddoEntries(megiddo.InterceptHosts); herr != nil {
		logSink("megiddo: shutdown hosts warning: %v", herr)
	} else {
		dnsflush.Flush()
	}
	if err := reboothosts.CancelMegiddoPendingRename(tempGuard); err != nil {
		logSink("megiddo: PendingFileRename cancel warning: %v", err)
	}
	watchdog.RemoveOwnerPID()

	if shutdownErr != nil && !errors.Is(shutdownErr, context.Canceled) {
		return shutdownErr
	}
	return nil
}

func refreshWatchdog(ctx context.Context, logSink func(string, ...any)) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := watchdog.UpsertMegiddoTask(); err != nil {
				logSink("megiddo: watchdog reschedule: %v", err)
			}
		}
	}
}

func elevatedNeighbor(pidFile string, self int) bool {
	raw, err := os.ReadFile(pidFile)
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid == self {
		return false
	}
	return processwin.Running(fmt.Sprintf("%d", pid))
}
