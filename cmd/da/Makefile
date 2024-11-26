.PHONY: da


da:
	@echo "Building JAM..."
	go build -o da da.go


network:
	./da --validatorindex 0 &
	./da --validatorindex 1 &

kill:
	pkill da
