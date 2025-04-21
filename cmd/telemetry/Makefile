peerjson?= peerlist/jam.json

node_port?= 6999

build:
	go build -tags tiny -o ../../bin/telemetry main.go

run:
	../../bin/telemetry -port $(node_port)  