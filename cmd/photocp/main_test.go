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

// TestRunApplicationLogic_SourceToEmptyTarget_DirectCopy tests basic copying to an empty target. (Formerly TestRunApplicationLogic_TargetEmpty_SourceNewFile)
func TestRunApplicationLogic_SourceToEmptyTarget_DirectCopy(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)

	photoTime := time.Date(2023, 11, 15, 10, 0, 0, 0, time.UTC)
	sourceFiles := []fileSpec{
		{Path: "photoD.png", Content: pngMinimal_2x2_A, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)

	processed, copied, filesToCopy, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 1, processed, "Should have processed 1 file")
	assert.Equal(t, 1, copied, "Should have copied 1 file")
	assert.Equal(t, 1, filesToCopy, "Files to copy should be 1") // In new logic, filesToCopy == copied
	assert.Len(t, duplicates, 0, "Should be no duplicates")
	assert.Equal(t, 0, unsupported, "No unsupported pixel hashes expected for valid PNG")

	expectedTargetPath := filepath.Join(targetDir, "2023", "11", "2023-11-15-100000.png")
	_, statErr := os.Stat(expectedTargetPath)
	assert.NoError(t, statErr, "Expected target file %s to exist", expectedTargetPath)
}

// TestRunApplicationLogic_SourceHasTwoIdenticalFiles_TargetEmpty tests that if two source files map to the same target path,
// the first is copied, and the second is marked as a duplicate of the first's copy in the target.
func TestRunApplicationLogic_SourceHasTwoIdenticalFiles_TargetEmpty(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)

	photoTime := time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC)
	// Assuming photoE1.png is processed first by runApplicationLogic due to directory scan order (though not guaranteed).
	// The critical part is that one is copied, the other is compared against the copy.
	sourceFile1Path := "photoE1.png" // Will be copied
	sourceFile2Path := "photoE2.png" // Will be compared against copy of photoE1.png

	sourceFilesSpec := []fileSpec{
		{Path: sourceFile1Path, Content: pngMinimal_2x2_A, ModTime: photoTime},
		{Path: sourceFile2Path, Content: pngMinimal_2x2_A, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFilesSpec)

	// Need the full path of the source file that will be discarded for assertion.
	// Which one is discarded depends on processing order, but it will be one of them.
	// The KeptFile will be the target path.
	fullSourceFile1Path := filepath.Join(sourceDir, sourceFile1Path)
	fullSourceFile2Path := filepath.Join(sourceDir, sourceFile2Path)

	processed, copied, filesToCopy, duplicates, _, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 2, processed, "Should have processed 2 files")
	assert.Equal(t, 1, copied, "Should have copied only 1 file (the first one to arrive at target name)")
	assert.Equal(t, 1, filesToCopy, "Files to copy should be 1")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry")

	expectedTargetFilePath := filepath.Join(targetDir, "2023", "12", "2023-12-01-120000.png")

	// The first processed source file (e.g. photoE1.png) is copied to expectedTargetFilePath.
	// The second source file (e.g. photoE2.png) is compared against this expectedTargetFilePath.
	assert.Equal(t, expectedTargetFilePath, duplicates[0].KeptFile, "KeptFile in report should be the path of the copied first file in target")

	// The DiscardedFile should be the source path of the *other* source file.
	// If photoE1.png was effectively copied (its content is at expectedTargetFilePath), then photoE2.png was discarded.
	// If photoE2.png was effectively copied, then photoE1.png was discarded.
	// The current implementation of runApplicationLogic processes files based on `imageFiles` order.
	// Let's assume photoE1.png is processed first.
	// This assertion needs to be robust to the actual order.
	// The key is that one of them is discarded.
	discardedIsE1 := duplicates[0].DiscardedFile == fullSourceFile1Path
	discardedIsE2 := duplicates[0].DiscardedFile == fullSourceFile2Path
	assert.True(t, discardedIsE1 || discardedIsE2, "Discarded file must be one of the source files.")

	// More specific check: if KeptFile's content matches E1 (which is same as E2),
	// then the DiscardedFile path must be the one NOT chosen for copying.
	// This test assumes photoE1.png is processed first. If so, photoE2.png is discarded.
	// For this test to be deterministic, we'd need to control processing order or check content.
	// Given they are identical, the first one processed gets copied. The other is its duplicate.
	// So, if photoE1.png is processed first, it's copied, and photoE2.png is the duplicate.
	// For now, we assume photoE2.png path is what's in DiscardedFile if photoE1.png was copied.
	// This depends on the file walk order. A safer check is just that *a* source file was discarded.
	// Let's assume for test stability that the file named "photoE2.png" is the one discarded if "photoE1.png" was copied.
	// This requires runApplicationLogic to effectively process photoE1.png then photoE2.png.
	// A truly robust way is to check that one is copied, and the other is listed as discarded against the copy.
	// For now:
	if strings.HasSuffix(duplicates[0].KeptFile, sourceFile1Path) { // Should not happen, KeptFile is target
		// This case is wrong, KeptFile is always a target path after processing.
		t.Errorf("KeptFile should be a target path, not source path.")
	}
	// Assert that the discarded file is one of the two original source files.
	// And that the KeptFile is the single copied file.
	assert.Contains(t, []string{fullSourceFile1Path, fullSourceFile2Path}, duplicates[0].DiscardedFile, "Discarded file should be one of the original source paths")

	assert.Contains(t, duplicates[0].Reason, pkg.ReasonPixelHashMatch, "Reason should indicate a pixel hash match")

	_, statErr := os.Stat(expectedTargetFilePath)
	assert.NoError(t, statErr, "Expected target file %s (copy of the first source file) to exist", expectedTargetFilePath)

	targetMonthDir := filepath.Join(targetDir, "2023", "12")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only one file (the copy of the first source file) should be in the target month directory")
}

// TestRunApplicationLogic_TargetExists_SourceDifferentContent_TargetPreserved_LowerResSource
// Tests when source/target map to same path but are NOT pixel duplicates (due to resolution differences affecting pixel hash).
// Expected: Existing target is preserved, source is discarded.
func TestRunApplicationLogic_TargetExists_SourceDifferentContent_TargetPreserved_LowerResSource(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file: Higher resolution (pngMinimal_4x4_A)
	targetFileContent := pngMinimal_4x4_A
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: targetFileContent, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Lower resolution (pngMinimal_2x2_A), same visual pixel content but different dimensions, so different pixel hash.
	sourceFileContent := pngMinimal_2x2_A
	sourceFiles := []fileSpec{
		{Path: "photoA.png", Content: sourceFileContent, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	fullSourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path)

	processed, copied, filesToCopy, duplicates, _, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 1, processed, "Should have processed 1 file")
	assert.Equal(t, 0, copied, "Should have copied 0 files as source is discarded")
	assert.Equal(t, 0, filesToCopy, "Files to copy should be 0")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry (source discarded, target kept)")

	assert.Equal(t, expectedTargetFilePath, duplicates[0].KeptFile)
	assert.Equal(t, fullSourceFilePath, duplicates[0].DiscardedFile)
	// In main.go, if AreFilesPotentiallyDuplicate is false for a name collision, this reason is used.
	assert.Equal(t, "Content different, but name collision; existing target preserved", duplicates[0].Reason)

	_, statErr := os.Stat(expectedTargetFilePath) // Original target
	assert.NoError(t, statErr, "Target file should still exist and be unchanged")

	// Check content of target file to ensure it wasn't overwritten
	targetContentBytes, readErr := os.ReadFile(expectedTargetFilePath)
	require.NoError(t, readErr)
	assert.Equal(t, targetFileContent, targetContentBytes, "Target file content should not have changed")

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only the original target file should be in the target month directory")
}

