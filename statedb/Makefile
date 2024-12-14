# Set variables for testing
TIMEOUT := 5m


safrole:
	@echo "Running TestSafroleVerify with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafroleVerify$

report:
	@echo "Running TestReportVerifyTiny with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestReportVerifyTiny$
	
valid_report:
	@echo "Running TestReportValidReportCaseTiny with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestReportValidReportCaseTiny$

assurance:
	@echo "Running TestVerifyAssuranceTiny with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestVerifyAssuranceTiny$

dispute:
	@echo "Running TestVerifyDisputesTiny with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestVerifyDisputesTiny$

stats:
	@echo "Running TestCodecStatistics with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestCodecStatistics$