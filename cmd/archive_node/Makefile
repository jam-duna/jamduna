peerjson?= peerlist/jam.json

node_port?= 13370
block_viewer_port?= 13371
jamweb_port?= 13372

build:
	go build -tags tiny -o ../../bin/archive_node main.go

run:
	../../bin/archive_node -peer $(peerjson) -port $(node_port) -graph_server_port $(block_viewer_port) -webserverport $(jamweb_port)