// TestRunApplicationLogic_TargetExists_SourceDifferentContent_TargetPreserved_HigherResSource
// Tests when source/target map to same path but are NOT pixel duplicates (due to resolution differences affecting pixel hash).
// Expected: Existing target is preserved, source is discarded.
func TestRunApplicationLogic_TargetExists_SourceDifferentContent_TargetPreserved_HigherResSource(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file: Lower resolution (pngMinimal_2x2_A)
	targetFileContent := pngMinimal_2x2_A
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: targetFileContent, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Higher resolution (pngMinimal_4x4_A), same visual pixel content but different dimensions, so different pixel hash.
	sourceFileContent := pngMinimal_4x4_A
	sourceFiles := []fileSpec{
		{Path: "photoB.png", Content: sourceFileContent, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	fullSourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path)

	processed, copied, filesToCopy, duplicates, _, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 1, processed, "Should have processed 1 file")
	assert.Equal(t, 0, copied, "Should have copied 0 files as source is discarded")
	assert.Equal(t, 0, filesToCopy, "Files to copy should be 0")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry (source discarded, target kept)")

	assert.Equal(t, expectedTargetFilePath, duplicates[0].KeptFile)
	assert.Equal(t, fullSourceFilePath, duplicates[0].DiscardedFile)
	assert.Equal(t, "Content different, but name collision; existing target preserved", duplicates[0].Reason)

	_, statErr := os.Stat(expectedTargetFilePath) // Original target
	assert.NoError(t, statErr, "Original target file should still exist and be unchanged")

	targetContentBytes, readErr := os.ReadFile(expectedTargetFilePath)
	require.NoError(t, readErr)
	assert.Equal(t, targetFileContent, targetContentBytes, "Target file content should not have changed")

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only the original target file should be in the target month directory")
}

