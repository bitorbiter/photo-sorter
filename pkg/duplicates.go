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
