TIMEOUT          := 10m
BINARY_NAME      := duna_target

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz fuzz_only

all: build

build:
	go build -tags=$(NETWORK) -o $(BINARY_NAME) .