// TestRunApplicationLogic_TargetExists_SourceDifferentFileSameName tests that if files map to the same target path but are different content (not duplicates),
// the existing target is preserved and the source is discarded. (No versioning with current main.go logic for this case).
func TestRunApplicationLogic_TargetExists_SourceDifferentFileSameName(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2023, 10, 27, 15, 30, 0, 0, time.UTC)

	// Target file (Content A)
	targetFileContentA := pngMinimal_2x2_A
	targetFiles := []fileSpec{
		{Path: filepath.Join("2023", "10", "2023-10-27-153000.png"), Content: targetFileContentA, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Different content (Content B), different pixel hash
	sourceFileContentB := pngMinimal_2x2_B
	sourceFiles := []fileSpec{
		{Path: "photoC.png", Content: sourceFileContentB, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	fullSourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path)

	processed, copied, filesToCopy, duplicates, _, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 1, processed, "Should have processed 1 file")
	assert.Equal(t, 0, copied, "Should have copied 0 files as source is discarded")
	assert.Equal(t, 0, filesToCopy, "Files to copy should be 0")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry (source discarded, target kept)")

	assert.Equal(t, expectedTargetFilePath, duplicates[0].KeptFile)
	assert.Equal(t, fullSourceFilePath, duplicates[0].DiscardedFile)
	assert.Equal(t, "Content different, but name collision; existing target preserved", duplicates[0].Reason)

	_, statErr := os.Stat(expectedTargetFilePath)
	assert.NoError(t, statErr, "Original target file should still exist and be unchanged")

	targetContentBytes, readErr := os.ReadFile(expectedTargetFilePath)
	require.NoError(t, readErr)
	assert.Equal(t, targetFileContentA, targetContentBytes, "Target file content should not have changed")

	targetMonthDir := filepath.Join(targetDir, "2023", "10")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only the original target file should be in the target month directory")
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
	sourceFilesS2Spec := []fileSpec{
		{Path: "S2.png", Content: sourceContentS2, ModTime: photoTime}, // Same date, maps to same exact target path initially
	}
	createTestFiles(t, sourceDir, sourceFilesS2Spec)
	sourceFilePathS2 := filepath.Join(sourceDir, sourceFilesS2Spec[0].Path)

	processed, copied, filesToCopy, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	// S1.png (source) vs T1.png (target at exactTargetPath) -> FileHashMatch, S1 discarded.
	// S2.png (source) vs T1.png (target at exactTargetPath) -> Content different (AreFilesPotentiallyDuplicate=false), S2 discarded.
	assert.Equal(t, 2, processed, "Should process 2 source files")
	assert.Equal(t, 0, copied, "Should have copied 0 files")
	assert.Equal(t, 0, filesToCopy, "Files to copy should be 0")
	require.Len(t, duplicates, 2, "Should be 2 duplicate entries (S1.png vs T1.png, and S2.png vs T1.png)")

	// Both S1.png and S2.png are source files with ".png" extension.
	// Pixel hashing will be attempted and will fail (text content).
	// So, both S1.png and S2.png will use file hash for comparison against the target (or try to).
	// S1 will use file hash. S2 comparison might also note file hash was attempted if it got that far.
	// The `sourceFilesThatUsedFileHash` map in main.go gets populated if `compResult.HashType == pkg.HashTypeFile`.
	// For S1 vs T1, compResult.HashType will be FileHash.
	// For S2 vs T1, compResult.AreDuplicates is false. HashType might still be FileHash if comparison reached that stage.
	assert.Equal(t, 2, unsupported, "Pixel hash unsupported should be 2 (for S1.png and S2.png as they are image types but will fail pixel decoding and attempt file hash)")

	// Check duplicate entry for S1.png
	dupS1Found := false
	dupS2Found := false
	for _, dup := range duplicates {
		if dup.DiscardedFile == sourceFilePathS1 {
			assert.Equal(t, expectedTargetFilePath, dup.KeptFile)
			assert.Equal(t, pkg.ReasonFileHashMatch+" (existing target kept)", dup.Reason, "S1 should be a file hash match with T1")
			dupS1Found = true
		} else if dup.DiscardedFile == sourceFilePathS2 {
			assert.Equal(t, expectedTargetFilePath, dup.KeptFile)
			assert.Equal(t, "Content different, but name collision; existing target preserved", dup.Reason, "S2 should be discarded as different content with name collision")
			dupS2Found = true
		}
	}
	assert.True(t, dupS1Found, "Duplicate entry for S1 not found or incorrect")
	assert.True(t, dupS2Found, "Duplicate entry for S2 not found or incorrect")

	_, statErr := os.Stat(expectedTargetFilePath) // T1
	assert.NoError(t, statErr, "Target file T1 should still exist")

	// No files should have been copied to target (S1 was duplicate, S2 was different but discarded)
	targetMonthDir := filepath.Join(targetDir, "2024", "01")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only the original target file T1 should be in the target month directory")
}

// TestRunApplicationLogic_SourceConflictsWithIdenticalTarget_MarkedDuplicate tests that a source file
// identical to an existing target file (same content, same name via date) is marked as a duplicate.
func TestRunApplicationLogic_SourceConflictsWithIdenticalTarget_MarkedDuplicate(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2024, 2, 20, 11, 0, 0, 0, time.UTC)

	// Target file
	targetFileContent := pngMinimal_2x2_A
	targetFiles := []fileSpec{
		{Path: filepath.Join("2024", "02", "2024-02-20-110000.png"), Content: targetFileContent, ModTime: photoTime},
	}
	createTestFiles(t, targetDir, targetFiles)
	expectedTargetFilePath := filepath.Join(targetDir, targetFiles[0].Path)

	// Source file: Identical content and maps to the same target path
	sourceFiles := []fileSpec{
		{Path: "source_identical.png", Content: targetFileContent, ModTime: photoTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)
	fullSourceFilePath := filepath.Join(sourceDir, sourceFiles[0].Path)

	processed, copied, filesToCopy, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 1, processed, "Should have processed 1 file")
	assert.Equal(t, 0, copied, "Should have copied 0 files")
	assert.Equal(t, 0, filesToCopy, "Files to copy should be 0")
	require.Len(t, duplicates, 1, "Should be 1 duplicate entry")
	assert.Equal(t, 0, unsupported, "No unsupported pixel hash for valid identical PNGs")

	dup := duplicates[0]
	assert.Equal(t, expectedTargetFilePath, dup.KeptFile)
	assert.Equal(t, fullSourceFilePath, dup.DiscardedFile)
	// Since they are identical PNGs, it should be a pixel hash match.
	assert.Contains(t, dup.Reason, pkg.ReasonPixelHashMatch)

	_, statErr := os.Stat(expectedTargetFilePath)
	assert.NoError(t, statErr, "Target file should still exist")

	targetContentBytes, readErr := os.ReadFile(expectedTargetFilePath)
	require.NoError(t, readErr)
	assert.Equal(t, targetFileContent, targetContentBytes, "Target file content should not have changed")

	targetMonthDir := filepath.Join(targetDir, "2024", "02")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only the original target file should be in the target month directory")
}

