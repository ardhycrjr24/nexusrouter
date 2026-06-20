#!/usr/bin/env bash
set -euo pipefail

REPO="${NEXUSROUTER_REPO:-https://github.com/ardhycrjr24/nexusrouter.git}"
BRANCH="${NEXUSROUTER_BRANCH:-main}"
INSTALL_DIR="${NEXUSROUTER_DIR:-$HOME/.nexusrouter}"
BIN_DIR="${NEXUSROUTER_BIN_DIR:-/usr/local/bin}"
SHARE_DIR="${NEXUSROUTER_SHARE_DIR:-/usr/local/share/nexusrouter}"
FORCE_MODE="${NEXUSROUTER_FORCE:-}" # replace, sync/keep, or abort
INSTALL_MODE="${NEXUSROUTER_INSTALL_MODE:-source}" # source or docker

info() { printf "\033[1;34m>\033[0m %s\n" "$*"; }
ok() { printf "\033[1;32mOK\033[0m %s\n" "$*"; }
warn() { printf "\033[1;33m!\033[0m %s\n" "$*"; }
die() {
  printf "\033[1;31mERROR\033[0m %s\n" "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install NexusRouter.

Usage:
  install.sh [--source|--docker]

Modes:
  --source  Build and install a local nexusrouter binary. Requires Go 1.24+,
            Node.js 20+, npm, and git. This is the default.
  --docker  Clone/update the repo and start NexusRouter with Docker Compose.
            Requires Docker with the compose plugin.

Environment overrides:
  NEXUSROUTER_DIR           Source checkout directory (default: ~/.nexusrouter)
  NEXUSROUTER_BIN_DIR       Binary install directory (default: /usr/local/bin)
  NEXUSROUTER_SHARE_DIR     Dashboard asset directory (default: /usr/local/share/nexusrouter)
  NEXUSROUTER_FORCE         replace, sync/keep, or abort for existing non-git dirs
  NEXUSROUTER_BRANCH        Git branch to install (default: main)
  NEXUSROUTER_INSTALL_MODE  source or docker
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --source)
      INSTALL_MODE="source"
      ;;
    --docker)
      INSTALL_MODE="docker"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown option: $1"
      ;;
  esac
  shift
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found"
}

version_ge() {
  awk -v have="$1" -v need="$2" 'BEGIN {
    split(have, h, ".")
    split(need, n, ".")
    for (i = 1; i <= 3; i++) {
      hv = h[i] + 0
      nv = n[i] + 0
      if (hv > nv) exit 0
      if (hv < nv) exit 1
    }
    exit 0
  }'
}

require_source_prereqs() {
  for cmd in git go node npm awk; do
    need_cmd "$cmd"
  done

  go_version="$(go version | awk '{print $3}' | sed 's/^go//' | cut -d- -f1)"
  version_ge "$go_version" "1.24.0" || die "Go 1.24+ is required (found $go_version)"

  node_version="$(node -v | sed 's/^v//' | cut -d- -f1)"
  version_ge "$node_version" "20.0.0" || die "Node.js 20+ is required (found $node_version)"
}

require_docker_prereqs() {
  need_cmd git
  need_cmd docker
  docker compose version >/dev/null 2>&1 || die "Docker Compose plugin is required (docker compose)"
}

