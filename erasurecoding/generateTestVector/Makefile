.PHONY: da
TAGS ?= full

gen:
	@echo "Building JAM..."
	go build -tags $(TAGS) -o generateTestVector generateTestVector.go; ./generateTestVector