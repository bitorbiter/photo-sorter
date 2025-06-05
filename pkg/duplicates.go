package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"os"
	// "path/filepath" // No longer directly needed here
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

const (
	ReasonSizeMismatch            = "size_mismatch"
	ReasonExifMismatch          = "exif_mismatch"
	ReasonPixelHashMatch        = "pixel_hash_match"
	ReasonPixelHashMismatch     = "pixel_hash_mismatch"
	ReasonFileHashMatch         = "file_hash_match"
	ReasonFileHashMismatch      = "file_hash_mismatch"
	ReasonError                 = "error"
	ReasonNotCompared           = "not_compared" // e.g. if one file has EXIF, other doesn't, so EXIF isn't strictly a mismatch but a point of divergence
	ReasonTargetNotFound        = "target_not_found"
	ReasonPixelHashNotAttempted = "pixel_hash_not_attempted"
	HashTypePixel               = "pixel_sha256"
	HashTypeFile                = "file_sha256"
	HashTypeExif                = "exif_signature" // Not a cryptographic hash, but a signature
)

type ComparisonResult struct {
	AreDuplicates bool
	Reason        string
	Hash1         string // Hash/Signature of filePath1
	Hash2         string // Hash/Signature of filePath2
	HashType      string // Type of hash/signature that led to the conclusion (or was last attempted for filePath1)
	FilePath1     string
	FilePath2     string
}

// ErrUnsupportedForPixelHashing is returned when a file format is not supported for pixel data hashing.
var ErrUnsupportedForPixelHashing = fmt.Errorf("file format not supported for pixel data hashing")

// ErrNoExif is returned when EXIF data is not found in a file.
var ErrNoExif = fmt.Errorf("EXIF data not found")

// getFileSize returns the size of a file in bytes.
func getFileSize(filePath string) (int64, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}
	return fi.Size(), nil
}

// getExifSignature generates a signature string from key EXIF tags.
// Returns ErrNoExif if EXIF data is not present or critical tags are missing.
func getExifSignature(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for EXIF parsing %s: %w", filePath, err)
	}
	defer file.Close()

	exif.RegisterParsers(mknote.All...)

	x, err := exif.Decode(file)
	if err != nil {
		if err == io.EOF || strings.Contains(err.Error(), "exif: failed to find exif intro marker") || strings.Contains(err.Error(), "tiff: short tag read") {
			return "", ErrNoExif
		}
		return "", fmt.Errorf("failed to decode EXIF for %s: %w", filePath, err)
	}

	tags := []exif.FieldName{
		"DateTimeOriginal", "Make", "Model", "ImageWidth", "ImageHeight",
		// Consider adding LensModel, FNumber, ExposureTime, ISOSpeedRatings if more granularity is needed
	}
	var signatureParts []string
	// var tagValue string // Removed - will use directly from StringVal()

	for _, tagName := range tags {
		tag, errGet := x.Get(tagName)
		if errGet != nil {
			signatureParts = append(signatureParts, "NA")
			continue
		}
		valStr, errStr := tag.StringVal() // Use new variable valStr
		if errStr != nil {
			signatureParts = append(signatureParts, "ERR")
			continue
		}
		signatureParts = append(signatureParts, strings.TrimSpace(valStr)) // Use valStr
	}

	allNA := true
	for _, part := range signatureParts {
		if part != "NA" && part != "ERR" { // If any tag was successfully read
			allNA = false
			break
		}
	}
	if allNA {
		return "", ErrNoExif // No useful EXIF tags found
	}

	return strings.Join(signatureParts, "_"), nil
}

// CalculateFileHash calculates the SHA-256 hash of a file's content.
func CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for hashing: %w", filePath, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to copy file content to hasher for %s: %w", filePath, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetImageResolution decodes the image configuration to get its width and height.
func GetImageResolution(filePath string) (width int, height int, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image file %s for resolution: %w", filePath, err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode image config for %s: %w", filePath, err)
	}

	return config.Width, config.Height, nil
}

