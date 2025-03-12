package config

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	// R2Client is the Cloudflare R2 client used across the application
	R2Client *s3.Client
	// BucketName is the name of the R2 bucket containing photos
	BucketName string
	// ThumbnailsBucketName is the bucket name for thumbnails
	ThumbnailsBucketName string
)

// InitR2Client initializes the Cloudflare R2 client
func InitR2Client(ctx context.Context) {
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	accessKeyID := os.Getenv("CLOUDFLARE_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("CLOUDFLARE_ACCESS_KEY_SECRET")
	BucketName = os.Getenv("CLOUDFLARE_BUCKET_NAME")
	ThumbnailsBucketName = os.Getenv("CLOUDFLARE_THUMBNAILS_BUCKET_NAME")

	if accountID == "" || accessKeyID == "" || accessKeySecret == "" || BucketName == "" || ThumbnailsBucketName == "" {
		log.Fatal("Missing Cloudflare R2 credentials or bucket names in environment variables")
	}

	// Load the default AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create custom options for the S3 client with the Cloudflare R2 endpoint
	r2Endpoint := "https://" + accountID + ".r2.cloudflarestorage.com"
	s3Options := s3.Options{
		Credentials:      cfg.Credentials,
		Region:           "auto",
		EndpointResolver: s3.EndpointResolverFromURL(r2Endpoint),
	}

	// Create the S3 client with custom options
	R2Client = s3.New(s3Options)
}
