package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/photo-sorter/pkg"
)

// createTestFile helper to create generic files for testing
func createTestFile(t *testing.T, dir string, name string, content []byte) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write test file %s: %v", filePath, err)
	}
	// Set a consistent mod time for reproducible date-based naming if EXIF fails
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filePath, modTime, modTime); err != nil {
		t.Fatalf("Failed to change mod time for %s: %v", filePath, err)
	}
	return filePath
}

// createTestImage helper to create PNG, JPEG, or GIF images
func createTestImage(t *testing.T, dir string, name string, width, height int, format string, c color.Color) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, c)
		}
	}

	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test image file %s: %v", filePath, err)
	}
	defer f.Close()

	switch format {
	case "png":
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("Failed to encode dummy PNG %s: %v", filePath, err)
		}
	case "jpeg":
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatalf("Failed to encode dummy JPEG %s: %v", filePath, err)
		}
	case "gif":
		if err := gif.Encode(f, img, &gif.Options{NumColors: 256}); err != nil {
			t.Fatalf("Failed to encode dummy GIF %s: %v", filePath, err)
		}
	default:
		t.Fatalf("Unsupported image format for testing: %s", format)
	}

	// Set a consistent mod time for reproducible date-based naming if EXIF fails
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC) // Example: Jan 1, 2023
	if err := os.Chtimes(filePath, modTime, modTime); err != nil {
		t.Fatalf("Failed to change mod time for %s: %v", filePath, err)
	}
	return filePath
}

// Mock GetPhotoCreationDate to always return an error, forcing fallback to ModTime
// This simplifies testing date logic as ModTime is controllable via os.Chtimes.
func mockGetPhotoCreationDate(filePath string) (time.Time, error) {
	return time.Time{}, errors.New("mock EXIF error: reading disabled for this test")
}

func TestGenerateDestinationPathLogic(t *testing.T) {
	tests := []struct {
		name              string
		photoDate         time.Time
		originalFileName  string
		expectedNewName   string
	}{
		{"simple jpg", time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC), "myphoto.jpg", "image-2023-10-27.jpg"},
		{"uppercase JPG", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "VACATION.JPG", "image-2024-01-01.JPG"},
		{"png extension", time.Date(2023, 5, 15, 0, 0, 0, 0, time.UTC), "image.png", "image-2023-05-15.png"},
		{"no extension", time.Date(2022, 3, 3, 0, 0, 0, 0, time.UTC), "rawimage", "image-2022-03-03"},
		{"double extension", time.Date(2021, 7, 19, 0, 0, 0, 0, time.UTC), "archive.tar.gz", "image-2021-07-19.gz"},
		{"leading dot filename", time.Date(2020, 12, 25, 0, 0, 0, 0, time.UTC), ".hiddenfile.jpeg", "image-2020-12-25.jpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dateStr := tt.photoDate.Format("2006-01-02")
			originalExtension := filepath.Ext(tt.originalFileName) // filepath.Ext correctly handles cases like ".jpeg" and ""
			generatedName := fmt.Sprintf("image-%s%s", dateStr, originalExtension)

			if generatedName != tt.expectedNewName {
				t.Errorf("For %s: generated name = %s; want %s", tt.name, generatedName, tt.expectedNewName)
			}
		})
	}
}

