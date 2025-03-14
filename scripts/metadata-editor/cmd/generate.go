package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"mime"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"metadata-editor/config"
)

// BlogConfig represents the structure of the blog config
type BlogConfig struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Author      string        `json:"author"`
	Children    []AlbumConfig `json:"children"`
}

// AlbumConfig represents an album in the blog
type AlbumConfig struct {
	Title    string       `json:"title"`
	URL      string       `json:"url"`
	Children []PhotoEntry `json:"children"`
}

// PhotoEntry represents a photo in an album
type PhotoEntry struct {
	URL string `json:"url"`
}

// Generate extracts all tags from the photos and generates a JSON mapping
func Generate(outputFile string) {
	configPath := "../../config.json"
	log.Println("Generating tags to image mapping...")
	ctx := context.Background()
	// Initialize the Cloudflare R2 client
	config.InitR2Client(ctx)

	log.Println("Extracting tags from photos...")

	// List all objects in the bucket
	listObjectsOutput, err := config.R2Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(config.BucketName),
	})
	if err != nil {
		log.Fatalf("Failed to list objects: %v", err)
	}

	// Map to store tags and their associated images
	tagsMap := make(map[string][]string)
	dec := new(mime.WordDecoder)

	// Process each object
	for _, obj := range listObjectsOutput.Contents {
		// Get object metadata
		headOutput, err := config.R2Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(config.BucketName),
			Key:    obj.Key,
		})
		if err != nil {
			log.Printf("Failed to get metadata for object %s: %v", *obj.Key, err)
			continue
		}

		// Create public URL for the image
		url := fmt.Sprintf("https://photo-r2.xiadong.info/%s", *obj.Key)

		// Extract and process tags from metadata
		if tagValue, ok := headOutput.Metadata["tag"]; ok {
			decodedString := ""
			if !strings.HasPrefix(tagValue, "=?utf-8?") {
				decodedString = tagValue
			} else {
				for _, seg := range strings.Split(tagValue, " ") {
					s, err := dec.Decode(seg)
					if err != nil {
						log.Printf("Failed to decode metadata key (%s) on %s: %v", tagValue, *obj.Key, err)
						continue
					}
					decodedString += s
				}
			}

			// Split tags and add URL to each tag's list
			for _, tag := range strings.Split(decodedString, ",") {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}

				// Add this URL to the tag's list
				tagsMap[tag] = append(tagsMap[tag], url)
			}
		}
	}

	// Read the existing config file
	existingConfig := BlogConfig{}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Could not read existing config file: %v", err)
		log.Println("Creating new config file")
		// Create default config if reading fails
		existingConfig = BlogConfig{
			Title:       "Photography Collection",
			Description: "Photo albums by tag",
			Author:      "Generated by metadata-editor",
			Children:    []AlbumConfig{},
		}
	} else {
		// Parse the existing config
		err = json.Unmarshal(configData, &existingConfig)
		if err != nil {
			log.Fatalf("Failed to parse existing config file: %v", err)
		}
		log.Printf("Successfully read existing config with %d albums", len(existingConfig.Children))
	}

	// Prepare the output data, preserving existing config metadata
	blogConfig := BlogConfig{
		Title:       existingConfig.Title,
		Description: existingConfig.Description,
		Author:      existingConfig.Author,
		Children:    []AlbumConfig{}, // Will be replaced with tag-based albums
	}

	// Create an album for each tag
	for tag, urls := range tagsMap {
		if len(urls) == 0 {
			continue
		}

		// Create photo entries for this album
		photoEntries := []PhotoEntry{}
		for _, url := range urls {
			photoEntries = append(photoEntries, PhotoEntry{
				URL: url,
			})
		}

		// Create the album
		album := AlbumConfig{
			Title:    tag,                        // Use the tag as the album title
			URL:      urls[rand.IntN(len(urls))], // Use the first photo as the album cover
			Children: photoEntries,
		}

		// Add the album to the blog config
		blogConfig.Children = append(blogConfig.Children, album)
	}
	sort.Slice(blogConfig.Children, func(i, j int) bool {
		return blogConfig.Children[i].Title < blogConfig.Children[j].Title
	})

	// Convert to JSON
	jsonData, err := json.MarshalIndent(blogConfig, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal blog config to JSON: %v", err)
	}

	// Write to file
	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		log.Fatalf("Failed to write JSON to file: %v", err)
	}

	log.Printf("Successfully generated blog config to %s", outputFile)
	log.Printf("Created %d albums from %d photos, preserving original config metadata",
		len(blogConfig.Children), len(listObjectsOutput.Contents))
}