// CalculatePixelDataHash calculates the SHA-256 hash of an image's raw pixel data.
func CalculatePixelDataHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for pixel hashing: %w", filePath, err)
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		// Check if the error is due to an unknown format, which we class as "unsupported"
		if err == image.ErrFormat {
			return "", fmt.Errorf("%w: format %s", ErrUnsupportedForPixelHashing, format)
		}
		// Other errors (e.g., corrupted data for a known format) also mean we can't get pixel data.
		return "", fmt.Errorf("%w: decoding image data for %s: %v", ErrUnsupportedForPixelHashing, filePath, err)
	}
	// Check if the decoded format is one we explicitly support for pixel hashing (e.g. jpeg, png, gif)
	// This is an extra check, as image.Decode might support more formats than we want for pixel hashing.
	// For now, assume if image.Decode succeeds, we try to hash.
	// Consider adding: if format != "jpeg" && format != "png" && format != "gif" { return "", ErrUnsupported... }

	hasher := sha256.New()
	bounds := img.Bounds()
	pixelBytes := make([]byte, 4)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			pixelBytes[0] = byte(r >> 8)
			pixelBytes[1] = byte(g >> 8)
			pixelBytes[2] = byte(b >> 8)
			pixelBytes[3] = byte(a >> 8)
			if _, errWrite := hasher.Write(pixelBytes); errWrite != nil {
				return "", fmt.Errorf("failed to write pixel data to hasher for %s: %w", filePath, errWrite)
			}
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// AreFilesPotentiallyDuplicate implements the multi-step duplicate detection logic.
func AreFilesPotentiallyDuplicate(filePath1, filePath2 string) (ComparisonResult, error) {
	result := ComparisonResult{
		AreDuplicates: false,
		Reason:        ReasonNotCompared,
		FilePath1:     filePath1,
		FilePath2:     filePath2,
	}

	// 1. Target File Existence Check
	if _, err := os.Stat(filePath2); os.IsNotExist(err) {
		result.Reason = ReasonTargetNotFound
		return result, nil
	}

	// Handle zero-byte files early. If both are zero bytes, they are duplicates.
	// If one is zero and the other isn't, they'll be caught by size check if not images,
	// or proceed to hashing if images (where pixel hash of empty image is not sensible, file hash will be different).
	size1, errSize1 := getFileSize(filePath1)
	if errSize1 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error getting size for %s: %w", filePath1, errSize1)
	}
	size2, errSize2 := getFileSize(filePath2)
	if errSize2 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error getting size for %s: %w", filePath2, errSize2)
	}

	if size1 == 0 && size2 == 0 {
		result.AreDuplicates = true
		result.Reason = ReasonFileHashMatch // Consistent with previous logic for zero-byte files
		result.HashType = HashTypeFile
		result.Hash1 = "zero_bytes"
		result.Hash2 = "zero_bytes"
		return result, nil
	}


	// 2. Determine if files are images
	isImg1 := IsImageExtension(filePath1)
	isImg2 := IsImageExtension(filePath2)

	pixelHashingAttemptedOrUnsupported := false

	if isImg1 && isImg2 {
		// 3.a EXIF Signature Check (for images)
		result.HashType = HashTypeExif // Tentative type
		exifSig1, errExif1 := getExifSignature(filePath1)
		exifSig2, errExif2 := getExifSignature(filePath2)

		if errExif1 == nil && errExif2 == nil { // Both have EXIF
			result.Hash1 = exifSig1
			result.Hash2 = exifSig2
			if exifSig1 != exifSig2 {
				result.Reason = ReasonExifMismatch
				// AreDuplicates remains false
				return result, nil
			}
			// EXIF signatures match, proceed to pixel hash. ReasonNotCompared will be updated by pixel/file hash.
			result.Reason = ReasonNotCompared
		} else if (errExif1 != nil && errExif1 != ErrNoExif) || (errExif2 != nil && errExif2 != ErrNoExif) {
			// An actual error occurred reading EXIF for at least one file (not just missing EXIF)
			// Log this error? For now, treat as inconclusive for EXIF and move to pixel hash.
			// Or, could return an error here if strict EXIF handling is required.
			// For robustness, perhaps log and continue to pixel hash.
			fmt.Printf("Warning: EXIF read error (f1: %v, f2: %v) for %s, %s. Proceeding to pixel hash.\n", errExif1, errExif2, filePath1, filePath2)
			result.Reason = ReasonNotCompared // EXIF check was inconclusive due to error
		}
		// If one or both have ErrNoExif, EXIF check is inconclusive. Proceed.

		// 3.b Pixel Data Hash Comparison (for images)
		pixelHashingAttemptedOrUnsupported = true // Mark that we are going down this path.
		pixelHash1, errPx1 := CalculatePixelDataHash(filePath1)
		if errPx1 == nil { // Successfully pixel-hashed filePath1
			pixelHash2, errPx2 := CalculatePixelDataHash(filePath2)
			if errPx2 == nil { // Successfully pixel-hashed filePath2 as well
				result.HashType = HashTypePixel
				result.Hash1 = pixelHash1
				result.Hash2 = pixelHash2
				if pixelHash1 == pixelHash2 {
					result.AreDuplicates = true
					result.Reason = ReasonPixelHashMatch
				} else {
					result.Reason = ReasonPixelHashMismatch
				}
				return result, nil // Pixel hash conclusive (match or mismatch)
			} else if strings.Contains(errPx2.Error(), ErrUnsupportedForPixelHashing.Error()) {
				// filePath1 pixel hashed, but filePath2 is unsupported. Fallback to full file hash for both.
				fmt.Printf("Info: Pixel hash for %s succeeded, but unsupported for %s. Falling back to file hash.\n", filePath1, filePath2)
				result.HashType = HashTypeFile // Fallback path
				result.Reason = ReasonPixelHashNotAttempted // Reset reason before full hash
			} else { // filePath1 pixel hashed, but filePath2 had another error.
				result.Reason = ReasonError
				return result, fmt.Errorf("error pixel hashing %s after %s succeeded: %w", filePath2, filePath1, errPx2)
			}
		} else if strings.Contains(errPx1.Error(), ErrUnsupportedForPixelHashing.Error()) {
			// Pixel hashing filePath1 was unsupported. Fallback to full file hash for both.
			fmt.Printf("Info: Pixel hash unsupported for %s. Falling back to file hash.\n", filePath1)
			result.HashType = HashTypeFile // Fallback path
			result.Reason = ReasonPixelHashNotAttempted // Reset reason
		} else { // filePath1 had a critical error during pixel hashing (not 'unsupported')
			result.Reason = ReasonError
			return result, fmt.Errorf("error pixel hashing %s: %w", filePath1, errPx1)
		}
		// If we reach here, it's because pixel hashing for at least one image was unsupported,
		// leading to a fallback to full file content hashing (3.c).
	}

	// 4. Fallback/Default Comparison (Full File Hash)
	// This section is reached if:
	// - Files are not both images (isImg1 && isImg2 is false).
	// - Files are both images, but pixel hashing was unsupported/failed for at least one, and we fell through.

	// 4.a File Size Check (only if not both images AND pixel hashing wasn't attempted/unsupported for images)
	// If pixelHashingAttemptedOrUnsupported is true, it means we tried the image path, which doesn't use size as a primary filter.
	// If it's false, it means we are on the non-image path, where size IS a primary filter.
	if !pixelHashingAttemptedOrUnsupported {
		// Sizes were already fetched for zero-byte check.
		if size1 != size2 {
			result.Reason = ReasonSizeMismatch
			result.HashType = "" // Size is not a hash
			// AreDuplicates is already false
			return result, nil
		}
		// If sizes are same, and one/both are non-images, proceed to file hash.
		// HashType will be File.
	}

	// 3.c / 4.b Full File Content Hashing
	// Reason would be ReasonNotCompared (if EXIF was inconclusive) or ReasonPixelHashNotAttempted (if pixel hash path led here)
	// or if it's the non-image path and sizes matched.
	result.HashType = HashTypeFile // Explicitly set for this stage

	fullHash1, errFf1 := CalculateFileHash(filePath1)
	if errFf1 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error full file hashing for %s: %w", filePath1, errFf1)
	}
	result.Hash1 = fullHash1

	fullHash2, errFf2 := CalculateFileHash(filePath2)
	if errFf2 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error full file hashing for %s: %w", filePath2, errFf2)
	}
	result.Hash2 = fullHash2

	if fullHash1 == fullHash2 {
		result.AreDuplicates = true
		result.Reason = ReasonFileHashMatch
	} else {
		// AreDuplicates is already false
		result.Reason = ReasonFileHashMismatch
	}
	return result, nil
}
