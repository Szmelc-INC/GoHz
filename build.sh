#!/bin/bash
set -euo pipefail

# build.sh - build all components
go build -o split/split ./split
echo "[+] built ./split/split"
sleep 1
go build -o analize/analize ./analize
echo "[+] built ./analize/analize"
