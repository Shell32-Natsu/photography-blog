package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	subcmd "metadata-editor/cmd"
	"metadata-editor/config"
	"metadata-editor/handlers"
)

func main() {
	// Define the base command
	if len(os.Args) < 2 {
		fmt.Println("Usage: metadata-editor <command> [arguments]")
		fmt.Println("Available commands: server, generate, refresh")
		os.Exit(1)
	}

	// Check which command is being used
	cmd := os.Args[1]

	switch cmd {
	case "server":
		serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
		port := serverCmd.Int("port", 18080, "Port to run the server on")

		// Parse the server command flags
		serverCmd.Parse(os.Args[2:])

		// Run the server
		runServer(*port)

	case "generate":
		genCmd := flag.NewFlagSet("generate", flag.ExitOnError)
		outputFile := genCmd.String("output", "../../config.json", "Output JSON file path")

		// Parse the generate command flags
		genCmd.Parse(os.Args[2:])

		// Run the generate command
		subcmd.Generate(*outputFile)

	case "refresh":
		refreshCmd := flag.NewFlagSet("refresh", flag.ExitOnError)
		refreshCmd.Parse(os.Args[2:])
		
		// Run the refresh command to generate thumbnails
		subcmd.Refresh()

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Available commands: server, generate, refresh")
		os.Exit(1)
	}
}

// runServer starts the HTTP server with API endpoints
func runServer(port int) {
	ctx := context.Background()
	// Initialize the Cloudflare R2 client
	config.InitR2Client(ctx)

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// Set up API endpoints
	http.HandleFunc("/photos", handlers.GetPhotos)
	http.HandleFunc("/photos/metadata", handlers.UpdatePhotoMetadata)

	// Start the server
	log.Printf("Starting metadata editor API server on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
