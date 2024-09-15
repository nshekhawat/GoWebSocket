package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
)

const magicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func computeAcceptKey(secWebSocketKey string) string {
	const magicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(secWebSocketKey + magicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func wsHandler(w http.ResponseWriter, r *http.Request) {

	allowedOrigin := "http://localhost:8080" // Change this to your allowed origin
	origin := r.Header.Get("Origin")
	if origin != allowedOrigin {
		log.Printf("Origin not allowed: %q\n", origin)
		http.Error(w, "Origin not allowed", http.StatusForbidden)
		return
	}

	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Not a valid WebSocket handshake", http.StatusBadRequest)
		return
	}

	secWebSocketKey := r.Header.Get("Sec-WebSocket-Key")
	if secWebSocketKey == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	secWebSocketAccept := computeAcceptKey(secWebSocketKey)

	header := w.Header()
	header.Set("Upgrade", "websocket")
	header.Set("Connection", "Upgrade")
	header.Set("Sec-WebSocket-Accept", secWebSocketAccept)
	w.WriteHeader(http.StatusSwitchingProtocols)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Could not hijack connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	message := "Hello World"
	if err := sendTextMessage(rw.Writer, message); err != nil {
		log.Println("Error sending message:", err)
		return
	}
	log.Println("Sent:", message)
}

func sendTextMessage(w *bufio.Writer, message string) error {
	payloadLen := len(message)
	if payloadLen > 125 {
		return fmt.Errorf("Message too long")
	}

	frame := []byte{0x81}
	frame = append(frame, byte(payloadLen))
	frame = append(frame, []byte(message)...)

	if _, err := w.Write(frame); err != nil {
		return err
	}
	return w.Flush()
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	fmt.Println("WebSocket server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
