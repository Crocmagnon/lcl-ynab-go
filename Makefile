.PHONY: push download all lint test

all: test lint push download

push:
	GOOS=linux GOARCH=amd64 go build ./cmd/push

download:
	GOOS=linux GOARCH=amd64 go build ./cmd/download

deploy: all
	scp push download ubuntu:/mnt/data/ynab

lint:
	golangci-lint run --fix ./...

test:
	go test ./... -race
