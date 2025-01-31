.PHONY: da


da:
	@echo "Building JAM..."
	go build -o da da.go


network:
	@for i in $(shell seq 0 5); do \
		# echo "Starting validator index $$i"; \
		./da -network_test tiny \
		-validatorindex $$i -port $$((9000 + $$i)) -metadata 127.0.0.1:$$((9000 + $$i)) & \
	done

kill:
	pkill da