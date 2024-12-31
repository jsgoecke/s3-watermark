package main

import (
	"context"
	"image"
	"image/color"
	"log"
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
			name: "All environment variables set (local files)",
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
			name: "All environment variables set (URLs)",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  "https://example.com/watermark1.png",
				EnvRightWatermark: "https://example.com/watermark2.png",
			},
			wantErr: false,
		},
		{
			name: "Mixed local file and URL",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: "https://example.com/watermark.png",
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
			name: "Invalid local watermark path",
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
			name: "Non-PNG local watermark",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: "test.jpg",
			},
			wantErr:     true,
			errContains: "must be a PNG file",
		},
		{
			name: "Non-PNG URL watermark",
			envVars: map[string]string{
				EnvBucket:         "test-bucket",
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  "https://example.com/watermark.jpg",
				EnvRightWatermark: rightWatermark,
			},
			wantErr:     true,
			errContains: "must end with .png",
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
		name            string
		envVars         map[string]string
		wantErr         bool
		errContains     string
	}{
		{
			name: "Valid parameters",
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
			name: "Empty bucket",
			envVars: map[string]string{
				EnvSourcePrefix:   "source/",
				EnvTargetPrefix:   "target/",
				EnvLeftWatermark:  leftWatermark,
				EnvRightWatermark: rightWatermark,
			},
			wantErr:     true,
			errContains: "is not set",
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

			logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
			ctx := context.Background()
			
			processor, err := NewImageProcessor(ctx, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewImageProcessor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("NewImageProcessor() error = %v, should contain %v", err, tt.errContains)
			}

			if err == nil {
				if processor.s3Client == nil {
					t.Error("NewImageProcessor() s3Client is nil")
				}
				if processor.leftWatermark == nil {
					t.Error("NewImageProcessor() leftWatermark is nil")
				}
				if processor.rightWatermark == nil {
					t.Error("NewImageProcessor() rightWatermark is nil")
				}
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

	// Set up environment
	os.Setenv(EnvBucket, "test-bucket")
	os.Setenv(EnvSourcePrefix, "source/")
	os.Setenv(EnvTargetPrefix, "target/")
	os.Setenv(EnvLeftWatermark, leftWatermark)
	os.Setenv(EnvRightWatermark, rightWatermark)

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	ctx := context.Background()
	
	processor, err := NewImageProcessor(ctx, logger)
	if err != nil {
		t.Fatalf("Failed to create ImageProcessor: %v", err)
	}

	// Test cases
	tests := []struct {
		name        string
		imgWidth    int
		imgHeight   int
		wantErr     bool
	}{
		{
			name:      "Normal image",
			imgWidth:  800,
			imgHeight: 600,
			wantErr:   false,
		},
		{
			name:      "Small image",
			imgWidth:  200,
			imgHeight: 150,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test image
			img := image.NewRGBA(image.Rect(0, 0, tt.imgWidth, tt.imgHeight))
			for x := 0; x < tt.imgWidth; x++ {
				for y := 0; y < tt.imgHeight; y++ {
					img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				}
			}

			result, err := processor.addWatermark(img)
			if (err != nil) != tt.wantErr {
				t.Errorf("addWatermark() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result == nil {
					t.Error("addWatermark() returned nil result")
				}
				if result.Bounds().Dx() != tt.imgWidth || result.Bounds().Dy() != tt.imgHeight {
					t.Errorf("addWatermark() result dimensions = %dx%d, want %dx%d",
						result.Bounds().Dx(), result.Bounds().Dy(),
						tt.imgWidth, tt.imgHeight)
				}
			} else {
				if err == nil {
					t.Errorf("addWatermark() did not return an error, wantErr %v", tt.wantErr)
				}
			}
		})
	}
}
