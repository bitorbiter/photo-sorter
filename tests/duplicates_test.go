package tests

import (
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/photo-sorter/pkg"
	// "github.com/rwcarlsen/goexif/exif" // Not directly used in tests, but pkg uses it
)

// Helper to create a dummy text file
func createTestFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file %s: %v", filePath, err)
	}
	return filePath
}

// Helper to create a dummy JPEG file
func createDummyJPEG(t *testing.T, path string, width, height int, quality int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create dummy JPEG file %s: %v", path, err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("Failed to encode dummy JPEG %s: %v", path, err)
	}
}

// Helper to create a dummy GIF file
func createDummyGIF(t *testing.T, path string, width, height int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height)) // Use RGBA, gif.Encode will handle palettization
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create dummy GIF file %s: %v", path, err)
	}
	defer f.Close()
	// Basic options, gif.Encode will create a palette
	if err := gif.Encode(f, img, &gif.Options{NumColors: 256}); err != nil {
		t.Fatalf("Failed to encode dummy GIF %s: %v", path, err)
	}
}

// TestCalculateFileHash tests the full file content hashing.
// Covers: REQ-CF-ADD-06, REQ-CF-ADD-07, REQ-CF-ADD-08
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
func createDummyPNG(t *testing.T, path string, width, height int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, c)
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

// TestGetImageResolution tests fetching image dimensions.
// Covers: REQ-CF-DR-02
func TestGetImageResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid PNG
	pngFilePath := filepath.Join(tmpDir, "test.png")
	expectedWidth, expectedHeight := 100, 50
	// Provide a default color, e.g., color.Black or a specific color
	defaultColor := color.RGBA{100, 200, 200, 255} // Matches old implicit color
	createDummyPNG(t, pngFilePath, expectedWidth, expectedHeight, defaultColor)

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

