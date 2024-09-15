package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
)

func main() {
	serverURL := "ws://localhost:8080/ws"
	u, err := url.Parse(serverURL)
	if err != nil {
		log.Fatal("URL parse error:", err)
	}

	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	defer conn.Close()

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		log.Fatal("Key generation error:", err)
	}
	secWebSocketKey := base64.StdEncoding.EncodeToString(key)

	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\n", u.RequestURI())
	fmt.Fprintf(conn, "Host: %s\r\n", u.Host)
	fmt.Fprintf(conn, "Upgrade: websocket\r\n")
	fmt.Fprintf(conn, "Connection: Upgrade\r\n")
	fmt.Fprintf(conn, "Sec-WebSocket-Key: %s\r\n", secWebSocketKey)
	fmt.Fprintf(conn, "Sec-WebSocket-Version: 13\r\n")
	fmt.Fprintf(conn, "Origin: http://localhost:8080\r\n")
	fmt.Fprintf(conn, "\r\n")

	reader := bufio.NewReader(conn)

	status, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal("Error reading status line:", err)
	}
	if !strings.Contains(status, "101") {
		log.Fatal("Did not receive 101 Switching Protocols")
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal("Error reading headers:", err)
		}
		if line == "\r\n" {
			break
		}
	}

	log.Println("Connected to server")

	message, err := readTextMessage(reader)
	if err != nil {
		log.Fatal("Error reading message:", err)
	}

	fmt.Printf("Received from server: %s\n", message)
}

func readTextMessage(r *bufio.Reader) (string, error) {
	header := make([]byte, 2)
	if _, err := r.Read(header); err != nil {
		return "", err
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	payloadLen := int(header[1] & 0x7F)

	if !fin {
		return "", fmt.Errorf("Continuation frames are not supported")
	}
	if opcode != 0x1 {
		return "", fmt.Errorf("Only text frames are supported")
	}
	if masked {
		return "", fmt.Errorf("Server frames should not be masked")
	}

	payload := make([]byte, payloadLen)
	if _, err := r.Read(payload); err != nil {
		return "", err
	}

	return string(payload), nil
}
