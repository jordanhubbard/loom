package main

import (
	"fmt"
	"log"
	"os"
)

const version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Printf("Arbiter v%s - Agentic Coding Orchestrator\n", version)
	fmt.Println("An agentic based coding orchestrator for both on-prem and off-prem development")
	fmt.Println()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("Version: %s\n", version)
		case "help", "--help", "-h":
			printHelp()
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			printHelp()
			os.Exit(1)
		}
	} else {
		fmt.Println("Starting arbiter service...")
		fmt.Println("Ready to orchestrate coding tasks")
		// Main service loop would go here
	}
}

func printHelp() {
	fmt.Println("Usage: arbiter [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version    Display version information")
	fmt.Println("  help       Display this help message")
	fmt.Println()
	fmt.Println("When run without commands, arbiter starts in service mode")
}