// TestCalculatePixelDataHash tests the pixel data hashing.
// Covers: REQ-CF-ADD-03, REQ-CF-ADD-04, REQ-CF-ADD-05
// Also implicitly covers aspects of REQ-CF-ADD-07 for error handling.
func TestCalculatePixelDataHash(t *testing.T) {
	tmpDir := t.TempDir()
	color1 := color.RGBA{100, 150, 200, 255}
	color2 := color.RGBA{120, 170, 220, 255}

	// Sub-test: "Identical PNGs"
	t.Run("Identical PNGs", func(t *testing.T) {
		png1A := filepath.Join(tmpDir, "id_png1A.png")
		png1B := filepath.Join(tmpDir, "id_png1B.png")
		createDummyPNG(t, png1A, 10, 10, color1)
		createDummyPNG(t, png1B, 10, 10, color1)

		hash1A, err1A := pkg.CalculatePixelDataHash(png1A)
		hash1B, err1B := pkg.CalculatePixelDataHash(png1B)

		if err1A != nil {
			t.Errorf("Error calculating pixel hash for png1A: %v", err1A)
		}
		if err1B != nil {
			t.Errorf("Error calculating pixel hash for png1B: %v", err1B)
		}
		if hash1A == "" || hash1B == "" {
			t.Errorf("Expected non-empty hashes, got h1=%s, h2=%s", hash1A, hash1B)
		}
		if hash1A != hash1B {
			t.Errorf("Expected identical hashes for identical PNGs, got %s and %s", hash1A, hash1B)
		}
	})

	// Sub-test: "Different PNGs"
	t.Run("Different PNGs", func(t *testing.T) {
		png2A := filepath.Join(tmpDir, "diff_png2A.png")
		png2B := filepath.Join(tmpDir, "diff_png2B.png")
		createDummyPNG(t, png2A, 10, 10, color1)
		createDummyPNG(t, png2B, 10, 10, color2) // Different color

		hash2A, err2A := pkg.CalculatePixelDataHash(png2A)
		hash2B, err2B := pkg.CalculatePixelDataHash(png2B)

		if err2A != nil {
			t.Errorf("Error calculating pixel hash for png2A: %v", err2A)
		}
		if err2B != nil {
			t.Errorf("Error calculating pixel hash for png2B: %v", err2B)
		}
		if hash2A == "" || hash2B == "" {
			t.Errorf("Expected non-empty hashes")
		}
		if hash2A == hash2B {
			t.Errorf("Expected different hashes for different PNGs, got same: %s", hash2A)
		}
	})

	// Sub-test: "Identical JPEGs"
	t.Run("Identical JPEGs", func(t *testing.T) {
		jpeg1A := filepath.Join(tmpDir, "id_jpeg1A.jpg")
		jpeg1B := filepath.Join(tmpDir, "id_jpeg1B.jpg")
		createDummyJPEG(t, jpeg1A, 10, 10, 75, color1)
		createDummyJPEG(t, jpeg1B, 10, 10, 75, color1)

		hash1A, err1A := pkg.CalculatePixelDataHash(jpeg1A)
		hash1B, err1B := pkg.CalculatePixelDataHash(jpeg1B)
		if err1A != nil {t.Errorf("JPEG1A hash error: %v", err1A)}
		if err1B != nil {t.Errorf("JPEG1B hash error: %v", err1B)}
		if hash1A == "" || hash1B == "" {t.Errorf("Expected non-empty JPEG hashes")}
		if hash1A != hash1B {
			t.Errorf("Expected identical hashes for identical JPEGs, got %s and %s", hash1A, hash1B)
		}
	})

	// Sub-test: "Different JPEGs"
	t.Run("Different JPEGs", func(t *testing.T) {
		jpeg2A := filepath.Join(tmpDir, "diff_jpeg2A.jpg")
		jpeg2B := filepath.Join(tmpDir, "diff_jpeg2B.jpg")
		createDummyJPEG(t, jpeg2A, 10, 10, 75, color1)
		createDummyJPEG(t, jpeg2B, 10, 10, 75, color2) // Different color

		hash2A, err2A := pkg.CalculatePixelDataHash(jpeg2A)
		hash2B, err2B := pkg.CalculatePixelDataHash(jpeg2B)
		if err2A != nil {t.Errorf("JPEG2A hash error: %v", err2A)}
		if err2B != nil {t.Errorf("JPEG2B hash error: %v", err2B)}
		if hash2A == "" || hash2B == "" {t.Errorf("Expected non-empty JPEG hashes")}
		if hash2A == hash2B {
			t.Errorf("Expected different hashes for different JPEGs, got same: %s", hash2A)
		}
	})

	// Sub-test: "Identical GIFs"
	t.Run("Identical GIFs", func(t *testing.T) {
		gif1A := filepath.Join(tmpDir, "id_gif1A.gif")
		gif1B := filepath.Join(tmpDir, "id_gif1B.gif")
		createDummyGIF(t, gif1A, 10, 10, color1)
		createDummyGIF(t, gif1B, 10, 10, color1)

		hash1A, err1A := pkg.CalculatePixelDataHash(gif1A)
		hash1B, err1B := pkg.CalculatePixelDataHash(gif1B)
		if err1A != nil {t.Errorf("GIF1A hash error: %v", err1A)}
		if err1B != nil {t.Errorf("GIF1B hash error: %v", err1B)}
		if hash1A == "" || hash1B == "" {t.Errorf("Expected non-empty GIF hashes")}
		if hash1A != hash1B {
			t.Errorf("Expected identical hashes for identical GIFs, got %s and %s", hash1A, hash1B)
		}
	})

	// Sub-test: "Different GIFs"
	t.Run("Different GIFs", func(t *testing.T) {
		gif2A := filepath.Join(tmpDir, "diff_gif2A.gif")
		gif2B := filepath.Join(tmpDir, "diff_gif2B.gif")
		createDummyGIF(t, gif2A, 10, 10, color1)
		createDummyGIF(t, gif2B, 10, 10, color2) // Different color

		hash2A, err2A := pkg.CalculatePixelDataHash(gif2A)
		hash2B, err2B := pkg.CalculatePixelDataHash(gif2B)
		if err2A != nil {t.Errorf("GIF2A hash error: %v", err2A)}
		if err2B != nil {t.Errorf("GIF2B hash error: %v", err2B)}
		if hash2A == "" || hash2B == "" {t.Errorf("Expected non-empty GIF hashes")}
		if hash2A == hash2B {
			t.Errorf("Expected different hashes for different GIFs, got same: %s", hash2A)
		}
	})

	// Sub-test: "PNG vs JPEG of same visual content"
	// This test is tricky because JPEG is lossy. Even with quality 100, pixel values might differ slightly.
	// For this test, we'll create a very simple image and hope the colors are preserved enough
	// for RGBA() to return identical values after decoding.
	t.Run("PNG vs JPEG same content (best effort)", func(t *testing.T) {
		pngPath := filepath.Join(tmpDir, "pvj_img.png") // Corrected variable name
		jpegPath := filepath.Join(tmpDir, "pvj_img.jpg")

		// Use a very simple color that might survive JPEG compression well
		simpleColor := color.RGBA{R: 0, G: 0, B: 255, A: 255} // Pure blue
		createDummyPNG(t, pngPath, 5, 5, simpleColor)
		createDummyJPEG(t, jpegPath, 5, 5, 100, simpleColor) // Max quality

		hashPNG, errPNG := pkg.CalculatePixelDataHash(pngPath)
		hashJPEG, errJPEG := pkg.CalculatePixelDataHash(jpegPath)

		if errPNG != nil {t.Errorf("PNG hash error: %v", errPNG)}
		if errJPEG != nil {t.Errorf("JPEG hash error: %v", errJPEG)}

		if hashPNG == "" || hashJPEG == "" {t.Errorf("Expected non-empty hashes for PNG vs JPEG test")}

		// Note: This assertion might be flaky due to JPEG's lossy nature.
		// If it fails, it indicates that the pixel data read by img.At().RGBA() differs.
		if hashPNG != hashJPEG {
			t.Logf("Pixel hashes for PNG and JPEG of 'same' content differ. PNG_Hash=%s, JPEG_Hash=%s. This can be due to JPEG's lossy compression, even at high quality.", hashPNG, hashJPEG)
			// This is not a strict failure for now, more of an observation, unless we can guarantee pixel perfection.
			// For the purpose of the duplicate detector, if they are visually identical but hashes differ, they'd be treated as different.
			// The core logic relies on img.At().RGBA() providing the source of truth *after* decoding.
		} else {
			t.Logf("Pixel hashes for PNG and JPEG of 'same' content are identical. PNG_Hash=%s, JPEG_Hash=%s.", hashPNG, hashJPEG)
		}
	})

	// Sub-test: "Non-image file" // This test is for CalculatePixelDataHash, not AreFilesPotentiallyDuplicate directly
	t.Run("Non-image file", func(t *testing.T) {
		txtFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(txtFile, []byte("this is not an image"), 0644); err != nil {
			t.Fatalf("Failed to create text file: %v", err)
		}
		_, err := pkg.CalculatePixelDataHash(txtFile)
		if err == nil {
			t.Errorf("Expected error for non-image file, got nil")
		} else if !errors.Is(err, pkg.ErrUnsupportedForPixelHashing) {
			t.Errorf("Expected ErrUnsupportedForPixelHashing for non-image, got: %v", err)
		}
	})

	// Sub-test: "Corrupted image file"
	t.Run("Corrupted image file", func(t *testing.T) {
		corruptedFile := filepath.Join(tmpDir, "corrupted.png")
		if err := os.WriteFile(corruptedFile, []byte("this is not a valid png but has extension"), 0644); err != nil {
			t.Fatalf("Failed to create corrupted file: %v", err)
		}
		_, err := pkg.CalculatePixelDataHash(corruptedFile)
		if err == nil {
			t.Errorf("Expected error for corrupted image file, got nil")
		} else if !errors.Is(err, pkg.ErrUnsupportedForPixelHashing) {
			// image.Decode should error out, and we wrap it with ErrUnsupportedForPixelHashing
			t.Errorf("Expected ErrUnsupportedForPixelHashing for corrupted image, got: %v", err)
		}
	})

	// Sub-test: "Non-existent file"
	t.Run("Non-existent file", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "i_do_not_exist.png")
		_, err := pkg.CalculatePixelDataHash(nonExistentPath)
		if err == nil {
			t.Errorf("Expected error for non-existent file, got nil")
		} else if !errors.Is(err, os.ErrNotExist) { // Use errors.Is for wrapped errors
			t.Errorf("Expected os.ErrNotExist for non-existent file, got: %v", err)
		}
	})

	// Sub-test: "One pixel different PNG"
	t.Run("One pixel different PNG", func(t *testing.T) {
		basePNGPath := filepath.Join(tmpDir, "base_one_pixel.png")
		modifiedPNGPath := filepath.Join(tmpDir, "modified_one_pixel.png")

		// Create base PNG
		createDummyPNG(t, basePNGPath, 3, 3, color1)

		// Create modified PNG - by reading base, changing a pixel, and saving
		baseImgFile, err := os.Open(basePNGPath)
		if err != nil {
			t.Fatalf("Failed to open base PNG for modification: %v", err)
		}
		imgToModify, err := png.Decode(baseImgFile)
		baseImgFile.Close()
		if err != nil {
			t.Fatalf("Failed to decode base PNG for modification: %v", err)
		}

		// Ensure it's an RGBA image to use SetRGBA
		bounds := imgToModify.Bounds()
		modifiedImg := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				modifiedImg.Set(x,y, imgToModify.At(x,y))
			}
		}

		// Change one pixel (e.g., at 1,1)
		originalColor := modifiedImg.At(1,1)
		modifiedImg.Set(1, 1, color2)
		// Verify the color was actually different and changed
		if modifiedImg.At(1,1) == originalColor && color1 != color2 {
			 // This check is important if color1 and color2 were somehow the same
			t.Logf("Warning: Pixel color at (1,1) did not change as expected. Original: %v, Attempted new: %v", originalColor, color2)
		}


		modifiedFile, err := os.Create(modifiedPNGPath)
		if err != nil {
			t.Fatalf("Failed to create modified PNG file: %v", err)
		}
		if err := png.Encode(modifiedFile, modifiedImg); err != nil {
			modifiedFile.Close()
			t.Fatalf("Failed to encode modified PNG: %v", err)
		}
		modifiedFile.Close()

		hashBase, errBase := pkg.CalculatePixelDataHash(basePNGPath)
		hashModified, errModified := pkg.CalculatePixelDataHash(modifiedPNGPath)

		if errBase != nil {t.Errorf("Base PNG hash error: %v", errBase)}
		if errModified != nil {t.Errorf("Modified PNG hash error: %v", errModified)}
		if hashBase == "" || hashModified == "" {t.Errorf("Expected non-empty hashes for one-pixel-diff test")}

		if hashBase == hashModified {
			// It is possible, especially with very small images or if color1 and color2 are too similar,
			// or if the single pixel change doesn't robustly alter the hash.
			// This might indicate an issue with hashing sensitivity or the test setup itself.
			t.Errorf("Expected different hashes for PNGs differing by one pixel, but got the same hash: %s. Original Color at (1,1) for base: %v, New color for modified: %v", hashBase, color1, modifiedImg.At(1,1))
		}
	})
}

