#!/usr/bin/env bash

# GitVista Pre-commit Hook Setup Script
# This script installs lefthook and required Go tools for development

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect OS
OS=$(uname -s)
case "$OS" in
    Darwin)
        DETECTED_OS="macOS"
        ;;
    Linux)
        DETECTED_OS="Linux"
        ;;
    *)
        DETECTED_OS="Unknown"
        ;;
esac

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "GitVista Pre-commit Hook Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check prerequisites
log_info "Checking prerequisites..."
echo ""

if ! command_exists git; then
    log_error "Git is not installed. Please install Git first."
    exit 1
fi
log_success "Git found: $(git --version)"

if ! command_exists go; then
    log_error "Go is not installed. Please install Go 1.26+ first."
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
log_success "Go found: $GO_VERSION"

echo ""

# Install lefthook
log_info "Installing lefthook..."
echo ""

if command_exists lefthook; then
    LEFTHOOK_VERSION=$(lefthook -version 2>/dev/null || echo "unknown")
    log_success "Lefthook already installed: $LEFTHOOK_VERSION"
else
    case "$DETECTED_OS" in
        macOS)
            if command_exists brew; then
                log_info "Installing lefthook via Homebrew..."
                brew install lefthook
                log_success "Lefthook installed"
            else
                log_warn "Homebrew not found. Please install lefthook manually:"
                echo "  Visit: https://github.com/evilmartians/lefthook/releases"
                echo "  Or: brew install lefthook (macOS)"
                exit 1
            fi
            ;;
        Linux)
            if command_exists apt; then
                log_info "Installing lefthook via apt..."
                sudo apt update
                sudo apt install -y lefthook
                log_success "Lefthook installed"
            elif command_exists brew; then
                log_info "Installing lefthook via Homebrew..."
                brew install lefthook
                log_success "Lefthook installed"
            else
                log_warn "No package manager found. Please install lefthook manually:"
                echo "  Visit: https://github.com/evilmartians/lefthook/releases"
                exit 1
            fi
            ;;
        *)
            log_warn "Unknown OS: $DETECTED_OS"
            echo "  Please install lefthook manually:"
            echo "  Visit: https://github.com/evilmartians/lefthook/releases"
            exit 1
            ;;
    esac
fi

echo ""

# Install Go tools
log_info "Installing Go tools..."
echo ""

TOOLS=(
    "github.com/golang/tools/cmd/goimports@latest"
    "honnef.co/go/tools/cmd/staticcheck@latest"
    "github.com/securego/gosec/v2/cmd/gosec@latest"
    "github.com/golang/vuln/cmd/govulncheck@latest"
)

for tool in "${TOOLS[@]}"; do
    TOOL_NAME=$(echo "$tool" | awk -F'/' '{print $(NF)}' | awk -F'@' '{print $1}')

    if command_exists "$TOOL_NAME"; then
        log_success "$TOOL_NAME already installed"
    else
        log_info "Installing $TOOL_NAME..."
        if ! go install "$tool"; then
            log_error "Failed to install $TOOL_NAME"
            exit 1
        fi
        log_success "$TOOL_NAME installed"
    fi
done

echo ""

# Install hooks
log_info "Installing Git hooks..."
cd "$PROJECT_ROOT"

if lefthook install; then
    log_success "Git hooks installed"
else
    log_error "Failed to install Git hooks"
    exit 1
fi

echo ""

# Run hooks once to verify
log_info "Verifying hook installation..."
if lefthook version >/dev/null 2>&1; then
    log_success "Hook verification passed"
else
    log_warn "Hook verification had issues - hooks may still be functional"
fi

echo ""

# Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
log_success "Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Review hook configuration:"
echo "     cat lefthook.yml"
echo ""
echo "  2. Test hooks by making a commit:"
echo "     git add README.md && git commit -m 'test: verify hooks'"
echo ""
echo "  3. Run hooks manually:"
echo "     lefthook run pre-commit"
echo ""
echo "  4. Start developing!"
echo ""
echo "For more information, see DEVELOPMENT.md"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
