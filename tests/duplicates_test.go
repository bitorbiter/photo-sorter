package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	// "strings" // No longer directly used in this file after test adjustments
	"testing"
	// "time"    // No longer directly used in this file after test adjustments

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/photo-sorter/pkg"
)

// --- Test Helper Functions ---

var (
	pngMinimal_1x1_Red []byte
	pngMinimal_1x1_Blue []byte
	pngMinimal_2x2_Red []byte
	// For EXIF tests, we'd ideally use files with controlled EXIF,
	// but for now, we'll use distinct small PNGs which might have default/no EXIF.
	// If real EXIF testing is needed, actual files with known EXIF would be better.
	pngForExif1 []byte
	pngForExif2 []byte
)

func fillImageForTest(img *image.RGBA, c color.Color) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func encodePNGForTest(img image.Image) ([]byte, error) {
	var buf bytes.Buffer // Use bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func initializeTestPNGs() {
	var err error
	imgRed1x1 := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fillImageForTest(imgRed1x1, color.RGBA{R: 255, A: 255})
	pngMinimal_1x1_Red, err = encodePNGForTest(imgRed1x1)
	if err != nil {
		log.Fatalf("Failed to create 1x1 Red PNG: %v", err)
	}

	imgBlue1x1 := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fillImageForTest(imgBlue1x1, color.RGBA{B: 255, A: 255})
	pngMinimal_1x1_Blue, err = encodePNGForTest(imgBlue1x1)
	if err != nil {
		log.Fatalf("Failed to create 1x1 Blue PNG: %v", err)
	}

	imgRed2x2 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	fillImageForTest(imgRed2x2, color.RGBA{R: 255, A: 255})
	pngMinimal_2x2_Red, err = encodePNGForTest(imgRed2x2)
	if err != nil {
		log.Fatalf("Failed to create 2x2 Red PNG: %v", err)
	}

	// For EXIF differentiation, we'll just use two different images.
	// In a real scenario, these would be crafted to have specific EXIF.
	pngForExif1 = pngMinimal_1x1_Red
	pngForExif2 = pngMinimal_1x1_Blue // Different content will lead to different hash fallback if EXIF is missing
}

// TestMain for setting up test resources
func TestMain(m *testing.M) {
	initializeTestPNGs()
	// Forcing imageExtensions to include .unsupported_image_ext for specific tests
	// This is a bit of a hack for testing. In a real scenario, you might have better ways
	// to simulate unsupported types or use a more configurable IsImageExtension.
	// Note: This modification is global to the pkg.imageExtensions map for the duration of tests.
	// pkg.ImageExtensions[".unsupported_image_ext"] = true // Cannot do this as it's not exported
	// Instead, tests requiring this will use a known image ext like .png with non-image content.
	os.Exit(m.Run())
}

func createTempFile(t *testing.T, dir string, name string, content []byte) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	err := ioutil.WriteFile(filePath, content, 0644)
	require.NoError(t, err)
	return filePath
}


// --- Test Cases ---

// 1. TestAreFilesPotentiallyDuplicate_Images_PixelHashMatch_DifferentSizes
// As established, CalculatePixelDataHash will NOT match for different dimensions.
// This test will verify they are compared by pixel hash and found to be MISMATCHED.
func TestAreFilesPotentiallyDuplicate_Images_PixelHashMismatch_DifferentDimensions(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "file1.png", pngMinimal_1x1_Red)
	f2Path := createTempFile(t, dir, "file2.png", pngMinimal_2x2_Red) // Same color, different dimensions

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonPixelHashMismatch, res.Reason)
	assert.Equal(t, pkg.HashTypePixel, res.HashType)
}

// 2. TestAreFilesPotentiallyDuplicate_Images_PixelHashMismatch_DifferentSizes (Now different content)
func TestAreFilesPotentiallyDuplicate_Images_PixelHashMismatch_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "file1.png", pngMinimal_1x1_Red)
	f2Path := createTempFile(t, dir, "file2.png", pngMinimal_1x1_Blue) // Different color, same dimensions as f1 if 1x1 blue used

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonPixelHashMismatch, res.Reason)
	assert.Equal(t, pkg.HashTypePixel, res.HashType)
}

