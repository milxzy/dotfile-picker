# dotfile picker architecture

> vibe reference: csgo config picker maps where you tap a creator tile and their loadout just snaps into place. we borrowed that flow for dotfiles and this document covers the v1 layout, with room for future versions when modules expand.

## top level map
- `cmd/dotpicker`: main tui binary
- `cmd/dotpicker-demo`: headless walkthrough binary
- `internal/`
  - `config`: defaults, directory bootstrap, manifest url
  - `manifest`: schema, remote fetch, repo structure detection
  - `cache`: git clone/pull helper plus submodule utilities
  - `deps`: cli dependency detection and installation hints
  - `backup`: timestamped snapshots of anything we overwrite
  - `diff`: unified diff engine for previews
  - `applier`: copies files into `$HOME`, talking to backup + diff
  - `tui`: bubble tea state machine, screens, and workflows

## data flow
1. `config.Default()` builds paths inside `~/.config/dotfile-picker`
2. `manifest` is loaded from local `configs/manifest.json` for fast startup
3. `tui.Model` orchestrates user selections and hands off to:
   - User selects category → creator → dotfile (no download yet)
   - `cache.Manager` downloads repo only when dotfile is selected
   - `manifest.DetectStructure` auto-detects repo layout
   - `deps.Checker` to warn about missing tools
   - `diff.Engine` to preview changes
   - `applier.Applier` to write files after creating backups

## package deep dive
### config
- file: `internal/config/config.go`
- responsibilities: figure out XDG paths, ensure cache/backup/log dirs exist, expose helpers like `CreatorCacheDir`

### manifest
- files: `internal/manifest/{types.go,fetcher.go,detector.go}`
- keeps the manifest schema (creators, categories, dotfiles)
- `Fetcher` handles remote + cached reads
- `DetectStructure` inspects cloned repos for layouts (chezmoi, stow, simple copy) and builds a `RepoStructure` used later

### cache
- files: `internal/cache/{manager.go,git.go}`
- wraps `git clone`, `git pull`, and submodule operations via `exec.Command`
- stores repos under `~/.config/dotfile-picker/cache/<creator>` so multiple runs reuse downloads

### deps
- files: `internal/deps/{checker.go,installer.go,nvim.go}`
- sniffs package managers (brew, apt, pacman, winget)
- defines dependency metadata per dotfile
- `NvimPluginManager` detectors spot lazy.nvim, packer.nvim, vim-plug and can install missing managers when the user agrees

### diff
- files: `internal/diff/engine.go`
- uses `github.com/sergi/go-diff` to generate unified diffs and summary stats per file
- outputs feed the tui diff screen before apply

### backup
- files: `internal/backup/manager.go`
- before applying a file, copies the existing version into `backups/<timestamp>/<relative-path>`
- provides restore helpers used if the applier hits an error mid-run

### applier
- files: `internal/applier/applier.go`
- main loop: expand tilde, ensure parent dirs, request backup, copy source file, report success
- handles both single files and entire directories based on the manifest structure info

### tui
- files: `internal/tui/{app.go,models.go,styles.go,dirbrowser.go,workflow_test.go}`
- entry point `Run()` sets up Bubble Tea, loads config, ensures directories, creates services
- `Model` holds all state: current screen, selected category/creator/dotfile, resolved files, diffs, dependency results
- screen flow (NEW): Loading → Category → Creator → Dotfile → Downloading (repo) → DependencyCheck (if needed) → TreeConfirm → PluginManagerDetect (nvim only) → Diff → Applying → Complete
- auto-detects repo structure; only shows directory browser if detection fails
- submodules are skipped entirely (modern plugin managers auto-install)
- views use Lip Gloss styles for titles, lists, tree views, and diff panes

## binaries
- `cmd/dotpicker/main.go`: thin wrapper running the tui
- `cmd/dotpicker-demo/main.go`: scripted walkthrough printing categories, featured creators, and usage hints without a TTY

## how to extend
1. new dotfile creator or layout? update the manifest registry, then ensure `manifest.DetectStructure` understands the repo pattern
2. new dependency check? declare it in `internal/deps` and surface in the manifest entry so the tui prompt knows what to warn about
3. new screen or workflow? add enums in `internal/tui/models.go`, implement view + handlers in `app.go`, and wire commands where needed

## tests
- `internal/tui/workflow_test.go` exercises key transitions to keep regressions from breaking interaction loops
- add targeted unit tests in other packages (diff, manifest, applier) when changing behavior; `go test ./...` stays fast

read this file side by side with the source tree and you can jump straight to the code you need without guesswork. contribute, remix, and keep the vibes friendly.
