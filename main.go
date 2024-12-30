package main

import (
	"context"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	WatermarkMaxHeight  = 100 // Maximum height of watermark in pixels
	WatermarkPadding    = 20  // Padding from edges in pixels
)

// validateEnvironment checks if all required environment variables are set
func validateEnvironment() error {
	required := []string{EnvBucket, EnvSourcePrefix, EnvTargetPrefix, EnvLeftWatermark, EnvRightWatermark}
	missing := []string{}

	for _, env := range required {
		if value := os.Getenv(env); value == "" {
			missing = append(missing, env)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	// Verify watermark files exist and are PNG
	watermarkPaths := []string{os.Getenv(EnvLeftWatermark), os.Getenv(EnvRightWatermark)}
	for _, path := range watermarkPaths {
		if !strings.HasSuffix(strings.ToLower(path), ".png") {
			return fmt.Errorf("watermark file must be PNG: %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("watermark file not found: %s", path)
		}
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
func NewImageProcessor(sourceBucket, sourcePrefix, targetPrefix, leftWatermarkPath, rightWatermarkPath string) (*ImageProcessor, error) {
	logger := log.New(os.Stdout, "[S3-WATERMARK] ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	if sourceBucket == "" || sourcePrefix == "" || targetPrefix == "" || leftWatermarkPath == "" || rightWatermarkPath == "" {
		return nil, fmt.Errorf("all parameters must be non-empty: bucket=%s, sourcePrefix=%s, targetPrefix=%s", 
			sourceBucket, sourcePrefix, targetPrefix)
	}

	logger.Printf("Loading watermark images...")
	leftWatermark, err := imaging.Open(leftWatermarkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load left watermark: %v", err)
	}

	rightWatermark, err := imaging.Open(rightWatermarkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load right watermark: %v", err)
	}

	logger.Printf("Initializing ImageProcessor with bucket: %s, source prefix: %s, target prefix: %s", 
		sourceBucket, sourcePrefix, targetPrefix)

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Printf("ERROR: Failed to load AWS SDK config: %v", err)
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	logger.Printf("Successfully initialized AWS SDK configuration")
	return &ImageProcessor{
		s3Client:       s3.NewFromConfig(cfg),
		sourceBucket:   sourceBucket,
		sourcePrefix:   sourcePrefix,
		targetPrefix:   targetPrefix,
		leftWatermark:  leftWatermark,
		rightWatermark: rightWatermark,
		logger:         logger,
	}, nil
}

// ProcessImages handles the main workflow
func (ip *ImageProcessor) ProcessImages() error {
	startTime := time.Now()
	ip.logger.Printf("Starting image processing workflow")
	ctx := context.Background()
	
	ip.logger.Printf("Listing objects in bucket: %s with prefix: %s", ip.sourceBucket, ip.sourcePrefix)
	input := &s3.ListObjectsV2Input{
		Bucket: &ip.sourceBucket,
		Prefix: &ip.sourcePrefix,
	}

	result, err := ip.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		ip.logger.Printf("ERROR: Failed to list objects: %v", err)
		return fmt.Errorf("failed to list objects: %v", err)
	}

	totalFiles := len(result.Contents)
	processedFiles := 0
	skippedFiles := 0
	failedFiles := 0

	ip.logger.Printf("Found %d objects in bucket", totalFiles)

	for _, object := range result.Contents {
		if !isImageFile(*object.Key) {
			ip.logger.Printf("Skipping non-image file: %s", *object.Key)
			skippedFiles++
			continue
		}

		ip.logger.Printf("Processing image: %s", *object.Key)
		if err := ip.processImage(ctx, *object.Key); err != nil {
			ip.logger.Printf("ERROR: Failed to process image %s: %v", *object.Key, err)
			failedFiles++
			continue
		}
		processedFiles++
	}

	duration := time.Since(startTime)
	ip.logger.Printf("Processing complete - Summary:")
	ip.logger.Printf("Total files: %d", totalFiles)
	ip.logger.Printf("Successfully processed: %d", processedFiles)
	ip.logger.Printf("Skipped: %d", skippedFiles)
	ip.logger.Printf("Failed: %d", failedFiles)
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
	watermarked := ip.addWatermark(img)

	// Create temporary file
	ip.logger.Printf("Creating temporary file for processed image")
	tempFile, err := os.CreateTemp("", "watermarked-*.png")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Save watermarked image
	ip.logger.Printf("Saving watermarked image to temporary file: %s", tempFile.Name())
	if err := imaging.Save(watermarked, tempFile.Name()); err != nil {
		return fmt.Errorf("failed to save watermarked image: %v", err)
	}

	// Upload to target location
	targetKey := filepath.Join(ip.targetPrefix, filepath.Base(key))
	ip.logger.Printf("Uploading watermarked image to target location: %s", targetKey)
	if err := ip.uploadImage(ctx, tempFile.Name(), targetKey); err != nil {
		return fmt.Errorf("failed to upload watermarked image: %v", err)
	}

	duration := time.Since(startTime)
	ip.logger.Printf("Successfully processed image %s in %v", key, duration)

	return nil
}

// addWatermark adds watermark images to the bottom corners of the image
func (ip *ImageProcessor) addWatermark(img image.Image) image.Image {
	ip.logger.Printf("Adding watermarks to image")
	ip.logger.Printf("Original image dimensions: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	
	// Clone the original image
	watermarked := imaging.Clone(img)
	imgWidth := watermarked.Bounds().Dx()
	imgHeight := watermarked.Bounds().Dy()
	
	// Resize watermarks if needed while maintaining aspect ratio
	leftWatermark := ip.leftWatermark
	rightWatermark := ip.rightWatermark
	
	if leftWatermark.Bounds().Dy() > WatermarkMaxHeight {
		leftWatermark = imaging.Resize(leftWatermark, 0, WatermarkMaxHeight, imaging.Lanczos)
		ip.logger.Printf("Resized left watermark to height: %d", WatermarkMaxHeight)
	}
	
	if rightWatermark.Bounds().Dy() > WatermarkMaxHeight {
		rightWatermark = imaging.Resize(rightWatermark, 0, WatermarkMaxHeight, imaging.Lanczos)
		ip.logger.Printf("Resized right watermark to height: %d", WatermarkMaxHeight)
	}
	
	// Calculate positions for watermarks
	leftX := WatermarkPadding
	rightX := imgWidth - rightWatermark.Bounds().Dx() - WatermarkPadding
	y := imgHeight - WatermarkMaxHeight - WatermarkPadding
	
	// Add left watermark
	watermarked = imaging.Overlay(watermarked, leftWatermark, image.Pt(leftX, y), 1.0)
	
	// Add right watermark
	watermarked = imaging.Overlay(watermarked, rightWatermark, image.Pt(rightX, y), 1.0)
	
	ip.logger.Printf("Watermarks added successfully")
	return watermarked
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
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("Starting S3 Watermark Script")

	if err := validateEnvironment(); err != nil {
		log.Fatalf("Environment validation failed: %v\nRequired environment variables:\n"+
			"  %s: S3 bucket name\n"+
			"  %s: Source directory prefix in S3\n"+
			"  %s: Target directory prefix in S3\n"+
			"  %s: Path to left watermark PNG file\n"+
			"  %s: Path to right watermark PNG file\n",
			err, EnvBucket, EnvSourcePrefix, EnvTargetPrefix, EnvLeftWatermark, EnvRightWatermark)
	}

	processor, err := NewImageProcessor(
		os.Getenv(EnvBucket),
		os.Getenv(EnvSourcePrefix),
		os.Getenv(EnvTargetPrefix),
		os.Getenv(EnvLeftWatermark),
		os.Getenv(EnvRightWatermark),
	)
	if err != nil {
		log.Fatalf("Failed to create image processor: %v", err)
	}

	if err := processor.ProcessImages(); err != nil {
		log.Fatalf("Failed to process images: %v", err)
	}

	log.Println("Successfully completed all image processing")
}
