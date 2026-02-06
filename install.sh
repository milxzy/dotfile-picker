#!/bin/bash
# install script for dotfile picker

set -e

# colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # no color

# config
REPO="milxzy/dot-generator"
BINARY_NAME="dotpicker"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

echo "installing dotfile picker..."

# detect os and arch
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux*)     OS="linux";;
    Darwin*)    OS="darwin";;
    *)          echo -e "${RED}unsupported os: $OS${NC}"; exit 1;;
esac

case "$ARCH" in
    x86_64*)    ARCH="amd64";;
    arm64*)     ARCH="arm64";;
    aarch64*)   ARCH="arm64";;
    *)          echo -e "${RED}unsupported architecture: $ARCH${NC}"; exit 1;;
esac

# get latest release
echo "fetching latest release..."
LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
DOWNLOAD_URL=$(curl -s "$LATEST_URL" | grep "browser_download_url.*${OS}_${ARCH}" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo -e "${YELLOW}no prebuilt binary found, installing from source...${NC}"
    
    # check if go is installed
    if ! command -v go &> /dev/null; then
        echo -e "${RED}go is not installed. please install go 1.21+ first${NC}"
        exit 1
    fi
    
    # install from source
    echo "building from source..."
    go install github.com/${REPO}/cmd/dotpicker@latest
    
    echo -e "${GREEN}installed successfully!${NC}"
    echo "run 'dotpicker' to start"
    exit 0
fi

# download binary
echo "downloading ${DOWNLOAD_URL}..."
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

curl -sL "$DOWNLOAD_URL" -o dotpicker.tar.gz
tar -xzf dotpicker.tar.gz

# install
echo "installing to ${INSTALL_DIR}..."
if [ -w "$INSTALL_DIR" ]; then
    mv dotpicker "$INSTALL_DIR/"
else
    sudo mv dotpicker "$INSTALL_DIR/"
fi

chmod +x "${INSTALL_DIR}/dotpicker"

# cleanup
cd -
rm -rf "$TMP_DIR"

echo -e "${GREEN}installed successfully!${NC}"
echo ""
echo "run 'dotpicker' to start"
echo ""
echo "for more info: https://github.com/${REPO}"
