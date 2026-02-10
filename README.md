# dotpicker

a friendly tui for discovering and applying dotfiles from your favorite content creators.

## what's this about?

ever see someone's sick terminal setup in a video and think "i need that"? this tool helps you browse dotfiles from content creators and apply them to your system without the hassle.

## features

- browse dotfiles from various content creators
- preview configs before applying them
- backup your existing configs automatically
- dependency checking so you don't break things
- git integration for managing dotfile repos
- clean terminal interface that doesn't get in your way

## quick start

```bash
# install it
go install github.com/milxzy/dot-generator/cmd/dotpicker@latest

# or if you prefer building from source
git clone https://github.com/milxzy/dot-generator
cd dot-generator
make install

# run it
dotpicker
```

## how it works

1. **browse** - pick from curated dotfiles collections
2. **preview** - see what's going to change before you commit
3. **backup** - we'll save your current configs just in case
4. **apply** - let the tool handle the installation
5. **enjoy** - your new setup is ready to go!

## safety first

- automatic backups of existing configs
- dependency checking before installation  
- diff preview so you know what's changing
- rollback support if something goes wrong

## supported configs

currently supports dotfiles for:
- nvim configurations
- shell configs (zsh, bash)
- tmux setups
- git configurations
- and more content creator configs

## building from source

```bash
# grab the code
git clone https://github.com/milxzy/dot-generator
cd dot-generator

# build it
make build

# or install system-wide
make install
```

## contributing

found a bug? want to add support for more dotfiles? pull requests are welcome!

## license

mit license - do whatever you want with this code, just don't blame me if something breaks :)