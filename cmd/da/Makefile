.PHONY: da
TAGS ?= small

da:
	@echo "Building JAM..."
	go build -tags $(TAGS) -o da da.go


network:
	@for i in $(shell seq 0 23); do \
		# echo "Starting validator index $$i"; \
		./da \
		-validatorindex $$i -port 9000 -metadata da- -hp jam- & \
	done

network_local:
	@for i in $(shell seq 0 23); do \
		# echo "Starting validator index $$i"; \
		./da \
		-validatorindex $$i -port $$((9000 + $$i)) -metadata 127.0.0.1:$$((9000 + $$i)) & \
	done

kill:
	pkill da