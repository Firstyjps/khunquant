#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-khunquant}"
KHUNQUANT_BIN="${KHUNQUANT_BIN:-}"
KHUNQUANT_HOME="${KHUNQUANT_HOME:-$HOME/.khunquant}"
KHUNQUANT_SSH_KEY_PATH="${KHUNQUANT_SSH_KEY_PATH:-$HOME/.ssh/khunquant_ed25519.key}"
START_ARGS="${START_ARGS:-start --no-browser}"
ENABLE_LINGER="${ENABLE_LINGER:-1}"
ACTION="install"

usage() {
  cat <<'USAGE'
Install a user-level systemd service that starts KhunQuant at boot.

Usage:
  scripts/install-systemd-user-service.sh [install|status|uninstall]

Environment overrides:
  SERVICE_NAME              systemd unit name without .service (default: khunquant)
  KHUNQUANT_BIN             khunquant binary path (default: auto-detect with command -v)
  KHUNQUANT_HOME            config directory (default: ~/.khunquant)
  KHUNQUANT_SSH_KEY_PATH    credential SSH key path (default: ~/.ssh/khunquant_ed25519.key)
  START_ARGS                command args (default: start --no-browser)
  ENABLE_LINGER             enable boot without login, 1 or 0 (default: 1)

Examples:
  scripts/install-systemd-user-service.sh
  START_ARGS=start scripts/install-systemd-user-service.sh
  scripts/install-systemd-user-service.sh status
  scripts/install-systemd-user-service.sh uninstall

After install:
  systemctl --user status khunquant
  journalctl --user -u khunquant -f
USAGE
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -gt 0 ]]; then
  ACTION="$1"
fi

if [[ "$ACTION" != "install" && "$ACTION" != "status" && "$ACTION" != "uninstall" ]]; then
  usage
  die "unknown action: $ACTION"
fi

if ! command -v systemctl >/dev/null 2>&1; then
  die "systemctl not found; this script requires systemd"
fi

UNIT_DIR="$HOME/.config/systemd/user"
UNIT_PATH="$UNIT_DIR/$SERVICE_NAME.service"

detect_binary() {
  if [[ -n "$KHUNQUANT_BIN" ]]; then
    printf '%s\n' "$KHUNQUANT_BIN"
    return
  fi
  command -v khunquant || true
}

run_user_systemctl() {
  systemctl --user "$@"
}

status_service() {
  run_user_systemctl status "$SERVICE_NAME.service" --no-pager || true
  echo
  echo "Logs:"
  echo "  journalctl --user -u $SERVICE_NAME -f"
}

uninstall_service() {
  info "Stopping and disabling $SERVICE_NAME.service"
  run_user_systemctl disable --now "$SERVICE_NAME.service" >/dev/null 2>&1 || true
  rm -f "$UNIT_PATH"
  run_user_systemctl daemon-reload
  info "Removed $UNIT_PATH"
}

install_service() {
  local bin
  bin="$(detect_binary)"
  [[ -n "$bin" ]] || die "khunquant not found on PATH; set KHUNQUANT_BIN=/path/to/khunquant"
  [[ -x "$bin" ]] || die "khunquant binary is not executable: $bin"

  if [[ ! -d "$KHUNQUANT_HOME" ]]; then
    info "Creating $KHUNQUANT_HOME"
    mkdir -p "$KHUNQUANT_HOME"
  fi

  if [[ ! -f "$KHUNQUANT_HOME/.passphrase" ]]; then
    echo "WARNING: $KHUNQUANT_HOME/.passphrase was not found."
    echo "         Encrypted credentials may require setup with: khunquant auth encrypt"
  fi

  if [[ ! -f "$KHUNQUANT_SSH_KEY_PATH" ]]; then
    echo "WARNING: $KHUNQUANT_SSH_KEY_PATH was not found."
    echo "         Encrypted credentials may fail to decrypt until the key exists."
  fi

  mkdir -p "$UNIT_DIR"

  info "Writing $UNIT_PATH"
  cat >"$UNIT_PATH" <<EOF
[Unit]
Description=KhunQuant
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory="$HOME"
Environment="HOME=$HOME"
Environment="KHUNQUANT_HOME=$KHUNQUANT_HOME"
Environment="KHUNQUANT_SSH_KEY_PATH=$KHUNQUANT_SSH_KEY_PATH"
ExecStart="$bin" $START_ARGS
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

  run_user_systemctl daemon-reload
  run_user_systemctl enable --now "$SERVICE_NAME.service"

  if [[ "$ENABLE_LINGER" == "1" ]]; then
    if command -v loginctl >/dev/null 2>&1; then
      info "Enabling lingering so the service starts at boot before login"
      loginctl enable-linger "$USER" || echo "WARNING: loginctl enable-linger failed; try: sudo loginctl enable-linger $USER"
    else
      echo "WARNING: loginctl not found; service may require user login before starting"
    fi
  fi

  info "Installed and started $SERVICE_NAME.service"
  echo
  status_service
}

case "$ACTION" in
  install) install_service ;;
  status) status_service ;;
  uninstall) uninstall_service ;;
esac
