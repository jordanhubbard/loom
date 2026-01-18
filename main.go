package main

import (
	"fmt"
	"log"

	"github.com/jordanhubbard/arbiter/pkg/config"
	"github.com/jordanhubbard/arbiter/pkg/server"
)

func main() {
	fmt.Println("Welcome to Arbiter - AI Coding Agent Orchestrator")
	fmt.Println("==================================================")

	// Load or create default configuration
	cfg := config.DefaultConfig()
	
	fmt.Println("\nArbiter Worker System initialized")
	fmt.Println("See docs/WORKER_SYSTEM.md for usage information")

	// Start the server
	fmt.Println("\nStarting Arbiter server...")
	srv := server.NewServer(cfg)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
