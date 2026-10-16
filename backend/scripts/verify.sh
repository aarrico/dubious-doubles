#!/bin/sh
set -eu
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  printf 'Run gofmt on:\n%s\n' "$unformatted" >&2
  exit 1
fi
go vet ./...
go test -count=1 ./...
