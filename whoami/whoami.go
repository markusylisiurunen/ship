package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultPort = "8080"
)

var logger = log.New(os.Stdout, "", log.LstdFlags)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", handle())
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Printf("Server is starting on port %s", port)
		serverErrors <- server.ListenAndServe()
	}()
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErrors:
		logger.Fatalf("Error starting server: %v", err)
	case sig := <-shutdown:
		logger.Printf("Start shutdown, signal: %v", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("Graceful shutdown failed: %v", err)
			if err := server.Close(); err != nil {
				logger.Fatalf("Could not stop server: %v", err)
			}
		}
	}
}

func handle() http.HandlerFunc {
	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	id := hex.EncodeToString(idBytes)
	type response struct {
		ID       string    `json:"id"`
		Time     time.Time `json:"time"`
		Hostname string    `json:"hostname"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, err := os.Hostname()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		resp := response{
			ID:       id,
			Time:     time.Now().UTC(),
			Hostname: hostname,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Printf("Error encoding the response: %v", err)
		}
	}
}
