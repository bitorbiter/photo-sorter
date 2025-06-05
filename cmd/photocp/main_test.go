package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/photo-sorter/pkg"
)

// --- Test Helper Functions ---

const (
	testDateFormat = "2006-01-02-150405"
)

// Pre-generated minimal PNG image data
var (
	pngMinimal_2x2_A []byte // Content A, 2x2
	pngMinimal_4x4_A []byte // Content A, 4x4 (same visual content as 2x2_A but higher res)
	pngMinimal_2x2_B []byte // Content B, 2x2 (different visual content from A)
	pngMinimal_4x4_C []byte // Content C, 4x4 (different visual content from A and B)
)

// initializePNGs creates the actual PNG byte data.
func initializePNGs() {
	var err error
	// Create PNG A (2x2) - e.g., all red
	imgA_2x2 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fillImage(imgA_2x2, color.RGBA{R: 255, A: 255}) // Red
	pngMinimal_2x2_A, err = encodePNG(imgA_2x2)
	if err != nil {
		log.Fatalf("Failed to create 2x2 PNG A: %v", err)
	}

	// Create PNG A (4x4) - e.g., all red
	imgA_4x4 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fillImage(imgA_4x4, color.RGBA{R: 255, A: 255}) // Red
	pngMinimal_4x4_A, err = encodePNG(imgA_4x4)
	if err != nil {
		log.Fatalf("Failed to create 4x4 PNG A: %v", err)
	}

	// Create PNG B (2x2) - e.g., all blue
	imgB_2x2 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fillImage(imgB_2x2, color.RGBA{B: 255, A: 255}) // Blue
	pngMinimal_2x2_B, err = encodePNG(imgB_2x2)
	if err != nil {
		log.Fatalf("Failed to create 2x2 PNG B: %v", err)
	}

	// Create PNG C (4x4) - e.g., all green
	imgC_4x4 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fillImage(imgC_4x4, color.RGBA{G: 255, A: 255}) // Green
	pngMinimal_4x4_C, err = encodePNG(imgC_4x4)
	if err != nil {
		log.Fatalf("Failed to create 4x4 PNG C: %v", err)
	}
}

func fillImage(img *image.RGBA, c color.Color) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf strings.Builder
	writer := &buf
	err := png.Encode(writer, img)
	if err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// TestMain is used to initialize PNGs once.
func TestMain(m *testing.M) {
	initializePNGs()
	os.Exit(m.Run())
}

type fileSpec struct {
	Path    string    // Relative to base (sourceDir or targetDir)
	Content []byte    // Use pre-generated PNGs or []byte("text content")
	ModTime time.Time // To simulate EXIF date if EXIF data itself is not embedded
}

func createTestFiles(t *testing.T, baseDir string, files []fileSpec) {
	t.Helper()
	for _, f := range files {
		fullPath := filepath.Join(baseDir, f.Path)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err, "Failed to create directory %s", dir)

		err = os.WriteFile(fullPath, f.Content, 0644)
		require.NoError(t, err, "Failed to write file %s", fullPath)

		if !f.ModTime.IsZero() {
			err = os.Chtimes(fullPath, f.ModTime, f.ModTime)
			require.NoError(t, err, "Failed to set mod time for %s", fullPath)
		}
	}
}

func setupTestDirs(t *testing.T) (sourceDir, targetDir string) {
	t.Helper()
	sourceDir = t.TempDir()
	targetDir = t.TempDir()
	return sourceDir, targetDir
}

// --- Test Cases ---

