	# Default timeout for tests (can be overridden: `make TIMEOUT=2m test-network NETWORK=...`)
TIMEOUT      ?= 5m
LongTimeout ?= 30m

# The list of networks you want to test by default
NETWORKS     ?= tiny # use tiny for now
NETWORK	 	 ?= tiny
TESTING ?=testing
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
	@echo "Now running Go tests in statedb with '-tags $(NETWORK)'"
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run "TestSafroleVerify" -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

report:
	@echo "Running 'TestReportVerify' (plus valid report) with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run TestReportVerify  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK) && \
		go test -timeout $(TIMEOUT) -v -run TestReportValidReportCase  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

valid_report:
	@echo "Running 'TestReportValidReportCase' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		go test -timeout $(TIMEOUT) -v -run TestReportValidReportCase  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

assurance:
	@echo "Running 'TestVerifyAssurance' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run TestVerifyAssurance -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

dispute:
	@echo "Running 'TestVerifyDisputes' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run TestVerifyDisputes  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

auth:
	@echo "Running 'TestVerifyAuth' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run TestVerifyAuth  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

accumulate:
	@echo "Running 'TestAccumulate' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && \
		echo "###$(NETWORK) Case###" && \
		go test -timeout $(TIMEOUT) -v -run TestAccumulateSTF  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)


stats_codec:
	@echo "Running 'TestCodecStatistics' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run "^TestCodecStatistics$$" -tags $(NETWORK)
stats:
	@echo "Running 'TestStatistics' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run "^TestStaticsSTFVerify$$" -tags "$(NETWORK),$(TESTING)" -network=$(NETWORK)


test-network: 
	@echo "Running tests for network='$(NETWORK)'"
			go test -timeout $(TIMEOUT) -v -run TestTestVectors  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)
	@echo "Finished all tests for network='$(NETWORK)'"

trace_interpreter:
	@echo "Running 'TestTraces' with timeout $(LongTimeout) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -v -timeout $(LongTimeout) -run TestTracesInterpreter  -tags "$(NETWORK) $(TESTING) benchprofile" -network=$(NETWORK)

trace_compiler:
	@echo "Running 'TestTraceCompiler' with timeout $(LongTimeout) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -v -timeout $(LongTimeout) -run TestTracesCompiler  -tags "$(NETWORK) $(TESTING) benchprofile" -network=$(NETWORK)	

single_fuzz:
	@echo "Running 'TestSingleFuzzTrace' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run TestSingleFuzzTrace  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

publish_fuzz:
	@echo "Running 'TestPublishFuzzTrace' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run TestPublishFuzzTrace  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

all_fuzz:
	@echo "Running 'TestFuzzTrace' with timeout $(TIMEOUT) for network='$(NETWORK)' ..."
	@cd $(STATEDB_DIR) && go test -timeout $(TIMEOUT) -v -run TestFuzzTrace  -tags "$(NETWORK) $(TESTING)" -network=$(NETWORK)

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
#building for the default network bandersnatch again
	@echo "Building bandersnatch with cargo --features '$(NETWORK)'"
	@cd $(BANDERSNATCH_DIR) && cargo build --quiet --release --features "tiny"
	@cd $(STATEDB_DIR)
	@echo "Done testing all networks. bandersnatchlib is built for $(NETWORK) network"