package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Test the readTextMessage function with various WebSocket frame scenarios
func TestReadTextMessage(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		want      string
		wantError bool
	}{
		{
			name: "Valid text frame",
			input: []byte{
				0x81, 0x05, 'H', 'e', 'l', 'l', 'o', // Text frame with "Hello"
			},
			want: "Hello",
		},
		{
			name: "Continuation frame",
			input: []byte{
				0x00, 0x05, 'H', 'e', 'l', 'l', 'o', // Continuation frame
			},
			wantError: true,
		},
		{
			name: "Binary frame",
			input: []byte{
				0x82, 0x05, 'H', 'e', 'l', 'l', 'o', // Binary frame
			},
			wantError: true,
		},
		{
			name: "Masked frame",
			input: []byte{
				0x81, 0x80, 0x00, 0x00, 0x00, 0x00, // Masked text frame
			},
			wantError: true,
		},
		{
			name: "Empty payload",
			input: []byte{
				0x81, 0x00, // Text frame with empty payload
			},
			want: "",
		},
		{
			name: "Payload length 126",
			input: []byte{
				0x81, 0x7E, 0x00, 0x7E, // Text frame with length 126
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(bytes.NewReader(tt.input))
			msg, err := readTextMessage(reader)
			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if msg != tt.want {
				t.Errorf("Got message %q, want %q", msg, tt.want)
			}
		})
	}
}

// Test the full client workflow with a mock WebSocket server
func TestClientIntegration(t *testing.T) {
	// Create a mock server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to create listener:", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// Start the mock server in a goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			// Read client handshake headers
			reader := bufio.NewReader(conn)
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}

			// Send handshake response
			conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n"))
			conn.Write([]byte("Upgrade: websocket\r\n"))
			conn.Write([]byte("Connection: Upgrade\r\n"))
			conn.Write([]byte("Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n"))
			conn.Write([]byte("\r\n"))

			// Send a text message frame
			message := "Test Message"
			frame := []byte{0x81, byte(len(message))}
			frame = append(frame, []byte(message)...)
			conn.Write(frame)
		}
	}()

	// Modify the serverURL in main to use the test server address
	// This requires refactoring main() to accept a parameter, which we can't do
	// Instead, we'll modify the test to run against the mock server address
	// This requires changing the code, but since we can't modify the original code,
	// we'll use a workaround by creating a test main function

	// Create a test main function that uses the mock server address
	testMain := func() {
		serverURL := "ws://" + serverAddr + "/ws"
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

		// Create JSON structure to hold the received message
		msg := struct {
			Content string `json:"message"`
		}{
			Content: message,
		}

		// Marshal the message into JSON format with indentation
		jsonData, err := json.MarshalIndent(msg, "", "  ")
		if err != nil {
			log.Fatal("JSON marshaling error:", err)
		}

		// Write the JSON to a file
		if err := os.WriteFile("received_message.json", jsonData, 0644); err != nil {
			log.Fatal("Error writing to file:", err)
		}

		log.Println("Received message saved to received_message.json")
	}

	// Run the test main function
	go testMain()

	// Wait for the client to complete
	time.Sleep(1 * time.Second)

	// Check if the JSON file was created
	content, err := os.ReadFile("received_message.json")
	if err != nil {
		t.Fatal("Failed to read JSON file:", err)
	}

	var msg struct {
		Content string `json:"message"`
	}
	if err := json.Unmarshal(content, &msg); err != nil {
		t.Fatal("Failed to parse JSON:", err)
	}

	if msg.Content != "Test Message" {
		t.Errorf("Got message %q, want %q", msg.Content, "Test Message")
	}

	// Cleanup
	os.Remove("received_message.json")
}

// Test error handling when the server sends an invalid status line
func TestInvalidStatusLine(t *testing.T) {
	// Create a mock server that sends an invalid status line
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to create listener:", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// Start the mock server in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Send invalid status line
		conn.Write([]byte("HTTP/1.1 200 OK\r\n")) // Not 101 Switching Protocols
	}()

	// Create a test main function that uses the mock server address
	testMain := func() {
		serverURL := "ws://" + serverAddr + "/ws"
		u, err := url.Parse(serverURL)
		if err != nil {
			log.Fatal("URL parse error:", err)
		}

		conn, err := net.Dial("tcp", u.Host)
		if err != nil {
			log.Fatal("Dial error:", err)
		}
		defer conn.Close()

		// Send handshake headers
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
	}

	// Run the test main function and capture the expected error
	// This requires redirecting stderr or recovering from panic
	// For simplicity, we'll just run it and expect it to fail
	// In a real test, we would capture the output to verify the error message
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected test to fail due to invalid status line")
		}
	}()

	testMain()
}

// Test error handling when the server sends invalid headers
func TestInvalidHeaders(t *testing.T) {
	// Create a mock server that sends invalid headers
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to create listener:", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// Start the mock server in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Send handshake response with missing empty line
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n"))
		conn.Write([]byte("Upgrade: websocket\r\n"))
		conn.Write([]byte("Connection: Upgrade\r\n"))
		// Missing empty line
	}()

	// Create a test main function that uses the mock server address
	testMain := func() {
		serverURL := "ws://" + serverAddr + "/ws"
		u, err := url.Parse(serverURL)
		if err != nil {
			log.Fatal("URL parse error:", err)
		}

		conn, err := net.Dial("tcp", u.Host)
		if err != nil {
			log.Fatal("Dial error:", err)
		}
		defer conn.Close()

		// Send handshake headers
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
	}

	// Run the test main function and capture the expected error
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected test to fail due to invalid headers")
		}
	}()

	testMain()
}
