TIMEOUT := 1m


.PHONY: fallback safrole assurances

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

fallback:
	mv src/fallback/node*/data/Blocks/* fallback/blocks
	mv src/fallback/node*/data/StateSnapshots/* fallback/state_snapshots
	mv src/fallback/node*/data/Traces/* fallback/traces

safrole:
	mv src/safrole/node*/data/Blocks/* safrole/blocks
	mv src/safrole/node*/data/StateSnapshots/* safrole/state_snapshots
	mv src/safrole/node*/data/Traces/* safrole/traces

assurances:
	mv src/assurances/node*/data/Blocks/* assurances/blocks
	mv src/assurances/node*/data/StateSnapshots/* assurances/state_snapshots
	mv src/assurances/node*/data/Traces/* assurances/traces


