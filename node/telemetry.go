package node

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"net"
	"time"
)

type TelemetryClient struct {
	Conn     net.Conn
	SenderID ed25519.PublicKey
	queue    chan []byte
	done     chan struct{}
}

func NewTelemetryClient(addr string, senderID ed25519.PublicKey) (*TelemetryClient, error) {
	client := &TelemetryClient{
		SenderID: senderID,
		queue:    make(chan []byte, 100),
		done:     make(chan struct{}),
	}
	go client.run(addr)
	go client.senderLoop()
	return client, nil
}

func (c *TelemetryClient) run(addr string) {
	for {
		select {
		case <-c.done:
			log.Println("Stopping telemetry client reconnect loop")
			return
		default:
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Printf("Connection error: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			c.Conn = conn
			log.Printf("Connected to telemetry server at %s", addr)

			// Block until error or closed
			buf := make([]byte, 1)
			if _, err := conn.Read(buf); err != nil {
				log.Printf("Connection dropped: %v", err)
				conn.Close()
			}
		}
	}
}

func (c *TelemetryClient) SendMessage(code uint8, msg []byte) error {
	select {
	case c.queue <- msg:
		return nil
	default:
		log.Printf("SendMessage queue full, dropping message")
		return fmt.Errorf("send queue full")
	}
}

func (c *TelemetryClient) senderLoop() {
	for {
		select {
		case msg := <-c.queue:
			if c.Conn != nil {
				_, err := c.Conn.Write(msg)
				if err != nil {
					log.Printf("Sender loop write error: %v", err)
				}
			} else {
				log.Printf("Sender loop: no active connection")
			}
		case <-c.done:
			log.Println("Sender loop exiting")
			return
		}
	}
}

func (c *TelemetryClient) Close() {
	close(c.done)
	if c.Conn != nil {
		c.Conn.Close()
	}
	close(c.queue)
}
