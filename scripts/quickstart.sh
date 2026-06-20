#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# NexusRouter — Zero-config local quickstart
#
# One-liner usage (from repo root):
#   bash scripts/quickstart.sh
#
# Or from anywhere via curl:
#   curl -fsSL https://raw.githubusercontent.com/ardhycrjr24/nexusrouter/main/scripts/quickstart.sh | bash
#
# What it does:
#   1. Detects or clones the repo
#   2. Checks prerequisites (Go 1.24+, Node.js 20+)
#   3. Installs npm dependencies (if needed)
#   4. Downloads Go modules (if needed)
#   5. Starts backend + frontend dev servers together
#
# No .env required — NexusRouter auto-generates:
#   • SQLite database   → ~/.nexusrouter/nexusrouter.db
#   • Master key        → ~/.nexusrouter/master.key
#   • JWT secret        → random per session
#   • Dashboard password → "nexusrouter" (prompted to change on first login)
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

REPO="https://github.com/ardhycrjr24/nexusrouter.git"
BRANCH="${NEXUSROUTER_BRANCH:-main}"

# ── Pretty output ────────────────────────────────────────────────────────────
BOLD='\033[1m'
BLUE='\033[1;34m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
RED='\033[1;31m'
RESET='\033[0m'

info()  { printf "${BLUE}▸${RESET} %s\n" "$*"; }
ok()    { printf "${GREEN}✔${RESET} %s\n" "$*"; }
warn()  { printf "${YELLOW}!${RESET} %s\n" "$*"; }
die()   { printf "${RED}✖${RESET} %s\n" "$*" >&2; exit 1; }

banner() {
  echo ""
  printf "${BOLD}  ╭──────────────────────────────────────╮${RESET}\n"
  printf "${BOLD}  │${RESET}   ${GREEN}🚀 NexusRouter — Local Quickstart${RESET}    ${BOLD}│${RESET}\n"
  printf "${BOLD}  ╰──────────────────────────────────────╯${RESET}\n"
  echo ""
}

# ── Prerequisites ────────────────────────────────────────────────────────────
need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found. Install it first."
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

check_prereqs() {
  info "Checking prerequisites…"

  for cmd in go node npm; do
    need_cmd "$cmd"
  done

  go_version="$(go version | awk '{print $3}' | sed 's/^go//' | cut -d- -f1)"
  version_ge "$go_version" "1.24.0" || die "Go 1.24+ required (found $go_version)"
  ok "Go $go_version"

  node_version="$(node -v | sed 's/^v//' | cut -d- -f1)"
  version_ge "$node_version" "20.0.0" || die "Node.js 20+ required (found $node_version)"
  ok "Node.js $node_version"

  npm_version="$(npm -v)"
  ok "npm $npm_version"
}

# ── Repo detection ───────────────────────────────────────────────────────────
resolve_repo() {
  # Already inside the nexusrouter repo?
  if [ -f "Makefile" ] && grep -q "nexusrouter" Makefile 2>/dev/null; then
    REPO_DIR="$(pwd)"
    ok "Using current directory: $REPO_DIR"
    return
  fi

  # Check if we're in a subdirectory of the repo
  local dir="$(pwd)"
  while [ "$dir" != "/" ]; do
    if [ -f "$dir/Makefile" ] && grep -q "nexusrouter" "$dir/Makefile" 2>/dev/null; then
      REPO_DIR="$dir"
      ok "Found repo at: $REPO_DIR"
      return
    fi
    dir="$(dirname "$dir")"
  done

  # Clone fresh
  REPO_DIR="${NEXUSROUTER_DIR:-$HOME/nexusrouter}"
  if [ -d "$REPO_DIR/.git" ]; then
    info "Updating existing checkout at $REPO_DIR"
    git -C "$REPO_DIR" pull --ff-only origin "$BRANCH" --quiet 2>/dev/null || true
  else
    info "Cloning NexusRouter into $REPO_DIR"
    need_cmd git
    git clone --depth 1 --branch "$BRANCH" "$REPO" "$REPO_DIR"
  fi
  ok "Repo ready at: $REPO_DIR"
}

# ── Install deps ─────────────────────────────────────────────────────────────
install_deps() {
  cd "$REPO_DIR"

  # Frontend npm dependencies
  if [ ! -d "frontend/node_modules" ]; then
    info "Installing frontend dependencies (npm ci)…"
    (cd frontend && npm ci --quiet)
    ok "Frontend dependencies installed"
  else
    ok "Frontend dependencies already installed"
  fi

  info "Preparing Go modules…"
  (cd backend && go mod download)
  ok "Go modules ready"
}

# ── Start dev ────────────────────────────────────────────────────────────────
start_dev() {
  cd "$REPO_DIR"

  echo ""
  printf "${BOLD}  ┌───────────────────────────────────────────────┐${RESET}\n"
  printf "${BOLD}  │${RESET}  ${GREEN}Everything ready! Starting NexusRouter…${RESET}         ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}                                               ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}  ${BLUE}Backend${RESET}    → http://localhost:20180           ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}  ${BLUE}Dashboard${RESET}  → http://localhost:5180            ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}  ${BLUE}Password${RESET}   → nexusrouter (change on first use) ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}                                               ${BOLD}│${RESET}\n"
  printf "${BOLD}  │${RESET}  ${YELLOW}Press Ctrl+C to stop both servers.${RESET}            ${BOLD}│${RESET}\n"
  printf "${BOLD}  └───────────────────────────────────────────────┘${RESET}\n"
  echo ""

  # Run make dev — which starts backend + waits for healthz + starts frontend
  make dev
}

# ── Main ─────────────────────────────────────────────────────────────────────
main() {
  banner
  check_prereqs
  resolve_repo
  install_deps
  start_dev
}

main "$@"
