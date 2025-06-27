package main

import (
	"fmt"
	"log"
	"os"

	"github.com/photography-blog/pkg/command"
	"github.com/photography-blog/pkg/frame"
	"github.com/photography-blog/pkg/parser"
)

func main() {
	// Parse configuration
	website, err := parser.Parse()
	if err != nil {
		log.Fatalf("Failed to parse configuration: %v", err)
	}

	// Create site structure and render pages
	if err := frame.CreatePathFromConfig(website); err != nil {
		log.Fatalf("Failed to create site structure: %v", err)
	}

	// Copy asset files
	if err := command.TryCopyFiles(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	fmt.Println("Site generated successfully!")
}