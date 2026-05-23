package paths

import (
	"os"
	"path/filepath"
)

func LocalAppMegiddo() (string, error) {
	dir, ok := os.LookupEnv("LOCALAPPDATA")
	if !ok || dir == "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(dir, "Megiddo"), nil
}

func ProxyCADir() (string, error) {
	base, err := LocalAppMegiddo()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "proxy_ca"), nil
}

func PacksDir() (string, error) {
	base, err := LocalAppMegiddo()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "packs"), nil
}

func KTX2CacheDir() (string, error) {
	base, err := LocalAppMegiddo()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ktx2-cache"), nil
}

func SystemHostsFile() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}
