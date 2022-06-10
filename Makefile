.PHONY: build docker

build:
	CGO_ENABLED=0 GO11MODULE=on go build ./cmd/k8s-event-tailer

GIT_REV=$(shell git rev-parse --short HEAD)

docker:
	docker build -t sandipb/k8s-event-tailer:$(GIT_REV) -t sandipb/k8s-event-tailer:latest .
