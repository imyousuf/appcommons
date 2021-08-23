SHELL = /bin/bash -o pipefail

UNAME_S := $(shell uname -s)

ARCH := $(shell arch)

OS := $(shell test -f /etc/os-release && cat /etc/os-release | grep '^NAME' | sed -e 's/NAME="\(.*\)"/\1/g')

all: clean os-deps dep-tools deps test build

apt-packages:
	sudo apt install --yes gcc-arm-linux-gnueabihf g++-arm-linux-gnueabihf gcc-aarch64-linux-gnu g++-aarch64-linux-gnu

brew-packages:

alpine-packages:
	apk add --no-cache gcc musl-dev curl

os-deps:
ifeq ($(UNAME_S),Linux)
ifeq ($(OS),Ubuntu)
os-deps: apt-packages
endif
ifeq ($(OS),LinuxMint)
os-deps: apt-packages
endif
ifeq ($(OS),Alpine Linux)
os-deps: alpine-packages
endif
endif
ifeq ($(UNAME_S),Darwin)
os-deps: brew-packages
endif

deps:
ifeq ($(OS),Alpine Linux)
	go mod download
endif
ifneq ($(OS),Alpine Linux)
	go mod vendor
endif

build-init:
	mkdir -p ./dist/

generate:

dep-tools:

time-test:
	time go test -timeout 30s -mod=readonly ./... -count=1

ci-test:
	go test -timeout 30s -mod=readonly -v ./... -short

test:
	go test -timeout 30s -mod=readonly ./...

install: build
	go install -mod=readonly

clean: build-init
	-rm -v ./testdatadir/*.cfg
	-rm -vrf ./dist/*
	-rm -v appcommons
