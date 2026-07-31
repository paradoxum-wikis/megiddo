package megiddo

const ProxyPort = 443

const CanaryCommonName = "Megiddo Proxy CA"

// appended to hosts lines so watchdog can find the entries
const HostMarker = "# Megiddo proxy entry"

const ProxyOwnerPIDFileName = "megiddo_proxy_owner.pid"

const WatchdogTaskName = "Megiddo-HostsWatchdog"

const PendingRenameTempFile = "megiddo_hosts_restore.txt"
const LocalServePathPrefix = "/megiddo-local/v1/"

const FtsHost = "fts.rbxcdn.com"

var InterceptHosts = []string{
	"assetdelivery.roblox.com",
	FtsHost,
}