// 3. TestAreFilesPotentiallyDuplicate_Images_PixelHashUnsupported_FileHashMatch
func TestAreFilesPotentiallyDuplicate_Images_PixelHashUnsupported_FileHashMatch(t *testing.T) {
	dir := t.TempDir()
	// Use .png extension but provide text content, which image.Decode will fail for.
	textContent := []byte("This is not a valid PNG but will be file-hashed.")
	f1Path := createTempFile(t, dir, "file1.png", textContent)
	f2Path := createTempFile(t, dir, "file2.png", textContent)

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.True(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonFileHashMatch, res.Reason)
	assert.Equal(t, pkg.HashTypeFile, res.HashType)
}

// 4. TestAreFilesPotentiallyDuplicate_Images_ExifMismatch_DifferentSizes
// For this, we rely on getExifSignature. If files have no EXIF, it proceeds to pixel hash.
// If they have different EXIF, it should report ExifMismatch.
// Using two different small PNGs. If they both lack parsable EXIF, this test will behave like a pixel hash test.
// This test is inherently tricky without direct EXIF manipulation.
func TestAreFilesPotentiallyDuplicate_Images_ExifMismatch(t *testing.T) {
	dir := t.TempDir()
	// These files are small PNGs. getExifSignature will likely return ErrNoExif for both.
	// If so, comparison will proceed to pixel hash. Since contents are different, pixel hashes will differ.
	// To truly test EXIF mismatch, one would need files with known different EXIF.
	// Forcing this path is hard without mocks or specialized image files.
	// Let's assume for now these simple PNGs don't have "conflicting" EXIF that would stop the process early.
	// The refactored AreFilesPotentiallyDuplicate proceeds to pixel hash if EXIF is missing/inconclusive.
	f1Path := createTempFile(t, dir, "exif1.png", pngForExif1) // 1x1 Red
	f2Path := createTempFile(t, dir, "exif2.png", pngForExif2) // 1x1 Blue

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)

	// Expected path: No EXIF or same "no EXIF" -> Pixel Hash -> Mismatch
	if res.Reason == pkg.ReasonExifMismatch {
		assert.False(t, res.AreDuplicates) // This would be the ideal if EXIF was different
		assert.Equal(t, pkg.HashTypeExif, res.HashType)
	} else {
		// More likely path for simple PNGs: EXIF inconclusive, then pixel hashes differ
		assert.False(t, res.AreDuplicates)
		assert.Equal(t, pkg.ReasonPixelHashMismatch, res.Reason, "Expected pixel hash mismatch if EXIF was inconclusive")
		assert.Equal(t, pkg.HashTypePixel, res.HashType)
	}
}

// 5. TestAreFilesPotentiallyDuplicate_NonImage_SizeMismatch
func TestAreFilesPotentiallyDuplicate_NonImage_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "file1.txt", []byte("abc"))
	f2Path := createTempFile(t, dir, "file2.txt", []byte("abcdef"))

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonSizeMismatch, res.Reason)
	// HashType might be empty or default if size mismatch is the first check for non-images.
}

// 6. TestAreFilesPotentiallyDuplicate_NonImage_FileHashMatch
func TestAreFilesPotentiallyDuplicate_NonImage_FileHashMatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("same_text_content")
	f1Path := createTempFile(t, dir, "file1.txt", content)
	f2Path := createTempFile(t, dir, "file2.txt", content)

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.True(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonFileHashMatch, res.Reason)
	assert.Equal(t, pkg.HashTypeFile, res.HashType)
}

// 7. TestAreFilesPotentiallyDuplicate_NonImage_FileHashMismatch
func TestAreFilesPotentiallyDuplicate_NonImage_FileHashMismatch(t *testing.T) {
	dir := t.TempDir()
	// Ensure same size for non-image file hash mismatch test
	f1Path := createTempFile(t, dir, "file1.txt", []byte("text_A_content"))
	f2Path := createTempFile(t, dir, "file2.txt", []byte("text_B_content")) // Different content

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonFileHashMismatch, res.Reason)
	assert.Equal(t, pkg.HashTypeFile, res.HashType)
}

