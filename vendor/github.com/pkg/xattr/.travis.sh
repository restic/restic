#!/bin/sh -e

echo "Building for Linux..."
GOOS=linux   go build

echo "Building for Darwin..."
GOOS=darwin  go build

echo "Building for FreeBSD..."
GOOS=freebsd go build

echo "Building for Windows...(dummy)"
GOOS=windows go build

echo "Running tests..."
go vet
go test -v -race -coverprofile=coverage.txt -covermode=atomic
