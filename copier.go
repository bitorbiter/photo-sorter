package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies a file from srcPath to destPath.
// It ensures the destination directory exists.
func CopyFile(srcPath, destPath string) error {
	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	sourceFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy content from %s to %s: %w", srcPath, destPath, err)
	}

	// It's good practice to sync the destination file to disk.
	err = destinationFile.Sync()
	if err != nil {
		// This error might not be critical for the copy itself but indicates a flushing issue.
		return fmt.Errorf("failed to sync destination file %s: %w", destPath, err)
	}

	return nil
}
