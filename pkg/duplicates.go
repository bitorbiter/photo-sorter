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
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

const (
	ReasonSizeMismatch        = "size_mismatch"
	ReasonExifMismatch      = "exif_mismatch"
	ReasonPixelHashMatch    = "pixel_hash_match"
	ReasonPixelHashMismatch = "pixel_hash_mismatch"
	ReasonFileHashMatch     = "file_hash_match"
	ReasonFileHashMismatch  = "file_hash_mismatch"
	ReasonError             = "error"
	ReasonNotCompared       = "not_compared" // e.g. if one file has EXIF, other doesn't, so EXIF isn't strictly a mismatch but a point of divergence
	HashTypePixel           = "pixel_sha256"
	HashTypeFile            = "file_sha256"
	HashTypeExif            = "exif_signature" // Not a cryptographic hash, but a signature
)

type ComparisonResult struct {
	AreDuplicates bool
	Reason        string
	Hash1         string
	Hash2         string
	HashType      string // Type of hash that led to the conclusion (or was last attempted)
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

	// Optionally register specific mknote parsers if needed for certain camera models
	exif.RegisterParsers(mknote.All...)

	x, err := exif.Decode(file)
	if err != nil {
		if err == io.EOF || err.Error() == "exif: failed to find exif intro marker" {
			return "", ErrNoExif // No EXIF data found or not a valid EXIF format
		}
		return "", fmt.Errorf("failed to decode EXIF for %s: %w", filePath, err)
	}

	tags := []exif.FieldName{
		"DateTimeOriginal", "Make", "Model", "ImageWidth", "ImageHeight",
	}
	var signatureParts []string
	var tagValue string

	for _, tagName := range tags {
		tag, err := x.Get(tagName)
		if err != nil {
			// Tag not found, append a placeholder or skip
			signatureParts = append(signatureParts, "NA")
			continue
		}
		tagValue, err = tag.StringVal()
		if err != nil {
			// Error getting string value, append placeholder
			signatureParts = append(signatureParts, "ERR")
			continue
		}
		signatureParts = append(signatureParts, strings.TrimSpace(tagValue))
	}

	// If all parts are "NA", it's likely not useful EXIF data or not an image.
	allNA := true
	for _, part := range signatureParts {
		if part != "NA" {
			allNA = false
			break
		}
	}
	if allNA {
		return "", ErrNoExif
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
// It supports JPEG, PNG, and GIF formats via the standard library.
func GetImageResolution(filePath string) (width int, height int, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open image file %s for resolution: %w", filePath, err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		// This error can occur if the file is not a recognized image format
		// or if the image data is corrupted.
		return 0, 0, fmt.Errorf("failed to decode image config for %s: %w", filePath, err)
	}

	return config.Width, config.Height, nil
}

// CalculatePixelDataHash calculates the SHA-256 hash of an image's raw pixel data.
// It supports JPEG, PNG, and GIF formats.
func CalculatePixelDataHash(filePath string) (string, error) {
	file, err := os.Open(filePath) // TODO: Consider if this file needs to be reopened or if it can be passed around
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for pixel hashing: %w", filePath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		// This can happen for unsupported formats or corrupted image data.
		return "", fmt.Errorf("%w: %v", ErrUnsupportedForPixelHashing, err)
	}

	hasher := sha256.New()
	bounds := img.Bounds()
	pixelBytes := make([]byte, 4) // For R, G, B, A components

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA() // uint32 values (0-0xFFFF)

			// Convert to uint8 (0-0xFF) for consistent hashing
			pixelBytes[0] = byte(r >> 8)
			pixelBytes[1] = byte(g >> 8)
			pixelBytes[2] = byte(b >> 8)
			pixelBytes[3] = byte(a >> 8)

			if _, err := hasher.Write(pixelBytes); err != nil {
				return "", fmt.Errorf("failed to write pixel data to hasher for %s: %w", filePath, err)
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

	// 1. File Size Check
	size1, err := getFileSize(filePath1)
	if err != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error getting size for %s: %w", filePath1, err)
	}
	size2, err := getFileSize(filePath2)
	if err != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error getting size for %s: %w", filePath2, err)
	}

	if size1 != size2 {
		result.Reason = ReasonSizeMismatch
		return result, nil
	}
	result.Reason = ReasonSizeMismatch // Default if sizes are same but files are zero bytes and considered duplicate

	// If files are zero bytes, they are considered duplicates at this stage.
	if size1 == 0 {
		result.AreDuplicates = true
		// ReasonSizeMismatch is okay here, implies they are "duplicates" because their size is identical (and zero)
		// Or, introduce ReasonZeroSizeMatch if more clarity is needed. For now, using existing constants.
		result.Reason = ReasonFileHashMatch // More accurate for zero byte files as they are "content" identical
		result.HashType = HashTypeFile      // Conceptually, content is empty string for both
		result.Hash1 = "zero_bytes"
		result.Hash2 = "zero_bytes"
		return result, nil
	}

	// 2. EXIF Signature Check
	exifSig1, errExif1 := getExifSignature(filePath1)
	exifSig2, errExif2 := getExifSignature(filePath2)

	haveExif1 := errExif1 == nil
	haveExif2 := errExif2 == nil
	result.HashType = HashTypeExif // Tentatively set, may change to pixel/file hash later

	if haveExif1 && haveExif2 {
		result.Hash1 = exifSig1
		result.Hash2 = exifSig2
		if exifSig1 != exifSig2 {
			result.Reason = ReasonExifMismatch
			return result, nil
		}
		// Signatures are the same, proceed to more detailed hashing.
		// Reason will be updated by pixel/file hash. If not, it means they passed EXIF and failed before pixel/file.
		result.Reason = ReasonNotCompared // Reset reason, EXIF matched, so comparison continues
	} else if (errExif1 != nil && errExif1 != ErrNoExif) || (errExif2 != nil && errExif2 != ErrNoExif) {
		result.Reason = ReasonError
		errToReturn := errExif1
		if errExif1 == nil || errExif1 == ErrNoExif {
			errToReturn = errExif2
		}
		return result, fmt.Errorf("error getting EXIF: %w", errToReturn)
	}
	// If one or both files have no EXIF (ErrNoExif), this check is inconclusive.
	// Reason remains ReasonNotCompared or changes if EXIF matched for both. HashType will change.

	// 3. Hashing (Pixel or Full)
	// Attempt Pixel Hash for filePath1
	pixelHash1, errPx1 := CalculatePixelDataHash(filePath1)
	isUnsupported1 := errPx1 != nil && strings.Contains(errPx1.Error(), ErrUnsupportedForPixelHashing.Error())

	if errPx1 == nil { // Successfully pixel-hashed filePath1
		pixelHash2, errPx2 := CalculatePixelDataHash(filePath2)
		isUnsupported2 := errPx2 != nil && strings.Contains(errPx2.Error(), ErrUnsupportedForPixelHashing.Error())

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
			return result, nil
		} else if isUnsupported2 {
			// filePath1 pixel hashed, but filePath2 is unsupported. Fallback to full hash for both.
			// Current HashType might be EXIF or Pixel (if f1 was pixel hashed). Set to File for fallback.
			result.HashType = HashTypeFile // Indicate fallback path
			result.Reason = ReasonNotCompared // Reset reason before full hash
		} else if errPx2 != nil {
			// filePath1 pixel hashed, but filePath2 had another error.
			result.Reason = ReasonError
			return result, fmt.Errorf("error pixel hashing %s after %s succeeded: %w", filePath2, filePath1, errPx2)
		}
	} else if !isUnsupported1 && errPx1 != nil {
		// filePath1 had an error during pixel hashing (not 'unsupported')
		result.Reason = ReasonError
		return result, fmt.Errorf("error pixel hashing %s: %w", filePath1, errPx1)
	}
	// Fallthrough to Full File Hash if:
	// - Pixel hashing filePath1 was unsupported (isUnsupported1 == true)
	// - Pixel hashing filePath1 succeeded, but filePath2 was unsupported (isUnsupported2 == true, handled above)

	// 4. Full File Content Hashing (Fallback/General)
	// Reason would be ReasonNotCompared if EXIF matched or one/both had no EXIF, or if pixel hash was unsupported.
	result.HashType = HashTypeFile // Explicitly set for this stage
	fullHash1, errFf1 := CalculateFileHash(filePath1)
	if errFf1 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error full file hashing for %s: %w", filePath1, errFf1)
	}
	fullHash2, errFf2 := CalculateFileHash(filePath2)
	if errFf2 != nil {
		result.Reason = ReasonError
		return result, fmt.Errorf("error full file hashing for %s: %w", filePath2, errFf2)
	}

	result.Hash1 = fullHash1
	result.Hash2 = fullHash2
	if fullHash1 == fullHash2 {
		result.AreDuplicates = true
		result.Reason = ReasonFileHashMatch
	} else {
		result.Reason = ReasonFileHashMismatch
	}
	return result, nil
}
