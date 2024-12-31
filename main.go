package main

import (
	"context"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"
)

// Required environment variables
const (
	EnvBucket           = "S3_BUCKET"
	EnvSourcePrefix     = "SOURCE_PREFIX"
	EnvTargetPrefix     = "TARGET_PREFIX"
	EnvLeftWatermark    = "LEFT_WATERMARK_PATH"
	EnvRightWatermark   = "RIGHT_WATERMARK_PATH"
	MaxWatermarkHeight  = 250 // Maximum height of watermark in pixels
	WatermarkPadding    = 20  // Padding around watermarks in pixels
	MaxWorkers          = 5   // Maximum number of concurrent workers
)

// loadWatermarkImage loads a watermark image from a file path or URL
func loadWatermarkImage(path string) (image.Image, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := http.Get(path)
		if err != nil {
			return nil, fmt.Errorf("failed to download watermark from URL %s: %v", path, err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download watermark from URL %s: status code %d", path, resp.StatusCode)
		}
		
		return imaging.Decode(resp.Body)
	}
	
	// Local file path
	return imaging.Open(path)
}

// validateWatermarkPath validates a watermark path
func validateWatermarkPath(path string) error {
	// Check if it's a URL
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if !strings.HasSuffix(strings.ToLower(path), ".png") {
			return fmt.Errorf("watermark URL must end with .png: %s", path)
		}
		return nil
	}

	// For local files, check extension first
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		return fmt.Errorf("watermark must be a PNG file: %s", path)
	}

	// Then check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("watermark file not found: %s", path)
	}

	return nil
}

// validateEnvironment checks if all required environment variables are set
func validateEnvironment() error {
	requiredVars := []string{
		EnvBucket,
		EnvSourcePrefix,
		EnvTargetPrefix,
		EnvLeftWatermark,
		EnvRightWatermark,
	}

	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			return fmt.Errorf("required environment variable %s is not set", v)
		}
	}

	// Validate watermark files
	if err := validateWatermarkPath(os.Getenv(EnvLeftWatermark)); err != nil {
		return err
	}
	if err := validateWatermarkPath(os.Getenv(EnvRightWatermark)); err != nil {
		return err
	}

	return nil
}

type ImageProcessor struct {
	s3Client       *s3.Client
	sourceBucket   string
	sourcePrefix   string
	targetPrefix   string
	leftWatermark  image.Image
	rightWatermark image.Image
	logger         *log.Logger
}

// NewImageProcessor creates a new instance of ImageProcessor
func NewImageProcessor(ctx context.Context, logger *log.Logger) (*ImageProcessor, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	if err := validateEnvironment(); err != nil {
		return nil, err
	}

	leftWatermarkPath := os.Getenv(EnvLeftWatermark)
	rightWatermarkPath := os.Getenv(EnvRightWatermark)

	leftWatermark, err := loadWatermarkImage(leftWatermarkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load left watermark: %v", err)
	}

	rightWatermark, err := loadWatermarkImage(rightWatermarkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load right watermark: %v", err)
	}

	logger.Printf("Initializing ImageProcessor with bucket: %s, source prefix: %s, target prefix: %s", 
		os.Getenv(EnvBucket), os.Getenv(EnvSourcePrefix), os.Getenv(EnvTargetPrefix))

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Printf("ERROR: Failed to load AWS SDK config: %v", err)
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	logger.Printf("Successfully initialized AWS SDK configuration")
	return &ImageProcessor{
		s3Client:       s3.NewFromConfig(cfg),
		sourceBucket:   os.Getenv(EnvBucket),
		sourcePrefix:   os.Getenv(EnvSourcePrefix),
		targetPrefix:   os.Getenv(EnvTargetPrefix),
		leftWatermark:  leftWatermark,
		rightWatermark: rightWatermark,
		logger:         logger,
	}, nil
}

type ProcessResult struct {
	Key string
	Err error
}

