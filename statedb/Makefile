# Set variables for testing
TIMEOUT := 5m

stats:
	@echo "Running TestCodecStatistics with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestCodecStatistics$

safrole:
	@echo "Running TestSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$

report:
	@echo "Running TestReportVerifyTiny with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestReportVerifyTiny$
