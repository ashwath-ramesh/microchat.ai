#!/bin/bash

# deployment script for microchat.ai - run as microchat user
set -e

# Check if running as microchat user
if [ "$(whoami)" != "microchat" ]; then
    echo "Error: This script must be run as the microchat user"
    echo "Usage: sudo -u microchat ./deploy/deploy.sh"
    exit 1
fi

echo "building & deploying microchat.ai..."

# Update code (fast-forward only, prevents merge conflicts)
git pull --ff-only origin main

# Build server (no sudo needed - user owns the directory)
go build -o server cmd/server/*.go

# Restart systemd service if it exists
# Requires sudoers entry: microchat ALL=(ALL) NOPASSWD: /bin/systemctl restart microchat
if systemctl is-enabled microchat &>/dev/null; then
  sudo systemctl restart microchat
  echo "service restarted"
else
  echo "failed to start microchat.service. please check: /etc/systemd/system/"
fi

echo "build & deployment complete!"

