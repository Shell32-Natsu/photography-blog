package command

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/photography-blog/pkg/config"
)

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func CopyFilesWithShell(source, destination string) error {
	cmd := exec.Command("cp", "-rf", source, destination)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not copy files from %s to %s: %w", source, destination, err)
	}
	return nil
}

func CopyFilesWithCmd() error {
	cmd := exec.Command("XCOPY", "/E", "/Y", config.GetAssetsPath(), config.GetPublicAssetPath())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not copy files from %s to %s: %w", 
			config.GetAssetsPath(), config.GetPublicAssetPath(), err)
	}
	return nil
}

func CopyAssetFilesWithShell() error {
	// Copy assets
	if err := CopyFilesWithShell(config.AssetsPath, config.PublicPath); err != nil {
		return err
	}

	// Copy images if they exist
	if PathExists(config.ImagePath) {
		if err := CopyFilesWithShell(config.ImagePath, config.PublicPath); err != nil {
			return err
		}
	}

	return nil
}

func TryCopyFiles() error {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "solaris":
		return CopyAssetFilesWithShell()
	case "windows":
		return CopyFilesWithCmd()
	default:
		return fmt.Errorf("auto copy assets files in current os: %s is not supported, please do it yourself", runtime.GOOS)
	}
}