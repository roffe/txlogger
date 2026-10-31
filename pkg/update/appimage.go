package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const appImageAsset = "txlogger-x86_64.AppImage"

// appImagePath returns the running AppImage path, or "" if not running as one.
func appImagePath() string {
	return os.Getenv("APPIMAGE")
}

// selfUpdateAppImage downloads the AppImage asset from rel and atomically replaces the running one.
func selfUpdateAppImage(rel *Release) error {
	target := appImagePath()
	if target == "" {
		return fmt.Errorf("not running as an AppImage")
	}
	var dl string
	for _, a := range rel.Assets {
		if a.Name == appImageAsset {
			dl = a.BrowserDownloadURL
		}
	}
	if dl == "" {
		return fmt.Errorf("release %s has no %s", rel.TagName, appImageAsset)
	}
	resp, err := http.Get(dl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".txlogger-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
