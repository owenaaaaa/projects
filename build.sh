#!/bin/bash
# Build script for Linux/Mac

echo "Building implant..."

go build -ldflags="-s -w" -o implant tool.go

if [ $? -eq 0 ]; then
    echo "[*] Build successful: implant"
else
    echo "[!] Build failed"
    exit 1
fi
