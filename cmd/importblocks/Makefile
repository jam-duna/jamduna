TIMEOUT := 1m

.PHONY: importblocks fallback safrole assurances

importblocks:
	@echo "Building importblocks..."
	go build -o importblocks importblocks.go

testfallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFallback$

testsafrole:
	@echo "Running TestSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$

testassurances:
	@echo "Running TestAssurances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestAssurances

clean:
		git rm fallback/*
		git rm safrole/*
		git rm assurances/*

TS=4888955
MODE=assurances

# Targets
cpmodedata:
	mkdir -p $(MODE)/state_transitions/
	mkdir -p $(MODE)/blocks/
	mkdir -p $(MODE)/state_snapshots/
	find /tmp/root/jam/$(TS)/node*/data/state_transitions/ -type f -exec cp {} $(MODE)/state_transitions/ \;
	find /tmp/root/jam/$(TS)/node*/data/Blocks/ -type f -exec cp {} $(MODE)/blocks/ \;
	find /tmp/root/jam/$(TS)/node*/data/StateSnapshots/ -type f -exec cp {} $(MODE)/state_snapshots/ \;


fuzz:
	@echo "Running Fuzzing with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestMKFuzz