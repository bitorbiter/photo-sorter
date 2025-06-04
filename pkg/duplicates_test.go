package pkg_test

import (
	"github.com/user/photo-sorter/pkg"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateFileHash(t *testing.T) {
	tmpDir := t.TempDir()

	file1Content := []byte("hello world")
	file1Path := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(file1Path, file1Content, 0644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}

	file2Content := []byte("hello world") // Same content as file1
	file2Path := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file2Path, file2Content, 0644); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}

	file3Content := []byte("different content")
	file3Path := filepath.Join(tmpDir, "file3.txt")
	if err := os.WriteFile(file3Path, file3Content, 0644); err != nil {
		t.Fatalf("Failed to write file3: %v", err)
	}

	nonExistentFilePath := filepath.Join(tmpDir, "non_existent_file.txt")

	hash1, err1 := pkg.CalculateFileHash(file1Path)
	if err1 != nil {
		t.Fatalf("pkg.CalculateFileHash(file1Path) error: %v", err1)
	}

	hash1Again, err1Again := pkg.CalculateFileHash(file1Path)
	if err1Again != nil {
		t.Fatalf("pkg.CalculateFileHash(file1Path) again error: %v", err1Again)
	}
	if hash1 != hash1Again {
		t.Errorf("pkg.CalculateFileHash(file1Path) returned different hashes on subsequent calls: %s vs %s", hash1, hash1Again)
	}

	hash2, err2 := pkg.CalculateFileHash(file2Path)
	if err2 != nil {
		t.Fatalf("pkg.CalculateFileHash(file2Path) error: %v", err2)
	}
	if hash1 != hash2 {
		t.Errorf("Expected identical hashes for identical files, got %s and %s", hash1, hash2)
	}

	hash3, err3 := pkg.CalculateFileHash(file3Path)
	if err3 != nil {
		t.Fatalf("pkg.CalculateFileHash(file3Path) error: %v", err3)
	}
	if hash1 == hash3 {
		t.Errorf("Expected different hashes for different files, got %s for both", hash1)
	}

	_, errNonExistent := pkg.CalculateFileHash(nonExistentFilePath)
	if errNonExistent == nil {
		t.Errorf("pkg.CalculateFileHash(nonExistentFilePath) expected error, got nil")
	}
}

// Helper to create a dummy PNG file for resolution testing
func createDummyPNG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill image with a color to make it non-zero if needed, though for config it might not matter
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{100, 200, 200, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create dummy PNG file %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("Failed to encode dummy PNG %s: %v", path, err)
	}
}

func TestGetImageResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid PNG
	pngFilePath := filepath.Join(tmpDir, "test.png")
	expectedWidth, expectedHeight := 100, 50
	createDummyPNG(t, pngFilePath, expectedWidth, expectedHeight)

	// File that is not an image
	notAnImagePath := filepath.Join(tmpDir, "not_an_image.txt")
	if err := os.WriteFile(notAnImagePath, []byte("this is not an image"), 0644); err != nil {
		t.Fatalf("Failed to create dummy text file: %v", err)
	}

	nonExistentFilePath := filepath.Join(tmpDir, "non_existent_image.png")

	tests := []struct {
		name           string
		filePath       string
		expectedWidth  int
		expectedHeight int
		expectErr      bool
	}{
		{
			name:           "valid PNG",
			filePath:       pngFilePath,
			expectedWidth:  expectedWidth,
			expectedHeight: expectedHeight,
			expectErr:      false,
		},
		{
			name:           "not an image file",
			filePath:       notAnImagePath,
			expectedWidth:  0,
			expectedHeight: 0,
			expectErr:      true,
		},
		{
			name:           "non-existent file",
			filePath:       nonExistentFilePath,
			expectedWidth:  0,
			expectedHeight: 0,
			expectErr:      true,
		},
		// TODO: Add tests for JPEG and GIF if small sample files can be created/obtained.
		// For JPEG, image.RegisterFormat is usually done via `import _ "image/jpeg"`.
		// For GIF, image.RegisterFormat is usually done via `import _ "image/gif"`.
		// These are already in duplicates.go, so they should work if valid files are provided.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, err := pkg.GetImageResolution(tt.filePath)

			if (err != nil) != tt.expectErr {
				t.Errorf("pkg.GetImageResolution() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !tt.expectErr {
				if width != tt.expectedWidth || height != tt.expectedHeight {
					t.Errorf("pkg.GetImageResolution() got_width = %v, got_height = %v, want_width = %v, want_height = %v",
						width, height, tt.expectedWidth, tt.expectedHeight)
				}
			}
		})
	}
}
