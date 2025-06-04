package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Source file setup
	srcFileName := "source.txt"
	srcFilePath := filepath.Join(tmpDir, srcFileName)
	srcContent := []byte("This is the source file content.")
	if err := os.WriteFile(srcFilePath, srcContent, 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Destination path within a subdirectory that will be created by CopyFile
	destDir := filepath.Join(tmpDir, "dest_sub")
	destFilePath := filepath.Join(destDir, "destination.txt")

	nonExistentSrcPath := filepath.Join(tmpDir, "non_existent_source.txt")
	invalidDestPath := "/dev/null/cannot_create_here/destination.txt" // Assuming this path is generally unwritable

	tests := []struct {
		name        string
		srcPath     string
		destPath    string
		expectErr   bool
		checkContent bool // whether to verify content after successful copy
	}{
		{
			name:        "successful copy",
			srcPath:     srcFilePath,
			destPath:    destFilePath,
			expectErr:   false,
			checkContent: true,
		},
		{
			name:        "source file does not exist",
			srcPath:     nonExistentSrcPath,
			destPath:    filepath.Join(tmpDir, "dest_non_existent_src.txt"),
			expectErr:   true,
			checkContent: false,
		},
		{
            name:        "destination is invalid (e.g. unwritable path)",
            srcPath:     srcFilePath,
            // On some systems, /dev/null itself is a file, so a subdir might fail.
            // A more robust test for unwritable might involve setting up permissions,
            // but this is a common way to test for path creation failures.
            // If this test is flaky, it might need adjustment based on the test environment.
            destPath:    invalidDestPath,
            expectErr:   true,
            checkContent: false,
        },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CopyFile(tt.srcPath, tt.destPath)

			if (err != nil) != tt.expectErr {
				t.Errorf("CopyFile() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr && tt.checkContent {
				copiedContent, readErr := os.ReadFile(tt.destPath)
				if readErr != nil {
					t.Fatalf("Failed to read copied file %s: %v", tt.destPath, readErr)
				}
				originalContent, readErr := os.ReadFile(tt.srcPath) // Read original again to be sure
				if readErr != nil {
					t.Fatalf("Failed to read original source file %s: %v", tt.srcPath, readErr)
				}
				if !reflect.DeepEqual(copiedContent, originalContent) {
					t.Errorf("CopyFile() content mismatch. Copied: %s, Original: %s", string(copiedContent), string(originalContent))
				}
			}
		})
	}
}
