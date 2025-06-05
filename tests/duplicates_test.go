package pkg_test

import (
	"github.com/user/photo-sorter/pkg"
	"image"
	"errors"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

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
		pngPath := filepath.Join(tmpDir, "pvj_img.png")
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

	// Sub-test: "Non-image file"
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
		} else if !os.IsNotExist(err) { // Check for underlying os.ErrNotExist
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
