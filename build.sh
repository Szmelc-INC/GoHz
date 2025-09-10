#!/bin/bash
set -euo pipefail

# build.sh - build all components
go build -o split split/main.go
echo "[+] built ./split"
sleep 1
go build -o analize analize/main.go
echo "[+] built ./analize"
