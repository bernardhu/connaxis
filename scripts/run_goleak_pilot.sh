#!/usr/bin/env bash
set -euo pipefail

# Run goleak on core server paths.
go test -tags=goleak -count=1 ./connection ./evhandler ./eventloop ./websocket
