// package deps handles neovim plugin manager detection
package deps

import (
	"os"
	"path/filepath"
	"strings"
)

// NvimPluginManager represents a neovim plugin manager
type NvimPluginManager struct {
	Name        string
	Identifier  string // what to search for in config files
	InstallURL  string
	InstallPath string
	InstallCmd  string // command to install it
}

var (
	// Packer is a popular neovim plugin manager
	Packer = &NvimPluginManager{
		Name:        "packer.nvim",
		Identifier:  "packer",
		InstallURL:  "https://github.com/wbthomason/packer.nvim",
		InstallPath: "~/.local/share/nvim/site/pack/packer/start/packer.nvim",
		InstallCmd:  "git clone --depth 1 https://github.com/wbthomason/packer.nvim ~/.local/share/nvim/site/pack/packer/start/packer.nvim",
	}

	// Lazy is a modern neovim plugin manager
	Lazy = &NvimPluginManager{
		Name:        "lazy.nvim",
		Identifier:  "lazy",
		InstallURL:  "https://github.com/folke/lazy.nvim",
		InstallPath: "~/.local/share/nvim/lazy/lazy.nvim",
		InstallCmd:  "git clone --filter=blob:none https://github.com/folke/lazy.nvim.git --branch=stable ~/.local/share/nvim/lazy/lazy.nvim",
	}

	// VimPlug is the classic vim/neovim plugin manager
	VimPlug = &NvimPluginManager{
		Name:        "vim-plug",
		Identifier:  "plug#begin",
		InstallURL:  "https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim",
		InstallPath: "~/.local/share/nvim/site/autoload/plug.vim",
		InstallCmd:  "sh -c 'curl -fLo ~/.local/share/nvim/site/autoload/plug.vim --create-dirs https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim'",
	}
)

// DetectPluginManager scans nvim config files for plugin manager usage
func DetectPluginManager(configPath string) (*NvimPluginManager, error) {
	managers := []*NvimPluginManager{Lazy, Packer, VimPlug}

	// Walk the config directory
	var foundManager *NvimPluginManager
	err := filepath.Walk(configPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Only check .lua and .vim files
		ext := filepath.Ext(path)
		if ext != ".lua" && ext != ".vim" {
			return nil
		}

		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Check for identifiers
		contentStr := string(content)
		for _, pm := range managers {
			if strings.Contains(contentStr, pm.Identifier) {
				foundManager = pm
				return filepath.SkipAll // found it, stop walking
			}
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return nil, err
	}

	return foundManager, nil
}

// IsPluginManagerInstalled checks if the plugin manager is installed
func IsPluginManagerInstalled(pm *NvimPluginManager) bool {
	path := expandPath(pm.InstallPath)
	_, err := os.Stat(path)
	return err == nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

// ExpandInstallPath expands ~ in install path
func (pm *NvimPluginManager) ExpandInstallPath() string {
	return expandPath(pm.InstallPath)
}