// TestRunApplicationLogic_SequentialSourceToSameTarget tests processing multiple source files that map to the same target path.
// S1 (original) -> copied to targetFile.png
// S2 (different content from S1) -> discarded as different from targetFile.png
// S3 (same content as S1) -> discarded as duplicate of targetFile.png
func TestRunApplicationLogic_SequentialSourceToSameTarget(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)
	photoTime := time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC) // All source files use same time

	s1Path := "s1_original.png"
	s2Path := "s2_different.png"
	s3Path := "s3_same_as_s1.png"

	// Ensure a specific processing order for deterministic testing if possible,
	// by naming files alphabetically or relying on Go's file walk order (usually alphabetical).
	// For this test, file names s1, s2, s3 should lead to that processing order.
	sourceFiles := []fileSpec{
		{Path: s1Path, Content: pngMinimal_2x2_A, ModTime: photoTime}, // S1 - Content A
		{Path: s2Path, Content: pngMinimal_2x2_B, ModTime: photoTime}, // S2 - Content B (different from A)
		{Path: s3Path, Content: pngMinimal_2x2_A, ModTime: photoTime}, // S3 - Content A (same as S1)
	}
	createTestFiles(t, sourceDir, sourceFiles)

	// fullS1Path := filepath.Join(sourceDir, s1Path) // Not directly used in assertions on duplicates
	fullS2Path := filepath.Join(sourceDir, s2Path)
	fullS3Path := filepath.Join(sourceDir, s3Path)

	// Expected target path for S1 (and where S2, S3 will also initially map)
	expectedTargetForS1 := filepath.Join(targetDir, "2024", "03", "2024-03-10-090000.png")

	processed, copied, filesToCopy, duplicates, unsupported, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err)

	assert.Equal(t, 3, processed, "Should process 3 source files")
	assert.Equal(t, 1, copied, "Should copy only S1")
	assert.Equal(t, 1, filesToCopy, "Files to copy should be 1")
	assert.Equal(t, 0, unsupported, "All are valid PNGs, so pixel hash should be supported for all sources")
	require.Len(t, duplicates, 2, "Should be 2 duplicate entries (for S2 and S3)")

	// Check that S1 was copied
	_, statErr := os.Stat(expectedTargetForS1)
	assert.NoError(t, statErr, "S1 should have been copied to %s", expectedTargetForS1)
	copiedContent, _ := os.ReadFile(expectedTargetForS1)
	assert.Equal(t, pngMinimal_2x2_A, copiedContent, "Content of copied file should be from S1")

	// Check duplicate entries
	s2Discarded := false
	s3Discarded := false
	for _, dup := range duplicates {
		assert.Equal(t, expectedTargetForS1, dup.KeptFile, "KeptFile for all duplicates should be the path of S1's copy")
		if dup.DiscardedFile == fullS2Path {
			s2Discarded = true
			assert.Equal(t, "Content different, but name collision; existing target preserved", dup.Reason, "Reason for S2 discard")
		} else if dup.DiscardedFile == fullS3Path {
			s3Discarded = true
			assert.Contains(t, dup.Reason, pkg.ReasonPixelHashMatch, "Reason for S3 discard should be pixel hash match")
		}
	}
	assert.True(t, s2Discarded, "S2 should be in discarded list")
	assert.True(t, s3Discarded, "S3 should be in discarded list")

	targetMonthDir := filepath.Join(targetDir, "2024", "03")
	dirEntries, _ := os.ReadDir(targetMonthDir)
	assert.Len(t, dirEntries, 1, "Only S1's copy should be in the target directory")

	// Log all files to help debug if needed (optional, can be removed)
	// filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
	// 	if err != nil { return err }
	// 	t.Logf("Found in target: %s (dir: %t)", path, info.IsDir()); return nil
	// })
	// filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
	// 	if err != nil { return err }
	// 	t.Logf("Found in source: %s (dir: %t)", path, info.IsDir()); return nil
	// })
}

// TODO: Add more tests:
// - Pixel hash unsupported for an actual image type (e.g. if pkg.IsImageExtension says true, but image.Decode fails or it's a format not in stdlib image decoders)
//   This is partly covered by TestRunApplicationLogic_PixelHashUnsupported_FallbackToFileHash, but could be more specific with an image that's not text.
// - Error conditions (e.g., cannot read source, cannot write target - though runApplicationLogic might return error before full processing).

// Placeholder for a more complex test
func TestRunApplicationLogic_ComplexScenario(t *testing.T) {
	t.Skip("Complex scenario test not yet implemented.")
}

