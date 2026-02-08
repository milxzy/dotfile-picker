// package deps handles dependency checking and installation
package deps

import (
	"os/exec"
	"strings"
)

// Dependency represents a required tool
type Dependency struct {
	Name         string            // e.g., "neovim"
	Command      string            // e.g., "nvim"
	PackageNames map[string]string // package manager -> package name
	Description  string
}

// CheckResult represents the result of checking a dependency
type CheckResult struct {
	Dependency Dependency
	Installed  bool
	Version    string
	InstallCmd string // command to install if missing
}

// Checker checks for installed dependencies
type Checker struct {
	pkgManager *PackageManager
}

// NewChecker creates a dependency checker
func NewChecker() (*Checker, error) {
	pm, err := DetectPackageManager()
	if err != nil {
		return nil, err
	}
	return &Checker{pkgManager: pm}, nil
}

// IsInstalled checks if a command is available
func IsInstalled(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// GetVersion gets the version of an installed command
func GetVersion(command string, versionFlag string) (string, error) {
	if versionFlag == "" {
		versionFlag = "--version"
	}

	cmd := exec.Command(command, versionFlag)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Return first line
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "", nil
}

// Check checks a single dependency
func (c *Checker) Check(dep Dependency) CheckResult {
	result := CheckResult{
		Dependency: dep,
		Installed:  IsInstalled(dep.Command),
	}

	if result.Installed {
		version, _ := GetVersion(dep.Command, "")
		result.Version = version
	} else {
		result.InstallCmd = c.pkgManager.GetInstallCommand(dep)
	}

	return result
}

// CheckMultiple checks multiple dependencies
func (c *Checker) CheckMultiple(deps []Dependency) []CheckResult {
	results := make([]CheckResult, 0, len(deps))
	for _, dep := range deps {
		results = append(results, c.Check(dep))
	}
	return results
}

// GetPackageManager returns the detected package manager
func (c *Checker) GetPackageManager() *PackageManager {
	return c.pkgManager
}
