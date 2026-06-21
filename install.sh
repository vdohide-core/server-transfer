#!/bin/bash

# Server Transfer Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/vdohide-core/server-transfer/main/install.sh | sudo -E bash -s -- [OPTIONS]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

WORKER_COUNT=1
UNINSTALL=false
MONGODB_URI=""
STORAGE_ID=""
STORAGE_PATH="/home/files"
PORT=8085

APP_NAME="server-transfer"
APP_DIR="/opt/$APP_NAME"
SERVICE_NAME="server-transfer"
GITHUB_REPO="vdohide-core/server-transfer"
RELEASES_URL="https://github.com/$GITHUB_REPO/releases/latest/download"

print_status()  { echo -e "${GREEN}[INFO]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

while [[ $# -gt 0 ]]; do
    case $1 in
        --uninstall)       UNINSTALL=true; shift ;;
        --count|-w|-n)     WORKER_COUNT="$2"; shift 2 ;;
        --mongodb-uri)     MONGODB_URI="$2"; shift 2 ;;
        --storage-id)      STORAGE_ID="$2"; shift 2 ;;
        --storage-path)    STORAGE_PATH="$2"; shift 2 ;;
        --port)            PORT="$2"; shift 2 ;;
        -h|--help)
            echo "Server Transfer Installer"
            echo ""
            echo "Usage: curl -fsSL https://raw.githubusercontent.com/$GITHUB_REPO/main/install.sh | sudo -E bash -s -- [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --uninstall          Uninstall completely"
            echo "  --count NUM          Number of worker instances (default: 1)"
            echo "  -w, -n NUM          Alias for --count"
            echo "  --mongodb-uri URI    MongoDB connection string"
            echo "  --storage-id ID      Storage ID (required — same as server-storage)"
            echo "  --storage-path DIR   Storage path (default: /home/files)"
            echo "  --port PORT          Health check port (default: 8084)"
            echo "  -h, --help           Show this help"
            echo ""
            echo "Examples:"
            echo "  curl -fsSL https://raw.githubusercontent.com/$GITHUB_REPO/main/install.sh | sudo -E bash -s -- \\"
            echo "      --mongodb-uri \"mongodb+srv://...\" \\"
            echo "      --storage-id \"uuid\" \\"
            echo "      --storage-path /home/files \\"
            echo "      -n 2"
            exit 0 ;;
        *)
            print_error "Unknown option: $1"; exit 1 ;;
    esac
done

if [ "$UNINSTALL" = true ]; then
    print_warning "Starting uninstallation..."
    for i in $(seq 1 20); do
        systemctl stop "${SERVICE_NAME}@${i}"    2>/dev/null || true
        systemctl disable "${SERVICE_NAME}@${i}" 2>/dev/null || true
    done
    systemctl stop "${SERVICE_NAME}"    2>/dev/null || true
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    [ -f "/etc/systemd/system/${SERVICE_NAME}@.service" ] && rm "/etc/systemd/system/${SERVICE_NAME}@.service"
    [ -f "/etc/systemd/system/${SERVICE_NAME}.service"  ] && rm "/etc/systemd/system/${SERVICE_NAME}.service"
    systemctl daemon-reload
    [ -d "$APP_DIR" ] && rm -rf "$APP_DIR"
    print_status "Uninstalled successfully!"
    exit 0
fi

if [ "$(id -u)" -ne 0 ]; then
    print_error "This script must be run as root (use sudo)"
    exit 1
fi

if [ -z "$STORAGE_ID" ]; then
    print_error "--storage-id is required (must match server-storage)"
    exit 1
fi

print_status "Starting installation... (Workers: $WORKER_COUNT)"

print_status "Installing curl..."
if command -v apt-get &>/dev/null; then
    apt-get update -qq
    apt-get install -y -qq curl
elif command -v yum &>/dev/null; then
    yum install -y curl
elif command -v dnf &>/dev/null; then
    dnf install -y curl
fi

if ! command -v curl &>/dev/null; then
    print_error "curl not found. Please install it manually."
    exit 1
fi

print_status "Stopping existing services..."
systemctl stop ${SERVICE_NAME}@* 2>/dev/null || true
systemctl stop ${SERVICE_NAME}   2>/dev/null || true

print_status "Creating app directory: $APP_DIR"
mkdir -p "$APP_DIR"
cd "$APP_DIR"

ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    BINARY="linux"
elif [ "$ARCH" = "aarch64" ]; then
    BINARY="linux-arm64"
else
    print_error "Unsupported architecture: $ARCH"
    exit 1
fi

print_status "Downloading binary ($BINARY) from latest release..."
curl -fsSL "$RELEASES_URL/$BINARY" -o "$APP_DIR/$APP_NAME"
chmod +x "$APP_DIR/$APP_NAME"
print_status "Binary downloaded."

print_status "Creating .env file..."
cat > "$APP_DIR/.env" <<EOF
MONGODB_URI=$MONGODB_URI
STORAGE_ID=$STORAGE_ID
STORAGE_PATH=$STORAGE_PATH
PORT=$PORT
LOG_PATH=$APP_DIR/logs/server-transfer.log
EOF

mkdir -p "$APP_DIR/logs"

print_status "Creating systemd service template..."
cat > /etc/systemd/system/${SERVICE_NAME}@.service <<EOF
[Unit]
Description=Server Transfer Worker %i
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/$APP_NAME
Restart=always
RestartSec=5
EnvironmentFile=$APP_DIR/.env
Environment="WORKER_ID=transfer_$(hostname)@%i"

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
print_status "Starting $WORKER_COUNT worker(s)..."
for i in $(seq 1 $WORKER_COUNT); do
    systemctl enable ${SERVICE_NAME}@$i
    systemctl start  ${SERVICE_NAME}@$i
    sleep 0.3
done

sleep 2
RUNNING=0
for i in $(seq 1 $WORKER_COUNT); do
    systemctl is-active --quiet ${SERVICE_NAME}@$i && RUNNING=$((RUNNING+1))
done

echo ""
echo "============================================"
if [ $RUNNING -eq $WORKER_COUNT ]; then
    print_status "Installation completed successfully!"
else
    print_warning "$RUNNING of $WORKER_COUNT workers running — check logs below"
    journalctl -u "${SERVICE_NAME}@1" -n 15 --no-pager
fi
echo "============================================"
echo ""
echo "  Directory:  $APP_DIR"
echo "  Workers:    $RUNNING / $WORKER_COUNT running"
echo "  Health:     http://localhost:$PORT/health"
echo ""
echo "  Enable worker in MongoDB:"
echo "    settings.transfer_enabled = true"
echo ""
echo "  Commands:"
echo "    View logs:   journalctl -u \"${SERVICE_NAME}@*\" -f"
echo "    Worker 1:    journalctl -u \"${SERVICE_NAME}@1\" -f"
echo "    Restart all: for i in \$(seq 1 $WORKER_COUNT); do systemctl restart ${SERVICE_NAME}@\$i; done"
echo "    Stop all:    for i in \$(seq 1 $WORKER_COUNT); do systemctl stop ${SERVICE_NAME}@\$i; done"
echo "    Uninstall:   curl -fsSL https://raw.githubusercontent.com/$GITHUB_REPO/main/install.sh | sudo bash -s -- --uninstall"
echo "============================================"
