# Default timeout for tests (can be overridden: `make TIMEOUT=2m test-network NETWORK=...`)
TIMEOUT      ?= 5m

# The list of networks you want to test by default
NETWORKS     ?= tiny full
NETWORK	 	 ?= tiny

# Bandersnatch and statedb directories (adjust if needed)
BANDERSNATCH_DIR := ../bandersnatch
STATEDB_DIR      := ../statedb

###############################################################################
# PHONY Targets
###############################################################################
.PHONY: safrole report valid_report assurance dispute stats \
        test-network test-all

###############################################################################
# Individual Test Targets
# - Each target uses $(NETWORK) to decide which build tag to use and which cargo feature to enable.
# - If you want more fine-grained logic (e.g., cargo build differently for each network),
#   adjust accordingly in each target.
###############################################################################

safrole:
	@echo "Running 'TestSafroleVerify' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@echo "Building bandersnatch with cargo --features '$(NETWORK)'"
	@cd $(BANDERSNATCH_DIR) && cargo build --quiet --release --features "$(NETWORK)"

	@echo "Now running Go tests in statedb with '-tags $(NETWORK)'"
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run "TestSafroleVerify$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')" -tags $(NETWORK)

report:
	@echo "Running 'TestReportVerify' (plus valid report) with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run "TestReportVerify$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')" -tags $(NETWORK) && \
		go test -timeout $(TIMEOUT) -v -run "TestReportValidReportCase$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')" -tags $(NETWORK)

valid_report:
	@echo "Running 'TestReportValidReportCase' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		go test -timeout $(TIMEOUT) -v -run "^TestReportValidReportCase$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')$$" -tags $(NETWORK)

assurance:
	@echo "Running 'TestVerifyAssurance' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run "TestVerifyAssurance$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')" -tags $(NETWORK)

dispute:
	@echo "Running 'TestVerifyDisputes' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run "TestVerifyDisputes$(shell echo $(NETWORK) | awk '{print toupper(substr($$0,1,1)) substr($$0,2)}')" -tags $(NETWORK)

stats:
	@echo "Running 'TestCodecStatistics' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run "^TestCodecStatistics$$" -tags $(NETWORK)


test-network: safrole report valid_report assurance dispute stats
	@echo "Finished all tests for network='$(NETWORK)'"

tiny:
	$(MAKE) test-network NETWORK=tiny;

full: 
	$(MAKE) test-network NETWORK=full;

test:
	@echo "Running ALL tests for all networks: $(NETWORKS)"
	@for net in $(NETWORKS); do \
		echo "--------------------------------------------------"; \
		echo "TESTING NETWORK: $$net"; \
		$(MAKE) test-network NETWORK=$$net; \
	done
	@echo "Done testing all networks."