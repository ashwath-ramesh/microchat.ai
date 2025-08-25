package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type chatRequest struct {
	SessionID string `json:"sid,omitempty"`
	Model     string `json:"m"`
	Message   string `json:"msg"`
}

type chatReply struct {
	SessionID string `json:"sid,omitempty"`
	Reply     string `json:"r"`
	Error     string `json:"e"`
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	// set response header to json
	w.Header().Set("Content-Type", "application/json")

	// only allow POST Request
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// parse json requests
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadGateway)
		return
	}
	fmt.Printf("Received: %s, %s, %s", req.SessionID, req.Model, req.Message)

	// create the reply
	resp := chatReply{
		SessionID: req.SessionID,
		Reply:     req.Message, // TODO: replace this. For now echo back request
		Error:     "",
	}

	// send json response
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func main() {
	// register the handler
	http.HandleFunc("/chat", chatHandler)

	fmt.Println("server starting on: 8080 ...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
