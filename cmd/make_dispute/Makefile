MODE?="assurances"
NETWORK?="tiny"

dispute:
	@echo "Make STF with Dispute"
	@echo "MODE: $(MODE)"
	@echo "NETWORK: $(NETWORK)"
	@echo "====================="
	@go run -tags $(NETWORK) make_disputes.go -mode $(MODE)