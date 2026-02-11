# dotfile picker

## what this is
- terminal ui built with bubble tea that lets you browse curated dotfile creators, preview their setups, and safely apply configs to your machine
- ships with manifest fetching, dependency hints, plugin manager helpers, and automatic backups so you can experiment without wrecking your setup
- written in go because bubbles, lipgloss, and the go standard library make fast native tooling feel fun again
- this is v1 of the experience; future drops will add more creators, smarter diffs, and new workflows without breaking the current flow

## inspired by
- classic csgo config picker maps where you wandered around a lobby, clicked a name, and instantly loaded someone else's setup
- the amount of hours i spent tweaking my dots when i was younger


## install
1. ensure go 1.25 or newer is on your path (`go version` should work)
2. clone the repo: `git clone git@github.com:milxzy/dotfile-picker.git && cd dotfile-picker`
3. build the tui: `make build` (drops a `dotpicker` binary into `bin/`)
4. optional system install: `sudo make install` to copy the binary to `/usr/local/bin`

if you prefer scripts, `./install.sh` handles the build plus a local install in one go. uninstalling is `sudo make uninstall`.

## how to use it
### interactive tui
1. run `./bin/dotpicker` (or `dotpicker` if you installed globally)
2. wait for the manifest download spinner to finish, then move through categories with the arrow keys
3. hit `enter` to drill into a creator, view their dotfiles, and select one to inspect
4. the app checks dependencies, pulls the repo into your cache, and detects structure (chezmoi, stow, plain directories, etc)
5. review diffs, confirm, and let the applier copy files while it tucks backups into `~/.config/dotfile-picker/backups`

key bindings: `enter` selects, `esc` goes back, `q` quits, `ctrl+c` hard exits. prompts for deps, plugin managers, or submodules accept the key hints shown on screen.

### headless demo
- run `go run ./cmd/dotpicker-demo` to print config dirs, manifest stats, and a quick tour of featured creators without launching the tui

## troubleshooting basics
- delete `~/.config/dotfile-picker/cache/<creator>` if a repo clone gets messy, then retry
- logs live in `~/.config/dotfile-picker/logs` when the logger is enabled (default scaffolding is ready even if most commands stay quiet)
- rerun `go mod tidy` whenever you upgrade go modules or pull big dependency changes

## roadmap ideas
- richer manifest metadata (platform tags, screenshots, verification badges)
- diff viewing upgrades that highlight highlights triple column style
- transplant mode to move configs between machines using the existing backup metadata
- milestone markers will land as v2, v3, etc so changes stay grouped and folks can follow along

## contributing
- open an issue with context (what you tried, what broke)
- keep gofmt and goimports happy before pushing
- add tests for diff logic or applier changes where practical
- pr template is simple: describe the change, show before and after behavior, mention manual test steps

thanks for keeping dotfiles weird but safe.
