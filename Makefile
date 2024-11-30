.PHONY: push download all

all: push download

push:
	GOOS=linux GOARCH=amd64 go build ./cmd/push

download:
	GOOS=linux GOARCH=amd64 go build ./cmd/download

deploy: all
	scp push download ubuntu:/mnt/data/ynab
