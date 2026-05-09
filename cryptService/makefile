BINARY_NAME=cryptService
VERSION?=1.0.0
BUILD_DIR=build

.PHONY: all build clean

all: build

build:
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY_NAME) main.go

clean:
	rm $(BINARY_NAME)