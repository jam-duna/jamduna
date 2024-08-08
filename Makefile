bandersnatch:
	cd crypto; cargo build --release
	@echo "Built bandersnatch library!"

	# Other existing targets...

# Add this target to format all Go files in the repository
beauty:
	@echo "Running go fmt on all Go files..."
	@go fmt ./...

# If you want to include it in your default target, you can add it here
all: fmt
	@echo "Building your project..."
	# Add your build commands here

# You can also add a target to check for go fmt issues without applying them
fmt-check:
	@echo "Checking formatting..."
	@diff -u <(echo -n) <(gofmt -d .)
