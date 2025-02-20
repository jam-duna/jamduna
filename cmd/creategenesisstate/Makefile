TIMEOUT := 2m
BANDERSNATCH_DIR :=  ../../bandersnatch
GENESIS_DIR := ../cmd/creategenesisstate/

.PHONY: creategenesisstate
# List of Networks
TAGS := small medium large 2xlarge 3xlarge full tiny

# Dynamic rule for creating genesis state based on tags
creategenesisstate:
	@for tag in $(TAGS); do \
		echo "Running genesis.go with tag: $$tag"; \
		start_time=$$(date +%s); \
		export GOFLAGS=-tags=$$tag; \
		cd $(BANDERSNATCH_DIR) && cargo clean && cargo build --quiet --release --features $$tag; \
		cd $(GENESIS_DIR) && go run genesis.go; \
		end_time=$$(date +%s); \
		elapsed_time=$$((end_time - start_time)); \
		echo "Time taken for $$tag: $$elapsed_time seconds"; \
	done;
	@echo "Setting tag back to tiny"
	@export GOFLAGS=-tags=tiny; \
	cd $(BANDERSNATCH_DIR) && cargo build --quiet --release --features tiny;
