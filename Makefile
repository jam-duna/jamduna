OUTPUT_DIR := bin
BINARY := jam
SRC := jam.go

jam:
	@echo "Building JAM..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(BINARY) $(SRC)

testnet:
	make jam
	docker build -t colorfulnotion/jam .
	docker-compose up

clean:
	rm -f $(OUTPUT_DIR)/$(BINARY)

bandersnatch:
	cd crypto; cargo build --release
	@echo "Built bandersnatch library!"

beauty:
	@echo "Running go fmt on all Go files..."
	@go fmt ./...

fmt-check:
	@echo "Checking formatting..."
	@diff -u <(echo -n) <(gofmt -d .)