func (ip *ImageProcessor) ProcessImages(ctx context.Context) error {
	startTime := time.Now()
	ip.logger.Printf("Starting image processing workflow")
	
	// Get list of images to process
	input := &s3.ListObjectsV2Input{
		Bucket: &ip.sourceBucket,
		Prefix: &ip.sourcePrefix,
	}

	result, err := ip.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		ip.logger.Printf("ERROR: Failed to list objects: %v", err)
		return fmt.Errorf("failed to list objects: %v", err)
	}

	if len(result.Contents) == 0 {
		ip.logger.Printf("No images found in bucket %s with prefix %s", ip.sourceBucket, ip.sourcePrefix)
		return nil
	}

	// Create channels for work distribution and results
	jobs := make(chan string, len(result.Contents))
	results := make(chan ProcessResult, len(result.Contents))
	
	// Start worker pool
	var wg sync.WaitGroup
	for w := 1; w <= MaxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for key := range jobs {
				err := ip.processImage(ctx, key)
				results <- ProcessResult{
					Key: key,
					Err: err,
				}
				if err != nil {
					ip.logger.Printf("Worker %d: Failed to process %s: %v", workerID, key, err)
				} else {
					ip.logger.Printf("Worker %d: Successfully processed %s", workerID, key)
				}
			}
		}(w)
	}

	// Send jobs to workers
	imageCount := 0
	for _, obj := range result.Contents {
		if obj.Key == nil {
			continue
		}
		key := *obj.Key
		if !isImageFile(key) {
			continue
		}
		jobs <- key
		imageCount++
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	var errors []error
	successCount := 0
	for result := range results {
		if result.Err != nil {
			errors = append(errors, fmt.Errorf("failed to process %s: %v", result.Key, result.Err))
		} else {
			successCount++
		}
	}

	// Log summary
	ip.logger.Printf("Processing complete. Successfully processed %d/%d images", successCount, imageCount)
	if len(errors) > 0 {
		return fmt.Errorf("encountered %d errors during processing: %v", len(errors), errors)
	}

	duration := time.Since(startTime)
	ip.logger.Printf("Total duration: %v", duration)

	return nil
}

// processImage handles individual image processing
func (ip *ImageProcessor) processImage(ctx context.Context, key string) error {
	startTime := time.Now()
	ip.logger.Printf("Starting processing of image: %s", key)

	// Download image
	ip.logger.Printf("Downloading image from S3: %s", key)
	getInput := &s3.GetObjectInput{
		Bucket: &ip.sourceBucket,
		Key:    &key,
	}

	result, err := ip.s3Client.GetObject(ctx, getInput)
	if err != nil {
		return fmt.Errorf("failed to get object %s: %v", key, err)
	}
	defer result.Body.Close()

	ip.logger.Printf("Successfully downloaded image: %s", key)

	// Decode image
	ip.logger.Printf("Decoding image: %s", key)
	img, err := imaging.Decode(result.Body)
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %v", key, err)
	}
	ip.logger.Printf("Successfully decoded image: %s, dimensions: %dx%d", key, img.Bounds().Dx(), img.Bounds().Dy())

	// Add watermark
	ip.logger.Printf("Adding watermark to image: %s", key)
	watermarked, err := ip.addWatermark(img)
	if err != nil {
		return fmt.Errorf("failed to add watermark to image %s: %v", key, err)
	}

	// Create temporary file
	ip.logger.Printf("Creating temporary file for processed image: %s", key)
	tempFile, err := os.CreateTemp("", "watermarked-*.jpg")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	// Save processed image to temp file
	err = imaging.Save(watermarked, tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to save processed image: %v", err)
	}

	// Upload processed image
	targetKey := strings.Replace(key, ip.sourcePrefix, ip.targetPrefix, 1)
	ip.logger.Printf("Uploading processed image to: %s", targetKey)

	err = ip.uploadImage(ctx, tempFile.Name(), targetKey)
	if err != nil {
		return fmt.Errorf("failed to upload processed image %s: %v", targetKey, err)
	}

	duration := time.Since(startTime)
	ip.logger.Printf("Successfully processed image %s in %v", key, duration)
	return nil
}

