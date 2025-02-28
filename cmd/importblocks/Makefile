TIMEOUT          := 10m
BINARY_NAME      := importblocks
BINARY_NAME_FUZZ := fuzzer
JAMDIR           ?= /tmp/ntust/jam
MODE             ?= generic
DATADIR		  	 ?= rawdata
FUZZDIR			 ?= fuzzed
NETWORK          ?= tiny
LATEST_TS        := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS               ?= $(LATEST_TS)
JOBID            ?= 

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz fuzz_only

all: build

build:
	go build -tags=$(NETWORK) -o $(BINARY_NAME) .

importblocks: build
	./$(BINARY_NAME)

run:
	go build -tags=$(NETWORK) -o $(BINARY_NAME_FUZZ) ../fuzzer
	./$(BINARY_NAME_FUZZ)

cpnode:
ifeq ($(JOBID),)
	@echo "Detecting latest timestamp directory in $(JAMDIR)..."
	$(eval TS=$(shell ls -1 "$(JAMDIR)" 2>/dev/null | grep '_' | sort -n | tail -1))
	$(eval JOBID=$(shell echo $(TS) | cut -d'_' -f2))
else
	@echo "JOBID specified: $(JOBID), filtering directory in $(JAMDIR)..."
	$(eval TS=$(shell ls -1 "$(JAMDIR)" 2>/dev/null | grep '_$(JOBID)' | sort -n | tail -1))
endif

	@echo "Using directory: $(TS) with jobID: $(JOBID)"
	@echo "Removing old data from $(DATADIR)/$(MODE)/..."
	rm -rf "$(DATADIR)/$(MODE)/state_transitions" "$(DATADIR)/$(MODE)/blocks" "$(DATADIR)/$(MODE)/state_snapshots"

	mkdir -p "$(DATADIR)/$(MODE)/state_transitions" \
	         "$(DATADIR)/$(MODE)/blocks" \
	         "$(DATADIR)/$(MODE)/state_snapshots"

	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/blocks/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/state_snapshots/" \;

cpstf:
	mkdir -p ${DATADIR}/$(MODE)/state_transitions ${DATADIR}/$(MODE)/blocks ${DATADIR}/$(MODE)/state_snapshots
	find "$(JAMDIR)/state_transitions" -type f -exec cp {} "${DATADIR}/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks" -type f -exec cp {} "${DATADIR}/$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots" -type f -exec cp {} "${DATADIR}/$(MODE)/state_snapshots/" \;

fuzz_only:
	rm -rf "$(FUZZDIR)"
	mkdir -p "$(FUZZDIR)"
	(cd ../../fuzz/ && go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestFuzz$$)

fuzz: fuzz_only
	./fuzzed_zipper.sh

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ)

verify:
	go test ../../statedb -run '^TestStateTranistionForPublish$$' -tags tiny


