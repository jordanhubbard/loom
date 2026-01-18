package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jordanhubbard/arbiter/internal/api"
	"github.com/jordanhubbard/arbiter/internal/storage"
)

//go:embed web/*
var webFiles embed.FS

func main() {
	// Create storage
	store := storage.New()

	// Create API handler
	handler := api.NewHandler(store)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/work", handler.ListWork)
	mux.HandleFunc("/api/work/create", handler.CreateWork)
	mux.HandleFunc("/api/agents", handler.ListAgents)
	mux.HandleFunc("/api/services", handler.ListServices)
	mux.HandleFunc("/api/services/preferred", handler.GetPreferredServices)
	
	// Service-specific routes (handle both costs and usage)
	mux.HandleFunc("/api/services/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > len("/api/services/") {
			// Determine which handler to use based on the path suffix
			if strings.Contains(path, "/costs") {
				if r.Method == http.MethodGet {
					handler.GetServiceCosts(w, r)
				} else if r.Method == http.MethodPut {
					handler.UpdateServiceCosts(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			} else if strings.Contains(path, "/usage") {
				handler.SimulateUsage(w, r)
			} else {
				http.Error(w, "Not found", http.StatusNotFound)
			}
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	})

	// Serve static web UI
	webFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting arbiter service on %s", addr)
	log.Printf("Web UI: http://localhost%s", addr)
	log.Printf("API: http://localhost%s/api", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