// TestRunApplicationLogic_TargetEmpty_SourceNewFile tests basic copying to an empty target.
func TestRunApplicationLogic_TargetEmpty_SourceNewFile(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)

	photoTime := time.Date(2023, 11, 15, 10, 0, 0, 0, time.UTC)
	sourceFiles := []fileSpec{
		{Path: "photoD.png", Content: pngMinimal_2x2_A, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)

	_, copied, _, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir)
	require.NoError(t, err)

	assert.Equal(t, 1, copied, "Should have copied 1 file")
	assert.Len(t, duplicates, 0, "Should be no duplicates")
	// Since pngMinimal_2x2_A is a valid PNG, pixel hash should be attempted.
	// unsupported should be 0 unless IsImageExtension filters out ".png" or hashing fails unexpectedly.
	// Given current setup, IsImageExtension includes ".png".
	assert.Equal(t, 0, unsupported, "No unsupported pixel hashes expected for valid PNG")


	expectedTargetPath := filepath.Join(targetDir, "2023", "11", "2023-11-15-100000.png")
	_, statErr := os.Stat(expectedTargetPath)
	assert.NoError(t, statErr, "Expected target file %s to exist", expectedTargetPath)
}

// TestRunApplicationLogic_SourceHasTwoIdenticalFiles_TargetEmpty tests that only one of two identical source files is copied.
func TestRunApplicationLogic_SourceHasTwoIdenticalFiles_TargetEmpty(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)

	photoTime := time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC)
	sourceFiles := []fileSpec{
		{Path: "photoE1.png", Content: pngMinimal_2x2_A, ModTime: photoTime}, // Content A
		{Path: "photoE2.png", Content: pngMinimal_2x2_A, ModTime: photoTime}, // Content A (identical)
	}
	createTestFiles(t, sourceDir, sourceFiles)

	_, copied, _, duplicates, _, err := runApplicationLogic(sourceDir, targetDir)
	require.NoError(t, err)

	assert.Equal(t, 1, copied, "Should have copied only 1 file")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry")

	// Determine which file was kept and which was discarded by checking the report.
	// The KeptFile path in the report will be the target path.
	expectedTargetBase := filepath.Join(targetDir, "2023", "12", "2023-12-01-120000.png")

	// Assuming the first encountered file (photoE1.png) is selected and copied.
	// Its path in the report's KeptFile field will be its new target path.
	// The DiscardedFile will be the source path of photoE2.png.
	assert.Equal(t, expectedTargetBase, duplicates[0].KeptFile, "KeptFile in report should be the target path of the first selected file")

	// Check that one of the source files was reported as discarded.
	// duplicates[0].DiscardedFile will be the full source path.
	isE1Discarded := strings.HasSuffix(duplicates[0].DiscardedFile, sourceFiles[0].Path) // e.g., photoE1.png
	isE2Discarded := strings.HasSuffix(duplicates[0].DiscardedFile, sourceFiles[1].Path) // e.g., photoE2.png
	assert.True(t, isE1Discarded || isE2Discarded, "Discarded file from report (%s) should be one of the original source files E1 or E2", duplicates[0].DiscardedFile)

	// Additional check: if E1 was kept (implied by it NOT being discarded), then E2 should be the one discarded.
	// And vice-versa. This makes the test more robust to the order of processing by runApplicationLogic.
	if !isE1Discarded { // If E1 was NOT discarded, it means E1 was kept.
		assert.True(t, isE2Discarded, "If photoE1.png was kept, then photoE2.png should have been reported as discarded.")
	}
	if !isE2Discarded { // If E2 was NOT discarded, it means E2 was kept.
		assert.True(t, isE1Discarded, "If photoE2.png was kept, then photoE1.png should have been reported as discarded.")
	}

	// Check that the kept file exists (its path is in duplicates[0].KeptFile)
	_, statErr := os.Stat(duplicates[0].KeptFile)
	assert.NoError(t, statErr, "Expected target file %s (from report's KeptFile) to exist", duplicates[0].KeptFile)

	// Ensure only one file in the target month dir
	targetMonthDir := filepath.Join(targetDir, "2023", "12")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only one file should be in the target month directory")
}

