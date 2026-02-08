// package deps handles package manager detection and installation
package deps

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PackageManager represents a system package manager
type PackageManager struct {
	Name       string
	InstallCmd string
	UpdateCmd  string
	SearchCmd  string
}

var (
	// Homebrew for macOS
	Homebrew = &PackageManager{
		Name:       "homebrew",
		InstallCmd: "brew install",
		UpdateCmd:  "brew update",
		SearchCmd:  "brew search",
	}

	// Apt for Debian/Ubuntu
	Apt = &PackageManager{
		Name:       "apt",
		InstallCmd: "sudo apt-get install -y",
		UpdateCmd:  "sudo apt-get update",
		SearchCmd:  "apt-cache search",
	}

	// Pacman for Arch Linux
	Pacman = &PackageManager{
		Name:       "pacman",
		InstallCmd: "sudo pacman -S --noconfirm",
		UpdateCmd:  "sudo pacman -Sy",
		SearchCmd:  "pacman -Ss",
	}

	// Dnf for Fedora
	Dnf = &PackageManager{
		Name:       "dnf",
		InstallCmd: "sudo dnf install -y",
		UpdateCmd:  "sudo dnf check-update",
		SearchCmd:  "dnf search",
	}
)

// DetectPackageManager detects which package manager is available
func DetectPackageManager() (*PackageManager, error) {
	// Check in order of preference
	managers := []*PackageManager{Homebrew, Apt, Pacman, Dnf}

	for _, pm := range managers {
		// Extract command name (e.g., "brew" from "brew install")
		parts := strings.Fields(pm.InstallCmd)
		if len(parts) == 0 {
			continue
		}
		cmdName := parts[0]
		if cmdName == "sudo" && len(parts) > 1 {
			cmdName = parts[1]
		}

		if IsInstalled(cmdName) {
			return pm, nil
		}
	}

	return nil, fmt.Errorf("no supported package manager found (checked: brew, apt, pacman, dnf)")
}

// GetInstallCommand returns the command to install a dependency
func (pm *PackageManager) GetInstallCommand(dep Dependency) string {
	pkgName := dep.PackageNames[pm.Name]
	if pkgName == "" {
		pkgName = dep.Name // fallback to dependency name
	}
	return fmt.Sprintf("%s %s", pm.InstallCmd, pkgName)
}

// Install executes the install command
func (pm *PackageManager) Install(dep Dependency) error {
	cmdStr := pm.GetInstallCommand(dep)
	// Split command into parts
	parts := splitCommand(cmdStr)

	if len(parts) == 0 {
		return fmt.Errorf("empty install command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin // allow interactive prompts

	return cmd.Run()
}

func splitCommand(cmdStr string) []string {
	// Simple split on spaces (handles quoted args via shell)
	return strings.Fields(cmdStr)
}
