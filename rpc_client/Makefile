mode?="fib"
isLocal?=false

run:
	@echo "Running tests with mode=$(mode)"
	@go test -run TestClient -tags "tiny network_test" -mode=$(mode) -timeout=0 -isLocal=$(isLocal)

game_of_life:
	@echo "Running tests with mode=game_of_life"
	@go test -run TestClient -tags "tiny network_test" -mode=game_of_life -isLocal=$(isLocal)

local_game_of_life:
	@echo "Running tests with mode=game_of_life and isLocal=true"
	@go test -run TestClient -tags "tiny network_test" -mode=game_of_life -isLocal=true