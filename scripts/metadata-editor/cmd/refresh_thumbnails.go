package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"metadata-editor/config"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"
)

// Refresh regenerates thumbnails for photos in the main bucket
func Refresh() {
	ctx := context.Background()
	// Initialize the Cloudflare R2 client
	config.InitR2Client(ctx)

	log.Println("Reading all photos...")

	objKeys := getObjKeys(ctx, config.BucketName)
	log.Printf("Found %d photos in main bucket.", len(objKeys))

	thumbnailObjKeys := getObjKeys(ctx, config.ThumbnailsBucketName)
	log.Printf("Found %d photos in thumbnail bucket.", len(thumbnailObjKeys))

	thumbnailsToCreate := []string{}

	// Compare the two sets of keys
	for key := range objKeys {
		if _, ok := thumbnailObjKeys[key]; !ok {
			thumbnailsToCreate = append(thumbnailsToCreate, key)
		}
	}
	log.Printf("Found %d thumbnails to create.", len(thumbnailsToCreate))

	// Generate thumbnails concurrently with a worker pool
	generateThumbnails(ctx, thumbnailsToCreate)
}

// getObjKeys retrieves all object keys from a bucket
func getObjKeys(ctx context.Context, bucket string) map[string]bool {
	res := make(map[string]bool)
	// List all objects in the bucket
	listObjectsOutput, err := config.R2Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		log.Fatalf("Failed to list objects: %v", err)
	}

	for _, obj := range listObjectsOutput.Contents {
		res[*obj.Key] = true
	}
	return res
}

// generateThumbnails processes the list of images and creates thumbnails
func generateThumbnails(ctx context.Context, keys []string) {
	// Set up a worker pool for concurrent processing
	concurrency := 5 // Number of workers
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)

	// Track success and failure counts
	var successCount, failureCount int
	var countMutex sync.Mutex

	log.Printf("Starting thumbnail generation with %d workers...", concurrency)

	for _, key := range keys {
		// Skip non-image files
		if !isImageFile(key) {
			log.Printf("Skipping non-image file: %s", key)
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(objKey string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			err := processThumbnail(ctx, objKey)

			countMutex.Lock()
			if err != nil {
				log.Printf("Error generating thumbnail for %s: %v", objKey, err)
				failureCount++
			} else {
				successCount++
				if successCount%10 == 0 {
					log.Printf("Progress: %d thumbnails generated", successCount)
				}
			}
			countMutex.Unlock()
		}(key)
	}

	wg.Wait()
	log.Printf("Thumbnail generation complete. Success: %d, Failures: %d", successCount, failureCount)
}

// processThumbnail generates a thumbnail for a single image
func processThumbnail(ctx context.Context, key string) error {
	// Get the original image from the main bucket
	getObjInput := &s3.GetObjectInput{
		Bucket: aws.String(config.BucketName),
		Key:    aws.String(key),
	}

	getObjOutput, err := config.R2Client.GetObject(ctx, getObjInput)
	if err != nil {
		return fmt.Errorf("failed to get original image: %w", err)
	}
	defer getObjOutput.Body.Close()

	// Read the image data
	imgData, err := io.ReadAll(getObjOutput.Body)
	if err != nil {
		return fmt.Errorf("failed to read image data: %w", err)
	}

	// Get content type from original
	contentType := "image/jpeg" // Default
	if getObjOutput.ContentType != nil {
		contentType = *getObjOutput.ContentType
	}

	// Process the image
	thumbnail, err := createThumbnail(imgData)
	if err != nil {
		return fmt.Errorf("failed to create thumbnail: %w", err)
	}

	// Upload the thumbnail to the thumbnail bucket
	putObjInput := &s3.PutObjectInput{
		Bucket:      aws.String(config.ThumbnailsBucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(thumbnail),
		ContentType: aws.String(contentType),
		// Copy metadata from original image
		Metadata: getObjOutput.Metadata,
	}

	_, err = config.R2Client.PutObject(ctx, putObjInput)
	if err != nil {
		return fmt.Errorf("failed to upload thumbnail: %w", err)
	}

	return nil
}

// createThumbnail generates a thumbnail from image data
func createThumbnail(imgData []byte) ([]byte, error) {
	// Read the image
	src, err := imaging.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize to a thumbnail (max width 300px)
	maxWidth := 1024
	maxHeight := 1024

	// Resize the image preserving aspect ratio
	dstImg := imaging.Fit(src, maxWidth, maxHeight, imaging.Lanczos)

	// Encode the thumbnail
	var buf bytes.Buffer
	err = imaging.Encode(&buf, dstImg, imaging.JPEG, imaging.JPEGQuality(85))
	if err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

// isImageFile checks if a file has an image extension
func isImageFile(filename string) bool {
	ext := strings.ToLower(filename)
	return strings.HasSuffix(ext, ".jpg") ||
		strings.HasSuffix(ext, ".jpeg") ||
		strings.HasSuffix(ext, ".png") ||
		strings.HasSuffix(ext, ".gif") ||
		strings.HasSuffix(ext, ".webp")
}
