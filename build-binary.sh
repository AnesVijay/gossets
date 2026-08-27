#!/bin/bash

echo "🚀 Building Go binary for Linux..."
GOOS=linux GOARCH=amd64 go build -o gossets main.go

echo "✅ Build complete! Binary: ./gossets"
