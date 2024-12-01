.PHONY: push download all lint test

all: test lint push download

dist:
	mkdir -p dist

push: dist
	GOOS=linux GOARCH=amd64 go build -o ./dist/push-linux-amd64 ./cmd/push
	go build -o ./dist/push ./cmd/push

download: dist
	GOOS=linux GOARCH=amd64 go build -o ./dist/download-linux-amd64 ./cmd/download
	go build -o ./dist/download ./cmd/download

deploy: all
	scp ./dist/push ./dist/download ubuntu:/mnt/data/ynab

lint:
	golangci-lint run --fix ./...

test:
	go test ./... -race