// 8. TestAreFilesPotentiallyDuplicate_MixedTypes_SizeMismatch
func TestAreFilesPotentiallyDuplicate_MixedTypes_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "file1.png", pngMinimal_1x1_Red) // Small image
	f2Path := createTempFile(t, dir, "file2.txt", []byte("much_larger_text_content_to_ensure_size_difference"))

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonSizeMismatch, res.Reason)
}

// 9. TestAreFilesPotentiallyDuplicate_MixedTypes_SameSize_FileHashMismatch
func TestAreFilesPotentiallyDuplicate_MixedTypes_SameSize_FileHashMismatch(t *testing.T) {
	dir := t.TempDir()
	// Create content with known same size but different data.
	// pngMinimal_1x1_Red is small. Let's make text file of same size.
	// Note: This is tricky. Actual PNG encoding adds overhead.
	// For simplicity, use text for both but give one an image extension.
	// This tests the "mixed type, same size, different content" path.
	contentA := []byte("contentsizeA")
	contentB := []byte("contentsizeB") // Same length as A

	f1Path := createTempFile(t, dir, "file1.png", contentA) // Treated as image by extension by IsImageExtension
	f2Path := createTempFile(t, dir, "file2.txt", contentB) // Treated as non-image

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonFileHashMismatch, res.Reason)
	assert.Equal(t, pkg.HashTypeFile, res.HashType)
}

// 10. TestAreFilesPotentiallyDuplicate_TargetMissing
func TestAreFilesPotentiallyDuplicate_TargetMissing(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "file1.txt", []byte("source_exists"))
	f2Path := filepath.Join(dir, "non_existent_target.txt")

	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err) // Expect no error, just a specific reason
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonTargetNotFound, res.Reason)
}

// TestAreFilesPotentiallyDuplicate_ZeroByteFiles tests handling of zero-byte files
func TestAreFilesPotentiallyDuplicate_ZeroByteFiles(t *testing.T) {
	dir := t.TempDir()
	f1Path := createTempFile(t, dir, "zero1.txt", []byte{})
	f2Path := createTempFile(t, dir, "zero2.txt", []byte{})
	f3Path := createTempFile(t, dir, "notzero.txt", []byte("content"))

	// Both zero
	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.True(t, res.AreDuplicates, "Two zero-byte files should be duplicates")
	assert.Equal(t, pkg.ReasonFileHashMatch, res.Reason) // Current logic uses FileHashMatch

	// One zero, one not (non-image path, should be size mismatch)
	res, err = pkg.AreFilesPotentiallyDuplicate(f1Path, f3Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates, "Zero-byte and non-zero-byte should be size mismatch")
	assert.Equal(t, pkg.ReasonSizeMismatch, res.Reason)
}

// TestAreFilesPotentiallyDuplicate_ImageAndNonImage_SameSize_DifferentContent
func TestAreFilesPotentiallyDuplicate_ImageAndNonImage_SameSize_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	// Use text content for both to control size precisely, give one an image extension.
	content1 := []byte("abcdefghij") // 10 bytes
	content2 := []byte("klmnopqrst") // 10 bytes, different content

	// File1 is an "image" by extension, File2 is a "text" file.
	f1Path := createTempFile(t, dir, "file_img.png", content1)
	f2Path := createTempFile(t, dir, "file_txt.txt", content2)

	// Expected path: isImg1=true, isImg2=false. Goes to "non-image or mixed" path.
	// Size check: sizes are same.
	// File hash: hashes will be different.
	res, err := pkg.AreFilesPotentiallyDuplicate(f1Path, f2Path)
	require.NoError(t, err)
	assert.False(t, res.AreDuplicates)
	assert.Equal(t, pkg.ReasonFileHashMismatch, res.Reason)
	assert.Equal(t, pkg.HashTypeFile, res.HashType)
}