// TestRunApplicationLogic_TargetExists_SourceLowerResDuplicate tests when target has a higher-res pixel duplicate.
func TestRunApplicationLogic_TargetExists_SourceLowerResDuplicate(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file: Higher resolution
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: pngMinimal_4x4_A, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)


	// Source file: Lower resolution, same pixel content
	sourceFiles := []fileSpec{
		{Path: "photoA.png", Content: pngMinimal_2x2_A, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	// sourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path) // No longer directly needed for assertions

	_, copied, _, duplicates, _, err := runApplicationLogic(sourceDir, targetDir)
	require.NoError(t, err)

	// ACTUAL BEHAVIOR (CalculatePixelDataHash won't match different-dimension images):
	// Source and Target are not pixel duplicates. Source will be copied and versioned.
	assert.Equal(t, 1, copied, "Should have copied 1 file (source, as it's not a pixel duplicate due to dimension diff)")
	assert.Len(t, duplicates, 0, "Should be 0 duplicate entries as dimension difference means no pixel hash match")

	_, statErr := os.Stat(expectedTargetFilePath) // Original target
	assert.NoError(t, statErr, "Target file should still exist")

	// Source file copied with a versioned name
	expectedNewPath := filepath.Join(targetDir, "2023", "10", "2023-10-27-153000-1.png")
	_, statErr = os.Stat(expectedNewPath)
	assert.NoError(t, statErr, "Source file should have been copied and versioned: %s", expectedNewPath)

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 2, "Two files should be in the target month directory")
}

// TestRunApplicationLogic_TargetExists_SourceHigherResDuplicate tests when source has a higher-res pixel duplicate.
func TestRunApplicationLogic_TargetExists_SourceHigherResDuplicate(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file: Lower resolution
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: pngMinimal_2x2_A, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	originalTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Higher resolution, same pixel content
	sourceFiles := []fileSpec{
		{Path: "photoB.png", Content: pngMinimal_4x4_A, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	// sourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path) // Original source path for reference

	_, copied, _, duplicates, _, err := runApplicationLogic(sourceDir, targetDir)
	require.NoError(t, err)

	// ACTUAL BEHAVIOR (CalculatePixelDataHash won't match different-dimension images):
	// Source and Target are not pixel duplicates. Source will be copied and versioned.
	assert.Equal(t, 1, copied, "Should have copied 1 file (source, as it's not a pixel duplicate due to dimension diff)")
	assert.Len(t, duplicates, 0, "Should be 0 duplicate entries as dimension difference means no pixel hash match for supersede")

	_, statErr := os.Stat(originalTargetFilePath) // Original target
	assert.NoError(t, statErr, "Original target file should still exist")

	// Source file copied with a versioned name
	expectedNewPath := filepath.Join(targetDir, "2023", "10", "2023-10-27-153000-1.png")
	_, statErr = os.Stat(expectedNewPath)
	assert.NoError(t, statErr, "Source file should have been copied and versioned: %s", expectedNewPath)

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 2, "Two files should be in the target month directory")
}


// TestRunApplicationLogic_TargetExists_SourceDifferentFileSameName tests versioning when a non-duplicate file has the same target name.
func TestRunApplicationLogic_TargetExists_SourceDifferentFileSameName(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: pngMinimal_2x2_A, ModTime: photoTime}, // Content A
	}
	createTestFiles(t, targetDir, targetFiles)
	originalTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Different content, different pixel hash
	sourceFiles := []fileSpec{
		{Path: "photoC.png", Content: pngMinimal_2x2_B, ModTime: photoTime}, // Content B
	}
	createTestFiles(t, sourceDir, sourceFiles)

	_, copied, _, duplicates, _, err := runApplicationLogic(sourceDir, targetDir)
	require.NoError(t, err)

	assert.Equal(t, 1, copied, "Should have copied 1 file")
	assert.Len(t, duplicates, 0, "Should be no duplicates reported for this interaction")

	// Expected new file path (versioned)
	expectedNewPath := filepath.Join(targetDir, "2023", "10", "2023-10-27-153000-1.png")

	_, statErr := os.Stat(originalTargetFilePath)
	assert.NoError(t, statErr, "Original target file should still exist")
	_, statErr = os.Stat(expectedNewPath)
	assert.NoError(t, statErr, "Newly copied source file should exist with a versioned name: %s", expectedNewPath)

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 2, "Should be two files in the target month directory")
}


