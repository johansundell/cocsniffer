GHACCOUNT := johansundell
NAME := cocsniffer
VERSION := v1.1

include common.mk

deps:
	go get github.com/c4milo/github-release
	go get github.com/mitchellh/gox
	go get github.com/johansundell/cocapi

