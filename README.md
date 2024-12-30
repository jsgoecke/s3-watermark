# S3 Image Watermarking Script

A Go-based utility for automatically watermarking images stored in AWS S3. This script processes images from a source S3 prefix, adds two watermark images (one in each bottom corner), and saves the processed images to a target S3 prefix.

## Features

- Processes images from AWS S3 source directory
- Adds two PNG watermarks to each image:
  - Left watermark in bottom-left corner
  - Right watermark in bottom-right corner
- Automatically resizes watermarks while maintaining aspect ratio
- Supports JPG, JPEG, and PNG input images
- Comprehensive logging of all operations
- Configurable through environment variables
- Full test coverage for core functionality

## Requirements

- Go 1.21 or later
- AWS credentials configured
- Two PNG watermark images (preferably with transparency)

## Installation

1. Clone the repository:
```bash
git clone [your-repository-url]
cd s3-watermark-script
```

2. Install dependencies:
```bash
go mod download
```

## Configuration

The script requires the following environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `S3_BUCKET` | Name of the S3 bucket | "my-images-bucket" |
| `SOURCE_PREFIX` | Source directory prefix in S3 | "original/images/" |
| `TARGET_PREFIX` | Target directory prefix for processed images | "processed/images/" |
| `LEFT_WATERMARK_PATH` | Path to left watermark PNG file | "/path/to/left-logo.png" |
| `RIGHT_WATERMARK_PATH` | Path to right watermark PNG file | "/path/to/right-logo.png" |

### AWS Credentials

Configure AWS credentials using one of these methods:

1. Environment variables:
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="your-region"
```

2. AWS credentials file (`~/.aws/credentials`):
```ini
[default]
aws_access_key_id = your-access-key
aws_secret_access_key = your-secret-key
region = your-region
```

## Usage

1. Set up environment variables:
```bash
export S3_BUCKET="your-bucket-name"
export SOURCE_PREFIX="source/images/"
export TARGET_PREFIX="processed/images/"
export LEFT_WATERMARK_PATH="/path/to/left-watermark.png"
export RIGHT_WATERMARK_PATH="/path/to/right-watermark.png"
```

2. Run the script:
```bash
go run main.go
```

## Watermark Specifications

- Maximum height: 100 pixels
- Edge padding: 20 pixels
- Position:
  - Left watermark: Bottom-left corner
  - Right watermark: Bottom-right corner
- Format: PNG (preferably with transparency)
- Aspect ratio is preserved when resizing

## Logging

The script provides detailed logging including:
- Processing start and completion
- Image dimensions
- Processing duration
- Success/failure status
- Error details when applicable
- Summary statistics

Example log output:
```
[S3-WATERMARK] 2024/12/30 15:47:03 Starting S3 Watermark Script
[S3-WATERMARK] 2024/12/30 15:47:03 Initializing ImageProcessor...
[S3-WATERMARK] 2024/12/30 15:47:03 Processing image: example.jpg
[S3-WATERMARK] 2024/12/30 15:47:04 Successfully processed all images
```

## Error Handling

The script includes comprehensive error handling for:
- Missing environment variables
- Invalid watermark files
- S3 access issues
- Image processing failures
- File format validation

## Development

### Running Tests

Run the test suite:
```bash
go test -v
```

Run with coverage:
```bash
go test -v -cover
```

### Code Structure

- `main.go`: Core script functionality
- `main_test.go`: Test suite
- `go.mod`: Dependencies and module information

## Limitations

- Only processes JPG, JPEG, and PNG images
- Watermark files must be PNG format
- Maximum processing batch size determined by AWS S3 listing limits

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request

## License

[Your chosen license]