// TestRunApplicationLogic_PixelHashUnsupported_FallbackToFileHash tests fallback to file hash for non-image files.
func TestRunApplicationLogic_PixelHashUnsupported_FallbackToFileHash(t *testing.T) {
    sourceDir, targetDir := setupTestDirs(t)
    photoTime := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)

    // Target: A PNG file with text content (will fail pixel hash, fallback to file hash)
    targetContent := []byte("This is target file T1 masquerading as PNG.")
    targetFiles := []fileSpec{
        {Path: filepath.Join("2024", "01", "2024-01-10-100000.png"), Content: targetContent, ModTime: photoTime},
    }
    createTestFiles(t, targetDir, targetFiles)
    expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

    // Source: A PNG file with identical text content (will be file hash duplicate)
    sourceFiles := []fileSpec{
        {Path: "S1.png", Content: targetContent, ModTime: photoTime},
    }
    createTestFiles(t, sourceDir, sourceFiles)
    sourceFilePathS1 := filepath.Join(sourceDir, sourceFiles[0].Path)

    // Source: Another PNG file with different text content
    sourceContentS2 := []byte("This is source file S2, different content, also as PNG.")
    sourceFiles2 := []fileSpec{
        {Path: "S2.png", Content: sourceContentS2, ModTime: photoTime}, // Same date, will try to version
    }
    createTestFiles(t, sourceDir, sourceFiles2)


    _, copied, _, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir)
    require.NoError(t, err)

    assert.Equal(t, 1, copied, "Should have copied 1 file (S2.png)")
    require.Len(t, duplicates, 1, "Should be 1 duplicate entry (S1.png vs T1.png)")
    // Both S1.png and T1.png are images by extension. They will fail pixel hash and use file hash.
    // S1.png should be counted in sourceFilesThatUsedFileHash.
    // S2.png (the one copied) will also be processed this way if it's an image type that fails pixel decode.
    // T1.png (target) doesn't count towards `unsupported` (which is for source files).
    assert.Equal(t, 2, unsupported, "Pixel hash unsupported should be 2 (for S1.png and S2.png as they are image types but will fail pixel decoding)")


    dup := duplicates[0]
    assert.Equal(t, expectedTargetFilePath, dup.KeptFile)
    assert.Equal(t, sourceFilePathS1, dup.DiscardedFile)
    assert.Equal(t, pkg.ReasonFileHashMatch, dup.Reason) // Expecting file hash match

    _, statErr := os.Stat(expectedTargetFilePath) // T1
    assert.NoError(t, statErr, "Target file T1 should still exist")

    expectedS2TargetPath := filepath.Join(targetDir, "2024", "01", "2024-01-10-100000-1.png") // S2 copied and versioned
    _, statErr = os.Stat(expectedS2TargetPath)
    assert.NoError(t, statErr, "Source file S2 should have been copied and versioned: %s", expectedS2TargetPath)
}

// TODO: Add more tests:
// - Source file is file-hash duplicate of target file (using actual images this time).
// - Source file is file-hash duplicate of an already selected-for-copy source file (images).
// - Pixel hash unsupported for an actual image type (e.g. if pkg.IsImageExtension says true, but image.Decode fails or it's a format not in stdlib image decoders)
// - Complex scenario:
//   - Target has T1.
//   - Source has S1 (better res than T1, pixel dup), S2 (same res as T1, pixel dup), S3 (file dup of T1).
//   Expected: S1 copied (supersedes T1 in report), S2 discarded (vs T1 or S1), S3 discarded (vs T1 or S1).
// - Error conditions (e.g., cannot read source, cannot write target - though runApplicationLogic might return error before full processing).

// Placeholder for a more complex test
func TestRunApplicationLogic_ComplexScenario(t *testing.T) {
	t.Skip("Complex scenario test not yet implemented.")
}
