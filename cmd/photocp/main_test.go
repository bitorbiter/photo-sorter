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
	"strings" // Added strings import
	"testing"
	"time"

	"github.com/user/photo-sorter/pkg"
)

// createTestFile helper to create generic files for testing
func createTestFile(t *testing.T, dir string, name string, content []byte, modTime time.Time) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write test file %s: %v", filePath, err)
	}
	if err := os.Chtimes(filePath, modTime, modTime); err != nil {
		t.Fatalf("Failed to change mod time for %s: %v", filePath, err)
	}
	return filePath
}

// createTestImage helper to create PNG, JPEG, or GIF images
func createTestImage(t *testing.T, dir string, name string, width, height int, format string, c color.Color, modTime time.Time) string {
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

// TestGenerateDestinationPathLogic tests the construction of the destination filename.
// Covers: REQ-CF-FR-03
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
// Covers:
// - REQ-CF-DS-01, REQ-CF-DS-02, REQ-CF-DS-04 (Date sorting, YYYY/MM/DD structure, fallback to modTime)
// - REQ-CF-FR-01, REQ-CF-FR-02, REQ-CF-FR-03 (File renaming)
// - REQ-CF-ADD-01 to REQ-CF-ADD-08 (Advanced duplicate detection - two-tiered, pixel/file hashing)
// - REQ-CF-DR-01, REQ-CF-DR-02, REQ-CF-DR-03 (Duplicate resolution - preference, fallback)
// - REQ-RP-RC-07, REQ-RP-RC-08, REQ-RP-RC-09, REQ-RP-RC-10, REQ-RP-RC-11 (Reporting aspects via duplicateReportEntries check)
func TestMainProcessingLogic_DuplicateDetectionAndCopying(t *testing.T) {
	sourceDir := t.TempDir()
	targetBaseDir := t.TempDir()

	redColor := color.RGBA{R: 255, A: 255}
	blueColor := color.RGBA{B: 255, A: 255}

	modTime1 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC) // For A, B, txt, unsup, raw
	modTime2 := time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC) // For C (high-res)
	modTime3 := time.Date(2023, 1, 3, 12, 0, 0, 0, time.UTC) // For unique_image

	imgPixelDupA := createTestImage(t, sourceDir, "pixel_dupA.png", 10, 10, "png", redColor, modTime1)
	imgPixelDupB := createTestImage(t, sourceDir, "pixel_dupB.png", 10, 10, "png", redColor, modTime1)
	imgPixelDupCRes := createTestImage(t, sourceDir, "pixel_dupC_high_res.png", 20, 20, "png", redColor, modTime2)

	txtFileDupA := createTestFile(t, sourceDir, "file_dupA.txt", []byte("identical content"), modTime1)
	txtFileDupB := createTestFile(t, sourceDir, "file_dupB.txt", []byte("identical content"), modTime1)

	imgUnique := createTestImage(t, sourceDir, "unique_image.png", 10, 10, "png", blueColor, modTime3)

	unsupportedPixelA := createTestFile(t, sourceDir, "unsupported_pixel.dat", []byte("some data for file hash A"), modTime1)
	unsupportedPixelB := createTestFile(t, sourceDir, "unsupported_pixel_dup.dat", []byte("some data for file hash A"), modTime1)

	rawFile := createTestFile(t, sourceDir, "image_for_rename.NEF", []byte("unique raw content"), modTime1)

	imageFiles := []string{
		imgPixelDupA, imgPixelDupB, imgPixelDupCRes,
		txtFileDupA, txtFileDupB,
		imgUnique,
		unsupportedPixelA, unsupportedPixelB,
		rawFile,
	}

	pixelHashedCandidates := make(map[string]FileInfo)
	fileHashedCandidates := make(map[string]FileInfo)
	var duplicateReportEntries []pkg.DuplicateInfo
	copiedFilesCounter := 0
	filesToCopyCount := 0

	// --- Phase 1: Populate Candidate Maps & DuplicateReportEntries ---
	for _, currentFilePath := range imageFiles {
		var processThisFile bool

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentFilePath)
		if errRes != nil {
			currentWidth, currentHeight = 0, 0
		}

		currentFileInfo := FileInfo{ Path: currentFilePath, Width: currentWidth, Height: currentHeight }
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
				if logPixelHashAttemptNeeded { /* log.Printf(...) */ }
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
				} else { /* log.Printf(...) */ currentFileInfo.HashType = "none"; processThisFile = false }
			} else { /* log.Printf(...) */ currentFileInfo.HashType = "none"; processThisFile = false }
		}
		_ = processThisFile
	}

	// --- Phase 2: Process Final Candidates for Copying ---
	finalCandidatesToCopy := []FileInfo{}
	for _, fi := range pixelHashedCandidates { finalCandidatesToCopy = append(finalCandidatesToCopy, fi) }
	for _, fi := range fileHashedCandidates { finalCandidatesToCopy = append(finalCandidatesToCopy, fi) }
	filesToCopyCount = len(finalCandidatesToCopy)

	for _, currentFileCopyInfo := range finalCandidatesToCopy {
		currentFilePath := currentFileCopyInfo.Path
		exifDate, exifErr := mockGetPhotoCreationDate(currentFilePath)
		var photoDate time.Time
		if exifErr == nil { photoDate = exifDate } else {
			fileInfoStat, statErr := os.Stat(currentFilePath)
			if statErr != nil { t.Errorf("Error statting file %s: %v", currentFilePath, statErr); continue }
			photoDate = fileInfoStat.ModTime()
		}
		targetDayDir, err := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if err != nil { t.Errorf("Error creating target dir for %s: %v", currentFilePath, err); continue }
		originalExtension := filepath.Ext(currentFilePath)
		dateStr := photoDate.Format("2006-01-02")
		newFileName := fmt.Sprintf("image-%s%s", dateStr, originalExtension)
		destPath := filepath.Join(targetDayDir, newFileName)
		if err := pkg.CopyFile(currentFilePath, destPath); err != nil { t.Errorf("Error copying file %s to %s: %v", currentFilePath, destPath, err) } else { copiedFilesCounter++ }
	}

	// --- Assertions ---
	expectedDuplicateEntries := 3
	if len(duplicateReportEntries) != expectedDuplicateEntries {
		t.Errorf("Expected %d duplicate entries, got %d. Entries:", expectedDuplicateEntries, len(duplicateReportEntries))
		for i, entry := range duplicateReportEntries {
			t.Logf("DupEntry %d: Kept: %s, Discarded: %s, Reason: %s", i, filepath.Base(entry.KeptFile), filepath.Base(entry.DiscardedFile), entry.Reason)
		}
	}

	foundPixelDupBDiscardedForA := false
	foundFileDupBDiscardedForA := false
	foundUnsupportedDupBDiscardedForA := false
	pixelADiscardedByC := false

	for _, entry := range duplicateReportEntries {
		dPathBase := filepath.Base(entry.DiscardedFile)
		kPathBase := filepath.Base(entry.KeptFile)
		if kPathBase == "pixel_dupA.png" && dPathBase == "pixel_dupB.png" && reasonContains(entry.Reason, "Pixel hash match") { foundPixelDupBDiscardedForA = true }
		if kPathBase == "file_dupA.txt" && dPathBase == "file_dupB.txt" && reasonContains(entry.Reason, "File hash match") { foundFileDupBDiscardedForA = true }
		if kPathBase == "unsupported_pixel.dat" && dPathBase == "unsupported_pixel_dup.dat" && reasonContains(entry.Reason, "File hash match") { foundUnsupportedDupBDiscardedForA = true }
		if kPathBase == "pixel_dupC_high_res.png" && dPathBase == "pixel_dupA.png" { pixelADiscardedByC = true }
	}

	if !foundPixelDupBDiscardedForA { t.Errorf("Expected pixel_dupB.png to be discarded as pixel duplicate of pixel_dupA.png") }
	if pixelADiscardedByC { t.Errorf("pixel_dupA.png was unexpectedly discarded/replaced by pixel_dupC_high_res.png. They should have different pixel hashes due to different dimensions.") }
	if !foundFileDupBDiscardedForA { t.Errorf("Expected file_dupB.txt to be discarded as file duplicate of file_dupA.txt") }
	if !foundUnsupportedDupBDiscardedForA { t.Errorf("Expected unsupported_pixel_dup.dat to be discarded as file duplicate of unsupported_pixel.dat") }

	expectedDateStr1 := "2023-01-01"
	expectedDateStr2 := "2023-01-02"
	expectedDateStr3 := "2023-01-03"

	expectedCopiedFileMap := map[string]bool{
		filepath.Join(targetBaseDir, "2023/01/01", fmt.Sprintf("image-%s.png", expectedDateStr1)): true,
		filepath.Join(targetBaseDir, "2023/01/02", fmt.Sprintf("image-%s.png", expectedDateStr2)): true,
		filepath.Join(targetBaseDir, "2023/01/01", fmt.Sprintf("image-%s.txt", expectedDateStr1)): true,
		filepath.Join(targetBaseDir, "2023/01/03", fmt.Sprintf("image-%s.png", expectedDateStr3)): true,
		filepath.Join(targetBaseDir, "2023/01/01", fmt.Sprintf("image-%s.dat", expectedDateStr1)): true,
		filepath.Join(targetBaseDir, "2023/01/01", fmt.Sprintf("image-%s.NEF", expectedDateStr1)): true,
	}
	expectedCopiedCount := 6

	if copiedFilesCounter != expectedCopiedCount { t.Errorf("Expected %d files to be copied, got %d", expectedCopiedCount, copiedFilesCounter) }
	if filesToCopyCount != expectedCopiedCount { t.Errorf("Expected filesToCopyCount to be %d, got %d", expectedCopiedCount, filesToCopyCount) }

	var foundFilesInTarget int
	targetDirsToCheck := []string{
		filepath.Join(targetBaseDir, "2023/01/01"),
		filepath.Join(targetBaseDir, "2023/01/02"),
		filepath.Join(targetBaseDir, "2023/01/03"),
	}

	for _, dir := range targetDirsToCheck {
		shouldExist := false
		for expectedPath := range expectedCopiedFileMap { if filepath.Dir(expectedPath) == dir { shouldExist = true; break } }
		if !shouldExist {
			if _, err := os.Stat(dir); !os.IsNotExist(err) { t.Errorf("Target directory %s exists but no files were expected in it.", dir) }
			continue
		}
		dirEntries, err := os.ReadDir(dir)
		if err != nil { t.Fatalf("Could not read target directory %s: %v", dir, err) }
		for _, entry := range dirEntries {
			fullPath := filepath.Join(dir, entry.Name())
			if _, expected := expectedCopiedFileMap[fullPath]; expected { foundFilesInTarget++ } else { t.Errorf("Unexpected file %s found in target directory %s", entry.Name(), dir) }
		}
	}
	if foundFilesInTarget != expectedCopiedCount { t.Errorf("Expected %d total files across target date directories, found %d", expectedCopiedCount, foundFilesInTarget) }

	discardedFileNamesActual := []string{"pixel_dupB.png", "file_dupB.txt", "unsupported_pixel_dup.dat"}
	for _, originalName := range discardedFileNamesActual {
		isRecordedAsDiscarded := false
		for _, entry := range duplicateReportEntries { if filepath.Base(entry.DiscardedFile) == originalName { isRecordedAsDiscarded = true; break } }
		if !isRecordedAsDiscarded { t.Errorf("Expected original file %s to be recorded as discarded in duplicateReportEntries, but it wasn't.", originalName) }
	}

	if pixelADiscardedByC { t.Errorf("pixel_dupA.png should NOT have been discarded by pixel_dupC_high_res.png as they have different pixel hashes.") }
}

// local helper for string check in DuplicateInfo Reason
func reasonContains(reason, substr string) bool {
	return strings.Contains(reason, substr)
}
