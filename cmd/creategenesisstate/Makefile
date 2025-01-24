TIMEOUT := 1m

.PHONY: creategenesisstate

# List of Networks
TAGS := small tiny medium large 2xlarge 3xlarge full

# Dynamic rule for creating genesis state based on tags
creategenesisstate:
	@for tag in $(TAGS); do \
		echo "Running genesis.go with tag: $$tag"; \
		start_time=$$(date +%s); \
		go run -tags=$$tag genesis.go; \
		end_time=$$(date +%s); \
		elapsed_time=$$((end_time - start_time)); \
		echo "Time taken for $$tag: $$elapsed_time seconds"; \
	done