handle_existing_dir() {
  if [ -d "$INSTALL_DIR/.git" ]; then
    info "Updating NexusRouter in $INSTALL_DIR"
    git -C "$INSTALL_DIR" fetch origin "$BRANCH" --depth 1 --quiet
    git -C "$INSTALL_DIR" checkout "$BRANCH" --quiet 2>/dev/null || true
    git -C "$INSTALL_DIR" pull --ff-only origin "$BRANCH" --quiet
    cd "$INSTALL_DIR"
    return 0
  fi

  if [ -d "$INSTALL_DIR" ]; then
    choice="$FORCE_MODE"
    if [ -z "$choice" ]; then
      echo ""
      warn "Directory $INSTALL_DIR already exists but is not a git checkout."
      echo "  1) Replace  - remove it and fresh clone"
      echo "  2) Sync     - overwrite it with latest $BRANCH"
      echo "  3) Abort"
      echo ""
      read -rp "Choose [1/2/3]: " choice </dev/tty
    fi

    case "$choice" in
      1|replace)
        warn "Removing existing $INSTALL_DIR"
        rm -rf "$INSTALL_DIR"
        info "Cloning NexusRouter into $INSTALL_DIR"
        git clone --depth 1 --branch "$BRANCH" "$REPO" "$INSTALL_DIR"
        ;;
      2|sync|keep)
        warn "Syncing $INSTALL_DIR to origin/$BRANCH. Local files may be overwritten."
        if ! git -C "$INSTALL_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
          git -C "$INSTALL_DIR" init -q
          git -C "$INSTALL_DIR" remote add origin "$REPO" 2>/dev/null || \
            git -C "$INSTALL_DIR" remote set-url origin "$REPO"
        fi
        git -C "$INSTALL_DIR" fetch origin "$BRANCH" --depth 1 --quiet
        git -C "$INSTALL_DIR" reset --hard "origin/$BRANCH" --quiet
        ;;
      3|abort)
        die "Aborted by user."
        ;;
      *)
        die "Invalid choice: $choice"
        ;;
    esac
    cd "$INSTALL_DIR"
    return 0
  fi

  info "Cloning NexusRouter into $INSTALL_DIR"
  git clone --depth 1 --branch "$BRANCH" "$REPO" "$INSTALL_DIR"
  cd "$INSTALL_DIR"
}

copy_with_optional_sudo() {
  src="$1"
  dest="$2"
  mode="${3:-}"
  dest_dir="$(dirname "$dest")"

  if mkdir -p "$dest_dir" 2>/dev/null && [ -w "$dest_dir" ]; then
    cp "$src" "$dest"
    [ -n "$mode" ] && chmod "$mode" "$dest"
    return 0
  fi

  need_cmd sudo
  info "Need sudo to write $dest"
  sudo mkdir -p "$dest_dir"
  sudo cp "$src" "$dest"
  [ -n "$mode" ] && sudo chmod "$mode" "$dest"
}

install_frontend_assets() {
  src="$INSTALL_DIR/frontend/dist"
  dest="$SHARE_DIR/frontend/dist"
  [ -d "$src" ] || die "Dashboard build output not found at $src"

  if mkdir -p "$(dirname "$dest")" 2>/dev/null && [ -w "$(dirname "$dest")" ]; then
    rm -rf "$dest"
    cp -R "$src" "$dest"
    return 0
  fi

  need_cmd sudo
  info "Need sudo to install dashboard assets to $dest"
  sudo mkdir -p "$(dirname "$dest")"
  sudo rm -rf "$dest"
  sudo cp -R "$src" "$dest"
}

run_source_install() {
  require_source_prereqs
  handle_existing_dir

  info "Installing dashboard dependencies"
  (cd frontend && npm ci --quiet)

  info "Building dashboard"
  (cd frontend && npm run build --silent)

  info "Building nexusrouter binary"
  GIT_DESCRIBE="$(git -C "$INSTALL_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
  (cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=${GIT_DESCRIBE}" -o "$INSTALL_DIR/nexusrouter" ./cmd/nexusrouter)

  copy_with_optional_sudo "$INSTALL_DIR/nexusrouter" "$BIN_DIR/nexusrouter" "0755"
  install_frontend_assets

  ok "NexusRouter installed to $BIN_DIR/nexusrouter"
  ok "Dashboard assets installed to $SHARE_DIR/frontend/dist"
  echo ""
  echo "Quick start:"
  echo "  nexusrouter                  # start the server on :20180"
  echo "  nexusrouter -bootstrap       # create your first API key"
  echo ""
  echo "Dashboard: http://localhost:20180  (default password: nexusrouter)"
}

run_docker_install() {
  require_docker_prereqs
  handle_existing_dir

  if [ ! -f .env ] && [ -f .env.example ]; then
    cp .env.example .env
    warn "Created .env from .env.example. Add NEXUSROUTER_MASTER_KEY for production/VPS installs."
  fi

  info "Starting NexusRouter with Docker Compose"
  docker compose up -d --build

  ok "NexusRouter is starting in Docker"
  echo ""
  echo "Dashboard: http://localhost:${NEXUSROUTER_PORT:-20180}"
  echo "Logs:      cd $INSTALL_DIR && docker compose logs -f nexusrouter"
}

case "$INSTALL_MODE" in
  source)
    run_source_install
    ;;
  docker)
    run_docker_install
    ;;
  *)
    die "Unknown NEXUSROUTER_INSTALL_MODE: $INSTALL_MODE"
    ;;
esac