// addWatermark adds watermarks to the given image
func (ip *ImageProcessor) addWatermark(img image.Image) (image.Image, error) {
	ip.logger.Printf("Adding watermarks to image")
	ip.logger.Printf("Original image dimensions: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())

	// Convert to RGBA if it's not already
	watermarked := imaging.Clone(img)
	imgWidth := watermarked.Bounds().Dx()
	imgHeight := watermarked.Bounds().Dy()

	// Create copies of watermarks for resizing
	leftWatermark := ip.leftWatermark
	rightWatermark := ip.rightWatermark
	
	if leftWatermark.Bounds().Dy() > MaxWatermarkHeight {
		leftWatermark = imaging.Resize(leftWatermark, 0, MaxWatermarkHeight, imaging.Lanczos)
		ip.logger.Printf("Resized left watermark to height: %d", MaxWatermarkHeight)
	}
	
	if rightWatermark.Bounds().Dy() > MaxWatermarkHeight {
		rightWatermark = imaging.Resize(rightWatermark, 0, MaxWatermarkHeight, imaging.Lanczos)
		ip.logger.Printf("Resized right watermark to height: %d", MaxWatermarkHeight)
	}
	
	// Calculate positions for watermarks
	leftX := WatermarkPadding
	rightX := imgWidth - rightWatermark.Bounds().Dx() - WatermarkPadding
	y := imgHeight - MaxWatermarkHeight - WatermarkPadding
	
	// Add left watermark
	watermarked = imaging.Overlay(watermarked, leftWatermark, image.Pt(leftX, y), 1.0)
	
	// Add right watermark
	watermarked = imaging.Overlay(watermarked, rightWatermark, image.Pt(rightX, y), 1.0)

	ip.logger.Printf("Watermarks added successfully")
	return watermarked, nil
}

// uploadImage uploads the processed image to S3
func (ip *ImageProcessor) uploadImage(ctx context.Context, filepath, targetKey string) error {
	ip.logger.Printf("Starting upload of file %s to S3 key: %s", filepath, targetKey)
	
	file, err := os.Open(filepath)
	if err != nil {
		ip.logger.Printf("ERROR: Failed to open file %s: %v", filepath, err)
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	putInput := &s3.PutObjectInput{
		Bucket: &ip.sourceBucket,
		Key:    &targetKey,
		Body:   file,
	}

	startTime := time.Now()
	_, err = ip.s3Client.PutObject(ctx, putInput)
	if err != nil {
		ip.logger.Printf("ERROR: Failed to upload file to S3: %v", err)
		return err
	}

	duration := time.Since(startTime)
	ip.logger.Printf("Successfully uploaded file to S3 in %v", duration)
	return nil
}

// isImageFile checks if the file is an image based on extension
func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func main() {
	logger := log.New(os.Stdout, "[S3-WATERMARK] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("Starting S3 Watermark Script")

	if err := validateEnvironment(); err != nil {
		logger.Printf("Environment validation failed: %v\nRequired environment variables:\n"+
			"  %s: S3 bucket name\n"+
			"  %s: Source directory prefix in S3\n"+
			"  %s: Target directory prefix in S3\n"+
			"  %s: Path to left watermark PNG file or URL\n"+
			"  %s: Path to right watermark PNG file or URL\n",
			err, EnvBucket, EnvSourcePrefix, EnvTargetPrefix, EnvLeftWatermark, EnvRightWatermark)
		os.Exit(1)
	}

	ctx := context.Background()
	processor, err := NewImageProcessor(ctx, logger)
	if err != nil {
		logger.Printf("Failed to initialize image processor: %v", err)
		os.Exit(1)
	}

	if err := processor.ProcessImages(ctx); err != nil {
		logger.Printf("Failed to process images: %v", err)
		os.Exit(1)
	}

	logger.Println("Successfully completed all image processing")
}