func TestRunApplicationLogic_HEICSupport(t *testing.T) {
	sourceDir, targetDir := setupTestDirs(t)

	// Create a placeholder HEIC file in the source directory.
	// EXIF extraction will fail, so it should use file modification time.
	// heif-go is registered, so GetImageResolution might succeed or fail gracefully.
	// AreFilesPotentiallyDuplicate will likely use file hash if pixel hash is unsupported for empty HEIC.
	heicModTime := time.Date(2024, 7, 15, 14, 30, 0, 0, time.UTC)
	heicContent := []byte("simulated heic content") // Not a real HEIC, just for testing file ops

	sourceFiles := []fileSpec{
		// Use one of the placeholder files created in step 1 of the overall task.
		// For the test, we create it here with specific mod time and content.
		{Path: "sampleA.heic", Content: heicContent, ModTime: heicModTime},
	}
	createTestFiles(t, sourceDir, sourceFiles)

	processed, copied, filesToCopy, duplicates, pixelHashUnsupported, err := runApplicationLogic(sourceDir, targetDir, false) // Added verbose=false
	require.NoError(t, err, "runApplicationLogic should not error for HEIC file")

	assert.Equal(t, 1, processed, "Should have processed 1 HEIC file")
	assert.Equal(t, 1, copied, "Should have copied 1 HEIC file")
	assert.Equal(t, 1, filesToCopy, "Files to copy should be 1") // filesToCopy == copied
	assert.Len(t, duplicates, 0, "Should be no duplicates for a single HEIC file to empty target")

	// Behavior of pixelHashUnsupported depends on heif-go's ability to decode the placeholder.
	// If heif-go decodes it (even if to a default image), unsupported might be 0.
	// If heif-go fails and it falls back to file hash, unsupported might be 1.
	// We'll be flexible here or assert based on expected heif-go behavior with minimal files.
	// Given it's "simulated heic content", heif-go will likely fail to decode it.
	// And pkg.IsImageExtension("sampleA.heic") is true.
	// So, pixel hash will be attempted, fail, and it will be marked as unsupported
	// IF AND ONLY IF it goes through the duplicate check path.
	// Since it's a direct copy to an empty target, it will NOT go through duplicate check.
	// Thus, pixelHashUnsupported will be 0 based on current main.go logic.
	assert.Equal(t, 0, pixelHashUnsupported, "Pixel hash should be 0 for a directly copied file, as it doesn't go through duplicate check logic where this is counted.")

	// Verify the file was copied to the correct location based on ModTime
	expectedTargetFilename := heicModTime.Format(testDateFormat) + ".heic"
	expectedTargetPath := filepath.Join(targetDir, heicModTime.Format("2006"), heicModTime.Format("01"), expectedTargetFilename)

	_, statErr := os.Stat(expectedTargetPath)
	assert.NoError(t, statErr, "Expected target HEIC file %s to exist", expectedTargetPath)

	// Verify content if needed (though it's just a placeholder)
	copiedContent, readErr := os.ReadFile(expectedTargetPath)
	require.NoError(t, readErr)
	assert.Equal(t, heicContent, copiedContent, "Content of copied HEIC file should match source")

	// Verify report (minimal check, more detailed checks could be added)
	reportFilePath := filepath.Join(targetDir, "report.txt")
	_, reportStatErr := os.Stat(reportFilePath)
	assert.NoError(t, reportStatErr, "Report file should exist")

	reportContent, readReportErr := os.ReadFile(reportFilePath)
	require.NoError(t, readReportErr)
	reportStr := string(reportContent)

	// Adjust expected report strings to match the actual format from pkg/reporter.go
	assert.Contains(t, reportStr, "Total files scanned: 1", "Report: Files Processed count incorrect")
	assert.Contains(t, reportStr, "Files successfully copied: 1", "Report: Files Copied count incorrect")
	assert.Contains(t, reportStr, "Duplicate files found and discarded/skipped: 0", "Report: Duplicates Found count incorrect")
	// Based on the corrected understanding, pixelHashUnsupported should be 0 for this test case.
	// Also ensure the report text matches the change from "Files where..." to "Image files where..."
	assert.Contains(t, reportStr, "Image files where pixel hashing was not supported (fallback to file hash): 0", "Report: Pixel Hash Unsupported count incorrect")
}
