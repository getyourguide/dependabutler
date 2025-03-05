#!/bin/bash
export PATH=$PATH:$(go env GOPATH)/bin
go install golang.org/x/lint/golint@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install mvdan.cc/gofumpt@latest
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
echo "🧪 go vet ./..."
go vet ./...
echo "🧪 golint -set_exit_status ./..."
golint -set_exit_status ./...
echo "🧪 staticcheck -checks all ./..."
staticcheck -checks all ./...
echo "🧪 gofumpt -d ."
gofumpt -d .
echo "🧪 gocyclo -over 18 ."
gocyclo -over 18 .
echo "🧪 go test ./..."
go test -vet=off ./...
