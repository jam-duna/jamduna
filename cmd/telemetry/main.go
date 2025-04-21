// jip3_telemetry_server_client.go

package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

type TelemetryServer struct {
	Addr string
}

func NewTelemetryServer(addr string) *TelemetryServer {
	return &TelemetryServer{Addr: addr}
}

func (s *TelemetryServer) Start() {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		log.Fatalf("Failed to bind: %v", err)
	}
	log.Printf("Telemetry server listening on %s", s.Addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *TelemetryServer) handleConn(conn net.Conn) {
	defer conn.Close()
	for {
		code := make([]byte, 1)
		if _, err := io.ReadFull(conn, code); err != nil {
			log.Println("Connection closed")
			return
		}
		// TODO: send to BigQuery + syslog
		log.Printf("Received telemetry code: %d", code[0])
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: telemetry <host:port>")
		return
	}
	addr := os.Args[1]

	srv := NewTelemetryServer(addr)
	srv.Start()
}