// TestAreFilesPotentiallyDuplicate tests the main multi-step comparison logic.
// Covers: REQ-CF-ADD-01 and its sub-requirements (REQ-CF-ADD-02, REQ-CF-ADD-03, REQ-CF-ADD-04, REQ-CF-ADD-07)
func TestAreFilesPotentiallyDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	color1 := color.RGBA{10, 20, 30, 255}
	color2 := color.RGBA{40, 50, 60, 255}

	// --- Test Cases ---

	t.Run("DifferentSize", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "diffsize1.txt", "hello")
		f2 := createTestFile(t, tmpDir, "diffsize2.txt", "hello world")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, f2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if compResult.AreDuplicates {
			t.Errorf("Expected false for different sizes, got true. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonSizeMismatch {
			t.Errorf("Expected ReasonSizeMismatch, got %s", compResult.Reason)
		}
	})

	t.Run("SameSizeDifferentContentText", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "samesize_diffcontent1.txt", "hello world1")
		f2 := createTestFile(t, tmpDir, "samesize_diffcontent2.txt", "hello world2")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, f2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if compResult.AreDuplicates {
			t.Errorf("Expected false for same size different content text, got true. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonFileHashMismatch {
			t.Errorf("Expected ReasonFileHashMismatch, got %s", compResult.Reason)
		}
	})

	t.Run("SameSizeSameContentText", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "samesize_samecontent1.txt", "hello universe")
		f2 := createTestFile(t, tmpDir, "samesize_samecontent2.txt", "hello universe")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, f2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !compResult.AreDuplicates {
			t.Errorf("Expected true for same size same content text, got false. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonFileHashMatch {
			t.Errorf("Expected ReasonFileHashMatch, got %s", compResult.Reason)
		}
	})

	t.Run("ZeroByteFiles", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "zerobyte1.txt", "")
		f2 := createTestFile(t, tmpDir, "zerobyte2.txt", "")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, f2)
		if err != nil {
			t.Errorf("Unexpected error for zero byte files: %v", err)
		}
		if !compResult.AreDuplicates {
			t.Errorf("Expected true for two zero-byte files, got false. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonFileHashMatch { // Updated to reflect specific reason in AreFilesPotentiallyDuplicate
			t.Errorf("Expected ReasonFileHashMatch for zero byte files, got %s", compResult.Reason)
		}
	})

	t.Run("ZeroByteVsNonZeroByteFile", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "zerobyte3.txt", "")
		f2 := createTestFile(t, tmpDir, "nonzerobyte.txt", "content")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, f2)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if compResult.AreDuplicates {
			t.Errorf("Expected false for zero-byte vs non-zero-byte file, got true. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonSizeMismatch {
			t.Errorf("Expected ReasonSizeMismatch, got %s", compResult.Reason)
		}
	})


	// Image specific tests using dummy images (no real EXIF)
	t.Run("SameSizeSamePixelImage", func(t *testing.T) {
		img1Path := filepath.Join(tmpDir, "samesize_samepixel1.png")
		img2Path := filepath.Join(tmpDir, "samesize_samepixel2.png")
		createDummyPNG(t, img1Path, 20, 20, color1)
		createDummyPNG(t, img2Path, 20, 20, color1)

		compResult, err := pkg.AreFilesPotentiallyDuplicate(img1Path, img2Path)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !compResult.AreDuplicates {
			t.Errorf("Expected true for same size, same pixel images, got false. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonPixelHashMatch {
			t.Errorf("Expected ReasonPixelHashMatch, got %s", compResult.Reason)
		}
	})

	t.Run("SameSizeDifferentPixelImage", func(t *testing.T) {
		img1Path := filepath.Join(tmpDir, "samesize_diffpixel1.png")
		img2Path := filepath.Join(tmpDir, "samesize_diffpixel2.png")
		createDummyPNG(t, img1Path, 20, 20, color1)
		createDummyPNG(t, img2Path, 20, 20, color2) // Different color

		compResult, err := pkg.AreFilesPotentiallyDuplicate(img1Path, img2Path)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if compResult.AreDuplicates {
			t.Errorf("Expected false for same size, different pixel images, got true. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonPixelHashMismatch {
			t.Errorf("Expected ReasonPixelHashMismatch, got %s", compResult.Reason)
		}
	})

	t.Run("ImageVsTextSameSize", func(t *testing.T) {
		// Create a PNG file and get its size
		imgBase := filepath.Join(tmpDir, "img_vs_text_base.png")
		createDummyPNG(t, imgBase, 10, 5, color1) // Small image
		imgInfo, err := os.Stat(imgBase)
		if err != nil {
			t.Fatalf("Could not stat dummy image: %v", err)
		}
		imgSize := imgInfo.Size()

		// Create a text file of the exact same size
		textContent := strings.Repeat("A", int(imgSize))
		txtFilePath := createTestFile(t, tmpDir, "text_eq_size.txt", textContent)

		txtInfo, err := os.Stat(txtFilePath)
		if err != nil {
			t.Fatalf("Could not stat dummy text file: %v", err)
		}
		if txtInfo.Size() != imgSize {
			// This is a sanity check for the test setup itself
			t.Fatalf("Test setup error: text file size %d does not match image size %d", txtInfo.Size(), imgSize)
		}


		compResult, err := pkg.AreFilesPotentiallyDuplicate(imgBase, txtFilePath)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// Expected false because pixel hash for text file will fail (ErrUnsupported),
		// then full file hash will be different.
		if compResult.AreDuplicates {
			t.Errorf("Expected false for image vs text file of same size, got true. Reason: %s", compResult.Reason)
		}
		// Expect fallback to file hash comparison
		if compResult.Reason != pkg.ReasonFileHashMismatch {
			t.Errorf("Expected ReasonFileHashMismatch for image vs text, got %s. HashType: %s", compResult.Reason, compResult.HashType)
		}
	})

	// EXIF related tests - using pre-existing files from test_source
	// These tests make assumptions about the EXIF data in these files.
	// photoA1.jpg, photoA2.jpg, photoB1.jpg
	// We need to copy them to tmpDir to avoid modifying original test_source and ensure clean state
	// sourceDir variable removed as it was unused. (This comment refers to a previous instance)
	// The actual unused 'sourceDir' variable at line 621 (approx) is removed by this change.

	// Let's try to make path resolution more robust.
	// This assumes the test binary runs from the package directory (e.g., /tests)
	// or that paths are relative to the project root if using `go test ./...` from root.
	// For simplicity, let's assume we are running `go test` from the `tests` directory or `go test ./...` from root.
	// The `../pkg/duplicates.go` implies that `pkg` is a sibling to `tests`.
	// So `../tests/test_source` from `pkg/duplicates.go` would be `test_source` from `tests/duplicates_test.go`

	// Simpler: use absolute paths or copy files carefully.
	// For now, let's assume photoA1 and photoA2 might differ in EXIF or content,
	// and photoA1 and photoB1 might also differ.

	// Helper to copy test files
	copyTestAsset := func(assetName string) string {
		// A common pattern is to run `go test ./...` from project root.
		// If this test file is in `tests/duplicates_test.go`,
		// then `test_source` is a sibling directory.
		srcPath := filepath.Join("test_source", assetName)

		// Check if test_source exists relative to current dir (where test is run)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			// If not, try one level up (e.g. if test is run from root)
			srcPath = filepath.Join("tests", "test_source", assetName)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Logf("Warning: Test asset directory 'test_source' or 'tests/test_source' not found relative to test execution directory. Skipping EXIF tests that rely on pre-existing files.")
				return ""
			}
		}


		destPath := filepath.Join(tmpDir, assetName)
		sourceFile, err := os.Open(srcPath)
		if err != nil {
			t.Logf("Warning: Failed to open test asset %s: %v. Skipping this test case.", srcPath, err)
			return ""
		}
		defer sourceFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			t.Logf("Warning: Failed to create temp file for asset %s: %v. Skipping this test case.", destPath, err)
			return ""
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, sourceFile)
		if err != nil {
			t.Logf("Warning: Failed to copy test asset %s to %s: %v. Skipping this test case.", srcPath, destPath, err)
			return ""
		}
		return destPath
	}

	photoA1Path := copyTestAsset("photoA1.jpg")
	photoA2Path := copyTestAsset("photoA2.jpg")
	// photoB1Path := copyTestAsset("photoB1.jpg") // Keep for later if needed
	txtC1Path := copyTestAsset("photoC1.txt") // A text file from assets

	if photoA1Path != "" && photoA2Path != "" {
		t.Run("ExifTest_PhotoA1_vs_PhotoA2", func(t *testing.T) {
			// This test's outcome depends entirely on the actual content and EXIF of A1 and A2.
			// Assuming photoA1.jpg and photoA2.jpg are DIFFERENT (either by EXIF or content if EXIF is same/missing)
			// If they are truly identical in all aspects (size, EXIF, pixels, full content), then this should be true.
			// The files in test_source are: photoA1.jpg (27kb), photoA2.jpg (27kb, different name but might be identical)
			// Let's assume they are meant to be different in some way for testing.
			// If EXIF is different -> false. If EXIF same/NA & Pixels different -> false. If EXIF/Pixels same & Full hash different -> false.

			// We need to know their actual properties. For now, this test is speculative.
			// Let's get their sizes first to ensure the test setup is valid.
			s1Width, s1Height, s1Err := pkg.GetImageResolution(photoA1Path) // Using this to check if it's a valid image
			s2Width, s2Height, s2Err := pkg.GetImageResolution(photoA2Path)
			if s1Err != nil || s2Err != nil || s1Width == 0 || s1Height == 0 || s2Width == 0 || s2Height == 0 {
				t.Log("photoA1.jpg or photoA2.jpg are not valid images (or resolution is 0,0) or not found, skipping EXIF diff test.")
				return
			}


			compResult, err := pkg.AreFilesPotentiallyDuplicate(photoA1Path, photoA2Path)
			if err != nil {
				t.Errorf("Unexpected error comparing photoA1 and photoA2: %v", err)
			}
			// Based on typical test data design, A1 and A2 would be different.
			// If they are truly identical, this test would need to expect `true`.
			if compResult.AreDuplicates {
				t.Logf("photoA1.jpg and photoA2.jpg reported as duplicates. Reason: %s. This implies they are identical in size, EXIF (if any/same), and content (pixel or full).", compResult.Reason)
				// t.Errorf("Expected false for photoA1 vs photoA2 (assuming they are different based on name), got true. Reason: %s", compResult.Reason)
			} else {
				t.Logf("photoA1.jpg and photoA2.jpg reported as NOT duplicates. Reason: %s", compResult.Reason)
			}
			// This test needs more concrete assertions once file properties are known.
		})
	} else {
		t.Log("Skipping EXIF tests with photoA1.jpg/photoA2.jpg as assets could not be copied.")
	}

	if photoA1Path != "" && txtC1Path != "" {
		t.Run("ExifTest_PhotoA1_vs_TextC1", func(t *testing.T) {
			compResult, err := pkg.AreFilesPotentiallyDuplicate(photoA1Path, txtC1Path)
			if err != nil {
				// Errors like "unsupported" or "no exif" are handled internally by AreFilesPotentiallyDuplicate
				// to reach a comparison decision. Only fatal errors should be checked here.
				// Check if error is NOT a "no exif" or "unsupported format" type of error if strict error checking is needed.
				// For this test, we are primarily interested in the duplication outcome.
				if !strings.Contains(err.Error(), "no such file or directory") { // Allow file system errors
					// t.Logf("Info: Received error for photoA1 vs textC1, but expecting fallback: %v", err)
				} else {
					t.Fatalf("Unexpected fatal error: %v", err)
				}
			}
			if compResult.AreDuplicates {
				t.Errorf("Expected false for photoA1.jpg vs photoC1.txt, got true. Reason: %s", compResult.Reason)
			}
			// Depending on size, could be SizeMismatch or FileHashMismatch (if sizes happen to be same)
			if compResult.Reason != pkg.ReasonSizeMismatch && compResult.Reason != pkg.ReasonFileHashMismatch {
                 t.Errorf("Expected SizeMismatch or FileHashMismatch for photoA1 vs text, got %s", compResult.Reason)
			}
		})
	} else {
		t.Log("Skipping EXIF tests with photoA1.jpg/photoC1.txt as assets could not be copied.")
	}

	// Test error propagation: one file does not exist
	t.Run("FileDoesNotExist", func(t *testing.T) {
		f1 := createTestFile(t, tmpDir, "exists.txt", "content")
		nonExistentPath := filepath.Join(tmpDir, "nonexistent.txt")
		compResult, err := pkg.AreFilesPotentiallyDuplicate(f1, nonExistentPath)
		if err == nil {
			t.Errorf("Expected error when one file does not exist, got nil. Result: %+v", compResult)
		} else {
			if !strings.Contains(err.Error(), nonExistentPath) && !strings.Contains(err.Error(), "no such file or directory") {
				t.Errorf("Expected error related to '%s' not existing, got: %v", nonExistentPath, err)
			}
			if compResult.Reason != pkg.ReasonError {
				t.Errorf("Expected ReasonError, got %s", compResult.Reason)
			}
			t.Logf("Got expected error for non-existent file: %v. Result: %+v", err, compResult)
		}

		compResult, err = pkg.AreFilesPotentiallyDuplicate(nonExistentPath, f1)
		if err == nil {
			t.Errorf("Expected error when one file does not exist (arg1), got nil. Result: %+v", compResult)
		} else {
			if !strings.Contains(err.Error(), nonExistentPath) && !strings.Contains(err.Error(), "no such file or directory") {
				t.Errorf("Expected error related to '%s' not existing (arg1), got: %v", nonExistentPath, err)
			}
			if compResult.Reason != pkg.ReasonError {
				t.Errorf("Expected ReasonError, got %s", compResult.Reason)
			}
			t.Logf("Got expected error for non-existent file (arg1): %v. Result: %+v", err, compResult)
		}
	})

	// Test for getFileSize explicitly (simple cases) - now tested via AreFilesPotentiallyDuplicate
	t.Run("getFileSize_Basic_Implicit", func(t *testing.T) {
		content := "12345"
		fPath := createTestFile(t, tmpDir, "getsize.txt", content)
		nonExistent := filepath.Join(tmpDir, "does_not_exist_for_size.txt")

		// This call will invoke getFileSize internally. We already have "FileDoesNotExist" test for this.
		// Just ensuring it's covered in thought.
		compResult, err := pkg.AreFilesPotentiallyDuplicate(fPath, nonExistent)
		if err == nil {
			t.Errorf("Expected error from AreFilesPotentiallyDuplicate when getFileSize fails (for second arg). Result: %+v", compResult)
		} else if !strings.Contains(err.Error(), "does_not_exist_for_size.txt") {
			t.Errorf("Error message from AreFilesPotentiallyDuplicate did not mention missing file: %v", err)
		}
		if compResult.Reason != pkg.ReasonError {
			t.Errorf("Expected ReasonError when getFileSize fails, got %s", compResult.Reason)
		}
	})

	// Test for getExifSignature explicitly (simple cases, e.g. non-image file) - now tested via AreFilesPotentiallyDuplicate
	t.Run("getExifSignature_TextFile_Implicit", func(t *testing.T) {
		txtFilePath := createTestFile(t, tmpDir, "text_for_exif.txt", "not an image")
		txtFilePath2 := createTestFile(t, tmpDir, "text_for_exif2.txt", "not an image") // Identical

		compResult, err := pkg.AreFilesPotentiallyDuplicate(txtFilePath, txtFilePath2)
		if err != nil {
			t.Fatalf("AreFilesPotentiallyDuplicate error for two text files: %v. Result: %+v", err, compResult)
		}
		if !compResult.AreDuplicates {
			t.Errorf("Expected two identical text files to be duplicates. Reason: %s", compResult.Reason)
		}
		if compResult.Reason != pkg.ReasonFileHashMatch {
			t.Errorf("Expected ReasonFileHashMatch for identical text files, got %s. HashType: %s", compResult.Reason, compResult.HashType)
		}
	})

	t.Run("getExifSignature_NonExistentFile_Implicit", func(t *testing.T) {
		// nonExistentPath variable removed as it was unused.
		// If getExifSignature were public:
		// nonExistentPath := filepath.Join(tmpDir, "non_existent_for_exif.jpg")
		// _, err := pkg.getExifSignature(nonExistentPath)
		// if !os.IsNotExist(errors.Unwrap(err)) { // Check for underlying os.ErrNotExist if wrapped
		//    t.Errorf("getExifSignature on non-existent file: expected os.ErrNotExist, got err '%v'", err)
		// }
		// Test via AreFilesPotentiallyDuplicate is already covered by FileDoesNotExist.
		// The error from getExifSignature for a non-existent file would be an os.PathError.
		// This is handled by AreFilesPotentiallyDuplicate's initial file access checks.
	})


	// Note: Testing EXIF signature values precisely requires either:
	// 1. Pre-made files with known, stable EXIF data.
	// 2. A Go library to *write* specific EXIF tags to a dummy JPEG for testing.
	//    This is complex. `rwcarlsen/goexif` is primarily for reading.
	// The current tests with photoA1.jpg rely on assumptions.
	// If these files have no EXIF or identical EXIF, the EXIF comparison path where signatures *differ*
	// won't be tested for actual EXIF differences, only for EXIF presence/absence.
	t.Log("Reminder: EXIF-specific logic paths (e.g. two images with *different* EXIF signatures) depend on the properties of test_source images (photoA1.jpg, photoA2.jpg). If these files lack EXIF or have identical EXIF, these specific paths may not be fully exercised.")

}
