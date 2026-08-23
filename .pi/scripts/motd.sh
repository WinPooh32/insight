#!/usr/bin/env bash
echo "Session starting time: $(date '+%Y-%m-%d %H:%M:%S')"
echo "Current working directory: $(pwd)"
echo
echo "Go project environment:
$(go env | grep -E 'GOMOD=|GOOS=|GOVERSION=')"
echo "Go module: $(go list -m)"