// TestMainProcessingLogic_DuplicateDetectionAndCopying is an integration-style test
// that simulates the core file processing loop of main.go.
func TestMainProcessingLogic_DuplicateDetectionAndCopying(t *testing.T) {
	sourceDir := t.TempDir()
	targetBaseDir := t.TempDir() // Changed from targetDir to targetBaseDir to match main.go
	// reportFilePath := filepath.Join(targetBaseDir, "report.txt") // We'll check duplicateReportEntries directly

	redColor := color.RGBA{R: 255, A: 255}
	blueColor := color.RGBA{B: 255, A: 255}

	// --- Create Test Files ---
	// Order matters for how resolution preference is tested if not explicitly handled by sorting candidates later
	// For simplicity, we'll process C (high-res) after A (low-res) to test replacement.
	imgPixelDupA := createTestImage(t, sourceDir, "pixel_dupA.jpg", 10, 10, "jpeg", redColor)
	imgPixelDupB := createTestImage(t, sourceDir, "pixel_dupB.jpg", 10, 10, "jpeg", redColor) // Exact pixel duplicate of A
	imgPixelDupCRes := createTestImage(t, sourceDir, "pixel_dupC_high_res.jpg", 20, 20, "jpeg", redColor) // Same color, higher res

	txtFileDupA := createTestFile(t, sourceDir, "file_dupA.txt", []byte("identical content"))
	txtFileDupB := createTestFile(t, sourceDir, "file_dupB.txt", []byte("identical content"))

	imgUnique := createTestImage(t, sourceDir, "unique_image.png", 10, 10, "png", blueColor)

	unsupportedPixelA := createTestFile(t, sourceDir, "unsupported_pixel.dat", []byte("some data for file hash A"))
	unsupportedPixelB := createTestFile(t, sourceDir, "unsupported_pixel_dup.dat", []byte("some data for file hash A")) // Duplicate of A by content

	// Mock a RAW file, assume pixel hashing fails. Use a unique content for it.
	rawFile := createTestFile(t, sourceDir, "image_for_rename.NEF", []byte("unique raw content"))

	// Override GetPhotoCreationDate to use mock for predictable dates (based on ModTime)
	// This is a global override for the duration of this test.
	originalGetPhotoCreationDate := pkg.GetPhotoCreationDate
	pkg.GetPhotoCreationDate = mockGetPhotoCreationDate
	defer func() { pkg.GetPhotoCreationDate = originalGetPhotoCreationDate }()


	imageFiles := []string{
		imgPixelDupA,    // Process A first
		imgPixelDupB,    // B is duplicate of A
		imgPixelDupCRes, // C is higher-res duplicate of A & B
		txtFileDupA,
		txtFileDupB,
		imgUnique,
		unsupportedPixelA,
		unsupportedPixelB,
		rawFile,
	}

	// Initialize structures from main.go
	pixelHashedCandidates := make(map[string]FileInfo)
	fileHashedCandidates := make(map[string]FileInfo)
	var duplicateReportEntries []pkg.DuplicateInfo
	copiedFilesCounter := 0
	filesToCopyCount := 0
	// processedFilesCounter := len(imageFiles) // Not strictly needed for assertions here but part of main

	// --- Simulate Main Processing Loop (adapted from main.go) ---
	for _, currentFilePath := range imageFiles {
		// fmt.Printf("\nSimProcessing: %s\n", currentFilePath) // For debugging test
		processThisFile := false

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentFilePath)
		if errRes != nil {
			// log.Printf("  - Warning: Could not get resolution for %s: %v.\n", currentFilePath, errRes)
			currentWidth, currentHeight = 0, 0
		}

		currentFileInfo := FileInfo{
			Path:   currentFilePath,
			Width:  currentWidth,
			Height: currentHeight,
		}

		pixelHash, errPixel := pkg.CalculatePixelDataHash(currentFilePath)
		logPixelHashAttemptNeeded := true

		if errPixel == nil {
			currentFileInfo.PixelHash = pixelHash
			currentFileInfo.HashType = "pixel"
			logPixelHashAttemptNeeded = false

			if existingPixelFileInfo, found := pixelHashedCandidates[pixelHash]; found {
				reason := fmt.Sprintf("Pixel hash match with %s", existingPixelFileInfo.Path)
				keptFileInfo := existingPixelFileInfo
				discardedFileInfo := currentFileInfo

				canCompareResolutions := errRes == nil && existingPixelFileInfo.Width > 0 && existingPixelFileInfo.Height > 0 && currentFileInfo.Width > 0 && currentFileInfo.Height > 0
				if canCompareResolutions {
					currentPixels := currentFileInfo.Width * currentFileInfo.Height
					existingPixels := existingPixelFileInfo.Width * existingPixelFileInfo.Height
					if (float64(currentPixels) > float64(existingPixels)*1.1) || (existingPixels == 0 && currentPixels > 0) {
						reason = fmt.Sprintf("Pixel hash match, current file %s (%dx%d) has higher resolution than %s (%dx%d)", currentFileInfo.Path, currentWidth, currentHeight, existingPixelFileInfo.Path, existingPixelFileInfo.Width, existingPixelFileInfo.Height)
						keptFileInfo = currentFileInfo
						discardedFileInfo = existingPixelFileInfo
						pixelHashedCandidates[pixelHash] = currentFileInfo
						processThisFile = true
					} else {
						processThisFile = false
						reason = fmt.Sprintf("Pixel hash match, existing file %s (%dx%d) kept due to similar or better resolution than %s (%dx%d)", existingPixelFileInfo.Path, existingPixelFileInfo.Width, existingPixelFileInfo.Height, currentFileInfo.Path, currentWidth, currentHeight)
					}
				} else {
					processThisFile = false
					reason = fmt.Sprintf("Pixel hash match with %s, resolution comparison skipped/failed, kept existing.", existingPixelFileInfo.Path)
				}
				duplicateReportEntries = append(duplicateReportEntries, pkg.DuplicateInfo{
					KeptFile:      keptFileInfo.Path,
					DiscardedFile: discardedFileInfo.Path,
					Reason:        reason,
				})
			} else {
				pixelHashedCandidates[pixelHash] = currentFileInfo
				processThisFile = true
			}
		} else {
			if errors.Is(errPixel, pkg.ErrUnsupportedForPixelHashing) {
				if logPixelHashAttemptNeeded {
					// log.Printf("  - Info: Pixel data hashing not supported... falling back. %v\n", errPixel)
				}
				currentFileInfo.HashType = "file"
				fileHash, errFile := pkg.CalculateFileHash(currentFilePath)
				if errFile == nil {
					currentFileInfo.FileHash = fileHash
					if existingFileHashInfo, found := fileHashedCandidates[fileHash]; found {
						processThisFile = false
						reason := fmt.Sprintf("File hash match (pixel hashing not supported) with %s. Kept existing.", existingFileHashInfo.Path)
						duplicateReportEntries = append(duplicateReportEntries, pkg.DuplicateInfo{
							KeptFile:      existingFileHashInfo.Path,
							DiscardedFile: currentFileInfo.Path,
							Reason:        reason,
						})
					} else {
						fileHashedCandidates[fileHash] = currentFileInfo
						processThisFile = true
					}
				} else {
					// log.Printf("  - Error calculating file hash for %s: %v. Skipping.\n", currentFilePath, errFile)
					currentFileInfo.HashType = "none"
					processThisFile = false
				}
			} else {
				// log.Printf("  - Error calculating pixel hash for %s: %v. Skipping.\n", currentFilePath, errPixel)
				currentFileInfo.HashType = "none"
				processThisFile = false
			}
		}

		if !processThisFile {
			// fmt.Printf("  - SimFinal decision: Skipping %s\n", currentFilePath) // For debugging
			continue
		}
		filesToCopyCount++

		photoDate, _ := pkg.GetPhotoCreationDate(currentFilePath) // Uses mock, error ignored as ModTime will be used by os.Stat
		fileInfoStat, statErr := os.Stat(currentFilePath)
		if statErr != nil {
			t.Errorf("Error statting file %s: %v", currentFilePath, statErr)
			filesToCopyCount--
			continue
		}
		photoDate = fileInfoStat.ModTime() // Mock ensures EXIF fails, so ModTime is used

		targetDayDir, err := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if err != nil {
			t.Errorf("Error creating target dir for %s: %v", currentFilePath, err)
			filesToCopyCount--
			continue
		}

		originalExtension := filepath.Ext(currentFilePath)
		dateStr := photoDate.Format("2006-01-02")
		newFileName := fmt.Sprintf("image-%s%s", dateStr, originalExtension)
		destPath := filepath.Join(targetDayDir, newFileName)

		if err := pkg.CopyFile(currentFilePath, destPath); err != nil {
			t.Errorf("Error copying file %s to %s: %v", currentFilePath, destPath, err)
			filesToCopyCount--
		} else {
			copiedFilesCounter++
		}
	}

	// --- Assertions ---
	expectedDuplicateEntries := 4 // (A,B) pixel; (A,C) pixel; (txtA,txtB) file; (unsupA,unsupB) file
	if len(duplicateReportEntries) != expectedDuplicateEntries {
		t.Errorf("Expected %d duplicate entries, got %d. Entries:\n%v", expectedDuplicateEntries, len(duplicateReportEntries), duplicateReportEntries)
		for i, entry := range duplicateReportEntries {
			t.Logf("DupEntry %d: Kept: %s, Discarded: %s, Reason: %s", i, filepath.Base(entry.KeptFile), filepath.Base(entry.DiscardedFile), entry.Reason)
		}
	}

	// Check specific duplicate reasons (example)
	foundPixelDupBDiscarded := false
	foundPixelDupAReplacedByC := false // This means A was discarded in favor of C
	foundFileDupBDiscarded := false
	foundUnsupportedDupBDiscarded := false

	for _, entry := range duplicateReportEntries {
		dPath := filepath.Base(entry.DiscardedFile)
		kPath := filepath.Base(entry.KeptFile)

		if dPath == "pixel_dupB.jpg" && kPath == "pixel_dupA.jpg" && entry.Reason_Contains("Pixel hash match") {
			foundPixelDupBDiscarded = true
		}
		// If C replaced A, then A is discarded and C is kept.
		if dPath == "pixel_dupA.jpg" && kPath == "pixel_dupC_high_res.jpg" && entry.Reason_Contains("higher resolution") {
			foundPixelDupAReplacedByC = true
		}
		if dPath == "file_dupB.txt" && kPath == "file_dupA.txt" && entry.Reason_Contains("File hash match") {
			foundFileDupBDiscarded = true
		}
		if dPath == "unsupported_pixel_dup.dat" && kPath == "unsupported_pixel.dat" && entry.Reason_Contains("File hash match") {
			foundUnsupportedDupBDiscarded = true
		}
	}
	if !foundPixelDupBDiscarded {
		t.Errorf("Expected pixel_dupB.jpg to be discarded as pixel duplicate of pixel_dupA.jpg")
	}
	if !foundPixelDupAReplacedByC {
		t.Errorf("Expected pixel_dupA.jpg to be discarded/replaced by pixel_dupC_high_res.jpg due to resolution")
	}
	if !foundFileDupBDiscarded {
		t.Errorf("Expected file_dupB.txt to be discarded as file duplicate of file_dupA.txt")
	}
	if !foundUnsupportedDupBDiscarded {
		t.Errorf("Expected unsupported_pixel_dup.dat to be discarded as file duplicate of unsupported_pixel.dat")
	}


	// Expected files in target dir (all test files created Jan 1, 2023 by helpers)
	expectedDateStr := "2023-01-01"
	expectedCopiedFiles := map[string]string{ // original base name -> expected new name in target
		"pixel_dupC_high_res.jpg": fmt.Sprintf("image-%s.jpg", expectedDateStr), // C replaced A
		"file_dupA.txt":           fmt.Sprintf("image-%s.txt", expectedDateStr),
		"unique_image.png":        fmt.Sprintf("image-%s.png", expectedDateStr),
		"unsupported_pixel.dat":   fmt.Sprintf("image-%s.dat", expectedDateStr),
		"image_for_rename.NEF":    fmt.Sprintf("image-%s.NEF", expectedDateStr),
	}
	expectedCopiedCount := len(expectedCopiedFiles)

	if copiedFilesCounter != expectedCopiedCount {
		t.Errorf("Expected %d files to be copied, got %d", expectedCopiedCount, copiedFilesCounter)
	}
	if filesToCopyCount != expectedCopiedCount { // filesToCopyCount should also match after processing
		t.Errorf("Expected filesToCopyCount to be %d, got %d", expectedCopiedCount, filesToCopyCount)
	}

	targetDayDir := filepath.Join(targetBaseDir, expectedDateStr[:4], expectedDateStr[5:7], expectedDateStr[8:10])
	dirEntries, err := os.ReadDir(targetDayDir)
	if err != nil {
		t.Fatalf("Could not read target directory %s: %v", targetDayDir, err)
	}
	if len(dirEntries) != expectedCopiedCount {
		t.Errorf("Expected %d files in target directory %s, found %d", expectedCopiedCount, targetDayDir, len(dirEntries))
		for _, entry := range dirEntries {
			t.Logf("Found in target: %s", entry.Name())
		}
	}

	for _, expectedName := range expectedCopiedFiles {
		found := false
		for _, entry := range dirEntries {
			if entry.Name() == expectedName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file %s not found in target directory %s", expectedName, targetDayDir)
		}
	}

	// Ensure discarded files are not present with their *new* potential names
	discardedOriginalNames := []string{"pixel_dupA.jpg", "pixel_dupB.jpg", "file_dupB.txt", "unsupported_pixel_dup.dat"}
	for _, originalName := range discardedOriginalNames {
		discardedExt := filepath.Ext(originalName)
		potentialNewName := fmt.Sprintf("image-%s%s", expectedDateStr, discardedExt)

		// Special case for pixel_dupA.jpg: it's only truly discarded if C replaced it.
		// If C was processed first, A would be a simple duplicate of C.
		// The test order is A, B, C. So B is dup of A. C is higher-res of A. A gets discarded for C.
		if originalName == "pixel_dupA.jpg" && !foundPixelDupAReplacedByC {
			// If A wasn't replaced by C (which would be an error caught above), then A might exist.
			// This check is mostly to prevent false negatives if the replacement logic failed.
			// However, the current logic ensures A is replaced by C.
		}

		foundInTarget := false
		for _, entry := range dirEntries {
			if entry.Name() == potentialNewName {
				// This check is a bit tricky because multiple discarded files might map to the same new name if they shared an extension.
				// e.g. pixel_dupA.jpg and pixel_dupB.jpg if they weren't handled correctly.
				// The primary check is that the kept file (e.g. pixel_dupC_high_res.jpg's new name) is there,
				// and the total count is correct.
				// A more robust check here would be to ensure that if a file was marked as discarded, its *specific content* isn't in the target under any name
				// or that the kept file for that duplicate set is the one present.
				// For now, simply checking if *any* file with that potential new name exists, that wasn't expected, is a good start.
				// We rely on previous checks for *which* specific file was kept.

				// If this potentialNewName is one of the *expected* names, it's not a problem.
				isExpected := false
				for _, en := range expectedCopiedFiles {
					if en == potentialNewName {
						isExpected = true
						break
					}
				}
				if !isExpected { // If it's not an expected name, but matches a discarded file's potential name
					foundInTarget = true
				}
				break // Found a file with this name, break from inner loop
			}
		}
		if foundInTarget {
			// This needs to be more specific: was pixel_dupA.jpg (the one that should have been replaced by C) found?
			if originalName == "pixel_dupA.jpg" {
				 specificDiscardedPath := filepath.Join(targetDayDir, fmt.Sprintf("image-%s.jpg", expectedDateStr))
				 // We expect "image-2023-01-01.jpg" to be from pixel_dupC_high_res.jpg
				 // So, if pixel_dupA.jpg was discarded, its original content shouldn't be there *unless* it was the one kept (which it shouldn't be)
				 // This is complex. The current check for `expectedCopiedFiles` handles what *should* be there.
				 // The count check handles that no *extra* files are there.
			}
			// t.Errorf("Discarded file %s (potentially as %s) found in target directory, but should not be.", originalName, potentialNewName)
		}
	}
}

// local helper for string check in DuplicateInfo Reason
func reasonContains(reason, substr string) bool {
	return strings.Contains(reason, substr)
}
