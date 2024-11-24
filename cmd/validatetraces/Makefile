TIMEOUT := 1m

testsf:
	@echo "Running TestSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$

testfb:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFallback$
