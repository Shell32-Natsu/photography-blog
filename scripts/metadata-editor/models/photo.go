package models

import (
	"context"
	"fmt"
	"log"
	"maps"
	"mime"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"metadata-editor/config"
)

// Photo represents a photo from the R2 bucket with its metadata
type Photo struct {
	Key          string            `json:"key"`
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	LastModified time.Time         `json:"lastModified"`
	Metadata     map[string]string `json:"metadata"`
}

// PhotoList represents a paginated list of photos
type PhotoList struct {
	Photos     []Photo  `json:"photos"`
	TotalCount int      `json:"totalCount"`
	Offset     int      `json:"offset"`
	Limit      int      `json:"limit"`
	AllTags    []string `json:"allTags"`
}

// UpdatePhotoRequest represents the request body for updating a photo's metadata
type UpdatePhotoRequest struct {
	Key      string            `json:"key"`      // Object key in the R2 bucket
	Metadata map[string]string `json:"metadata"` // New metadata to apply
}

// UpdatePhotoResponse represents the response when updating a photo's metadata
type UpdatePhotoResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetPhotos retrieves photos from the R2 bucket with pagination
func GetPhotos(ctx context.Context, offset, limit int) (PhotoList, error) {
	// List all objects in the bucket
	listObjectsOutput, err := config.R2Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(config.BucketName),
	})
	if err != nil {
		log.Printf("Failed to list objects in bucket: %v", err)
		return PhotoList{}, fmt.Errorf("failed to list objects in bucket: %w", err)
	}
	// Get all unique tags from the photos
	allTags := make(map[string]bool)

	// Convert S3 objects to our Photo model
	var returnedPhotos []Photo
	dec := new(mime.WordDecoder)

	// Create a channel to receive processed photos
	type photoResult struct {
		photo Photo
		err   error
	}
	resultCh := make(chan photoResult)

	// Process each object asynchronously
	for _, obj := range listObjectsOutput.Contents {
		go func(obj types.Object) {
			// Get object metadata
			headOutput, err := config.R2Client.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(config.BucketName),
				Key:    obj.Key,
			})
			if err != nil {
				log.Printf("Failed to get metadata for object %s: %v", *obj.Key, err)
				resultCh <- photoResult{err: err}
				return
			}

			// Convert S3 metadata to our format
			metadata := make(map[string]string)
			for k, v := range headOutput.Metadata {
				if !strings.HasPrefix(v, "=?utf-8?") {
					metadata[k] = v
					continue
				}
				decodedString := ""
				for _, seg := range strings.Split(v, " ") {
					s, err := dec.Decode(seg)
					if err != nil {
						log.Printf("Failed to decode metadata key (%s:%s) on %s: %v", k, v, *obj.Key, err)
						continue
					}
					decodedString += s
				}
				metadata[k] = decodedString
			}

			name := *obj.Key
			if lastSlash := strings.LastIndex(name, "/"); lastSlash >= 0 {
				name = name[lastSlash+1:]
			}

			// Create public URL
			url := fmt.Sprintf("https://photo-r2.xiadong.info/%s", *obj.Key)

			photo := Photo{
				Key:          *obj.Key,
				Name:         name,
				URL:          url,
				LastModified: *obj.LastModified,
				Metadata:     metadata,
			}

			resultCh <- photoResult{photo: photo}
		}(obj)
	}

	// Collect results from all goroutines
	for i := 0; i < len(listObjectsOutput.Contents); i++ {
		result := <-resultCh
		if result.err != nil {
			continue
		}

		// Process tags from metadata for all photos
		if tagString, ok := result.photo.Metadata["tag"]; ok {
			for _, t := range strings.Split(tagString, ",") {
				if t != "" {
					allTags[t] = true
				}
			}
		}

		returnedPhotos = append(returnedPhotos, result.photo)
	}

	// Sort photos by file name (alphabetically)
	sort.Slice(returnedPhotos, func(i, j int) bool {
		return returnedPhotos[i].Name < returnedPhotos[j].Name
	})

	// Apply pagination
	paginatedPhotos := PhotoList{
		TotalCount: len(returnedPhotos),
		Offset:     offset,
		Limit:      limit,
	}

	// Make sure offset is within bounds
	if offset >= len(returnedPhotos) {
		return paginatedPhotos, nil
	}

	// Calculate the end index for pagination
	endIndex := offset + limit
	if endIndex > len(returnedPhotos) {
		endIndex = len(returnedPhotos)
	}

	for tag := range allTags {
		paginatedPhotos.AllTags = append(paginatedPhotos.AllTags, tag)
	}

	// Set the paginated photos
	paginatedPhotos.Photos = returnedPhotos[offset:endIndex]

	return paginatedPhotos, nil
}

// UpdatePhotoMetadata updates the custom metadata for a photo in the R2 bucket
func UpdatePhotoMetadata(ctx context.Context, req UpdatePhotoRequest) (UpdatePhotoResponse, error) {
	// First, check if the object exists and get its current metadata
	headOutput, err := config.R2Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(config.BucketName),
		Key:    aws.String(req.Key),
	})
	if err != nil {
		log.Printf("Failed to find object %s: %v", req.Key, err)
		return UpdatePhotoResponse{
			Success: false,
			Message: fmt.Sprintf("Photo with key %s not found", req.Key),
		}, fmt.Errorf("photo not found: %w", err)
	}

	// Get the object's content type from existing metadata
	contentType := "application/octet-stream"
	if headOutput.ContentType != nil {
		contentType = *headOutput.ContentType
	}

	// We need to use CopyObject to update metadata in S3/R2
	// First, prepare the metadata to be updated
	metadata := make(map[string]string)
	maps.Copy(metadata, req.Metadata)

	// Create copy input
	copyInput := &s3.CopyObjectInput{
		Bucket:            aws.String(config.BucketName),
		CopySource:        aws.String(config.BucketName + "/" + req.Key),
		Key:               aws.String(req.Key),
		Metadata:          metadata,
		MetadataDirective: "REPLACE", // This tells S3 to replace the metadata completely
		ContentType:       aws.String(contentType),
	}

	// Copy the object to itself with new metadata
	_, err = config.R2Client.CopyObject(ctx, copyInput)
	if err != nil {
		log.Printf("Failed to update metadata for object %s: %v", req.Key, err)
		return UpdatePhotoResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update metadata: %v", err),
		}, fmt.Errorf("failed to update metadata: %w", err)
	}

	return UpdatePhotoResponse{
		Success: true,
		Message: "Metadata updated successfully",
	}, nil
}
