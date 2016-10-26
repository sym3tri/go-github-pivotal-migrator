#!/bin/bash -e

export GOPATH=${PWD}/gopath

go get \
        github.com/coreos/pkg/flagutil \
        github.com/google/go-github/github \
        github.com/sym3tri/go-pivotaltracker/v5/pivotal \
        golang.org/x/oauth2

go run ./main.go $@
