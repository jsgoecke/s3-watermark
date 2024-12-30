package main

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
)

// createTestPNG creates a test PNG file with the given dimensions and returns its path
func createTestPNG(t *testing.T, width, height int, prefix string) string {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// Fill with a solid color
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	
	// Create temporary file
	tmpfile, err := os.CreateTemp("", prefix+"*.png")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	
	// Save the image
	if err := imaging.Save(img, tmpfile.Name()); err != nil {
		t.Fatalf("Failed to save test image: %v", err)
	}
	
	return tmpfile.Name()
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"JPG file", "test.jpg", true},
		{"JPEG file", "test.jpeg", true},
		{"PNG file", "test.png", true},
		{"Text file", "test.txt", false},
		{"No extension", "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isImageFile(tt.filename); got != tt.want {
				t.Errorf("isImageFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateEnvironment(t *testing.T) {
	// Create test watermark files
	leftWatermark := createTestPNG(t, 100, 50, "left")
	rightWatermark := createTestPNG(t, 100, 50, "right")
	defer os.Remove(leftWatermark)
	defer os.Remove(rightWatermark)

	// Save current environment
	savedEnv := make(map[string]string)
	envVars := []string{EnvBucket, EnvSourcePrefix, EnvTargetPrefix, EnvLeftWatermark, EnvRightWatermark}
	for _, env := range envVars {
		savedEnv[env] = os.Getenv(env)
	}

	// Restore environment after test
	defer func() {
		for env, value := range savedEnv {
			if value != "" {
				os.Setenv(env, value)
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "All environment variables set",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: rightWatermark,
			},
			wantErr: false,
		},
		{
			name: "Missing bucket",
			envVars: map[string]string{
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: rightWatermark,
			},
			wantErr:     true,
			errContains: EnvBucket,
		},
		{
			name: "Invalid left watermark path",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  "nonexistent.png",
				EnvRightWatermark: rightWatermark,
			},
			wantErr:     true,
			errContains: "watermark file not found",
		},
		{
			name: "Non-PNG watermark",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: "test.jpg",
			},
			wantErr:     true,
			errContains: "must be PNG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment variables
			for _, env := range envVars {
				os.Unsetenv(env)
			}

			// Set test environment variables
			for env, value := range tt.envVars {
				os.Setenv(env, value)
			}

			err := validateEnvironment()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("validateEnvironment() error = %v, should contain %v", err, tt.errContains)
			}
		})
	}
}

func TestNewImageProcessor(t *testing.T) {
	// Create test watermark files
	leftWatermark := createTestPNG(t, 100, 50, "left")
	rightWatermark := createTestPNG(t, 100, 50, "right")
	defer os.Remove(leftWatermark)
	defer os.Remove(rightWatermark)

	tests := []struct {
		name            string
		bucket          string
		sourcePrefix    string
		targetPrefix    string
		leftWatermark   string
		rightWatermark  string
		wantErr         bool
		errContains     string
	}{
		{
			name:           "Valid parameters",
			bucket:         "test-bucket",
			sourcePrefix:   "source/",
			targetPrefix:   "target/",
			leftWatermark:  leftWatermark,
			rightWatermark: rightWatermark,
			wantErr:        false,
		},
		{
			name:           "Empty bucket",
			sourcePrefix:   "source/",
			targetPrefix:   "target/",
			leftWatermark:  leftWatermark,
			rightWatermark: rightWatermark,
			wantErr:        true,
			errContains:    "must be non-empty",
		},
		{
			name:           "Invalid left watermark path",
			bucket:         "test-bucket",
			sourcePrefix:   "source/",
			targetPrefix:   "target/",
			leftWatermark:  "nonexistent.png",
			rightWatermark: rightWatermark,
			wantErr:        true,
			errContains:    "failed to load left watermark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor, err := NewImageProcessor(
				tt.bucket,
				tt.sourcePrefix,
				tt.targetPrefix,
				tt.leftWatermark,
				tt.rightWatermark,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewImageProcessor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("NewImageProcessor() error = %v, should contain %v", err, tt.errContains)
				}
				return
			}

			// Verify processor fields
			if processor.sourceBucket != tt.bucket {
				t.Errorf("NewImageProcessor() bucket = %v, want %v", processor.sourceBucket, tt.bucket)
			}
			if processor.sourcePrefix != tt.sourcePrefix {
				t.Errorf("NewImageProcessor() sourcePrefix = %v, want %v", processor.sourcePrefix, tt.sourcePrefix)
			}
			if processor.targetPrefix != tt.targetPrefix {
				t.Errorf("NewImageProcessor() targetPrefix = %v, want %v", processor.targetPrefix, tt.targetPrefix)
			}
			if processor.leftWatermark == nil {
				t.Error("NewImageProcessor() leftWatermark is nil")
			}
			if processor.rightWatermark == nil {
				t.Error("NewImageProcessor() rightWatermark is nil")
			}
			if processor.logger == nil {
				t.Error("NewImageProcessor() logger is nil")
			}
		})
	}
}

func TestAddWatermark(t *testing.T) {
	// Create test watermark files
	leftWatermark := createTestPNG(t, 100, 50, "left")
	rightWatermark := createTestPNG(t, 100, 50, "right")
	defer os.Remove(leftWatermark)
	defer os.Remove(rightWatermark)

	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	
	// Create processor with test watermarks
	processor, err := NewImageProcessor(
		"test-bucket",
		"source/",
		"target/",
		leftWatermark,
		rightWatermark,
	)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	// Test watermark addition
	watermarked := processor.addWatermark(img)
	
	// Verify image dimensions haven't changed
	if watermarked.Bounds() != img.Bounds() {
		t.Errorf("Watermarked image dimensions changed: got %v, want %v",
			watermarked.Bounds(), img.Bounds())
	}
	
	// Verify the watermarked image is not nil
	if watermarked == nil {
		t.Error("Watermarked image is nil")
	}

	// Test with small image
	smallImg := image.NewRGBA(image.Rect(0, 0, 200, 150))
	smallWatermarked := processor.addWatermark(smallImg)
	if smallWatermarked.Bounds() != smallImg.Bounds() {
		t.Error("Small image dimensions changed after watermarking")
	}
}
