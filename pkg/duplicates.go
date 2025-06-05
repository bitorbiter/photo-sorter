package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"os"
)

// ErrUnsupportedForPixelHashing is returned when a file format is not supported for pixel data hashing.
var ErrUnsupportedForPixelHashing = fmt.Errorf("file format not supported for pixel data hashing")

// CalculateFileHash calculates the SHA-256 hash of a file's content.
func CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for hashing: %w", filePath, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to copy file content to hasher for %s: %w", filePath, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetImageResolution decodes the image configuration to get its width and height.
// It supports JPEG, PNG, and GIF formats via the standard library.
func GetImageResolution(filePath string) (width int, height int, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image file %s for resolution: %w", filePath, err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		// This error can occur if the file is not a recognized image format
		// or if the image data is corrupted.
		return 0, 0, fmt.Errorf("failed to decode image config for %s: %w", filePath, err)
	}

	return config.Width, config.Height, nil
}

// CalculatePixelDataHash calculates the SHA-256 hash of an image's raw pixel data.
// It supports JPEG, PNG, and GIF formats.
func CalculatePixelDataHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for pixel hashing: %w", filePath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		// This can happen for unsupported formats or corrupted image data.
		return "", fmt.Errorf("%w: %v", ErrUnsupportedForPixelHashing, err)
	}

	hasher := sha256.New()
	bounds := img.Bounds()
	pixelBytes := make([]byte, 4) // For R, G, B, A components

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA() // uint32 values (0-0xFFFF)

			// Convert to uint8 (0-0xFF) for consistent hashing
			pixelBytes[0] = byte(r >> 8)
			pixelBytes[1] = byte(g >> 8)
			pixelBytes[2] = byte(b >> 8)
			pixelBytes[3] = byte(a >> 8)

			if _, err := hasher.Write(pixelBytes); err != nil {
				return "", fmt.Errorf("failed to write pixel data to hasher for %s: %w", filePath, err)
			}
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
