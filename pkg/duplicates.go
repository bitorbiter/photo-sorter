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
	ReasonSizeMismatch            = "size_mismatch"            // Indicates a mismatch in file sizes.
	ReasonExifMismatch          = "exif_mismatch"            // Indicates differing EXIF signatures for two images.
	ReasonPixelHashMatch        = "pixel_hash_match"         // Indicates matching raw pixel data hashes for two images.
	ReasonPixelHashMismatch     = "pixel_hash_mismatch"      // Indicates differing raw pixel data hashes for two images.
	ReasonFileHashMatch         = "file_hash_match"          // Indicates matching full file content hashes.
	ReasonFileHashMismatch      = "file_hash_mismatch"       // Indicates differing full file content hashes.
	ReasonError                 = "error"                    // Indicates an unexpected error during comparison.
	ReasonNotCompared           = "not_compared"             // Initial or intermediate state if comparison is inconclusive at a stage.
	ReasonTargetNotFound        = "target_not_found"         // Indicates the target file for comparison does not exist.
	ReasonPixelHashNotAttempted = "pixel_hash_not_attempted" // Internal reason if pixel hashing was skipped before trying file hash.
	HashTypePixel               = "pixel_sha256"             // Pixel data hash (SHA-256).
	HashTypeFile                = "file_sha256"              // Full file content hash (SHA-256).
	HashTypeExif                = "exif_signature"           // EXIF-based signature string.
)

// ComparisonResult holds the outcome of a file comparison.
type ComparisonResult struct {
	AreDuplicates bool   // True if files are considered duplicates based on the logic.
	Reason        string // Reason code for the comparison outcome.
	Hash1         string // Hash/Signature of filePath1, relevant to the HashType.
	Hash2         string // Hash/Signature of filePath2, relevant to the HashType.
	HashType      string // Type of hash/signature that led to the conclusion or was last attempted.
	FilePath1     string // Path of the first file compared.
	FilePath2     string // Path of the second file compared.
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
	}
	var signatureParts []string

	for _, tagName := range tags {
		tag, errGet := x.Get(tagName)
		if errGet != nil {
			signatureParts = append(signatureParts, "NA")
			continue
		}
		valStr, errStr := tag.StringVal()
		if errStr != nil {
			signatureParts = append(signatureParts, "ERR")
			continue
		}
		signatureParts = append(signatureParts, strings.TrimSpace(valStr))
	}

	allNA := true
	for _, part := range signatureParts {
		if part != "NA" && part != "ERR" {
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
// This identifies images with bit-for-bit identical pixel streams.
func CalculatePixelDataHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s for pixel hashing: %w", filePath, err)
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		if err == image.ErrFormat {
			return "", fmt.Errorf("%w: format %s", ErrUnsupportedForPixelHashing, format)
		}
		return "", fmt.Errorf("%w: decoding image data for %s: %v", ErrUnsupportedForPixelHashing, filePath, err)
	}

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

// AreFilesPotentiallyDuplicate implements the multi-step duplicate detection logic
// as described in detail in docs/technical_details.md.
// It compares filePath1 and filePath2 to determine if they are duplicates.
func AreFilesPotentiallyDuplicate(filePath1, filePath2 string) (ComparisonResult, error) {
	result := ComparisonResult{
		AreDuplicates: false,
		Reason:        ReasonNotCompared,
		FilePath1:     filePath1,
		FilePath2:     filePath2,
	}

	if _, err := os.Stat(filePath2); os.IsNotExist(err) {
		result.Reason = ReasonTargetNotFound
		return result, nil
	}

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
		result.Reason = ReasonFileHashMatch
		result.HashType = HashTypeFile
		result.Hash1 = "zero_bytes"
		result.Hash2 = "zero_bytes"
		return result, nil
	}

	isImg1 := IsImageExtension(filePath1)
	isImg2 := IsImageExtension(filePath2)
	pixelHashingAttemptedOrUnsupported := false

	if isImg1 && isImg2 {
		result.HashType = HashTypeExif
		exifSig1, errExif1 := getExifSignature(filePath1)
		exifSig2, errExif2 := getExifSignature(filePath2)

		if errExif1 == nil && errExif2 == nil {
			result.Hash1 = exifSig1
			result.Hash2 = exifSig2
			if exifSig1 != exifSig2 {
				result.Reason = ReasonExifMismatch
				return result, nil
			}
			result.Reason = ReasonNotCompared
		} else if (errExif1 != nil && errExif1 != ErrNoExif) || (errExif2 != nil && errExif2 != ErrNoExif) {
			fmt.Printf("Warning: EXIF read error (f1: %v, f2: %v) for %s, %s. Proceeding to pixel hash.\n", errExif1, errExif2, filePath1, filePath2)
			result.Reason = ReasonNotCompared
		}

		pixelHashingAttemptedOrUnsupported = true
		pixelHash1, errPx1 := CalculatePixelDataHash(filePath1)
		if errPx1 == nil {
			pixelHash2, errPx2 := CalculatePixelDataHash(filePath2)
			if errPx2 == nil {
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
			} else if strings.Contains(errPx2.Error(), ErrUnsupportedForPixelHashing.Error()) {
				fmt.Printf("Info: Pixel hash for %s succeeded, but unsupported for %s. Falling back to file hash.\n", filePath1, filePath2)
				result.HashType = HashTypeFile
				result.Reason = ReasonPixelHashNotAttempted
			} else {
				result.Reason = ReasonError
				return result, fmt.Errorf("error pixel hashing %s after %s succeeded: %w", filePath2, filePath1, errPx2)
			}
		} else if strings.Contains(errPx1.Error(), ErrUnsupportedForPixelHashing.Error()) {
			fmt.Printf("Info: Pixel hash unsupported for %s. Falling back to file hash.\n", filePath1)
			result.HashType = HashTypeFile
			result.Reason = ReasonPixelHashNotAttempted
		} else {
			result.Reason = ReasonError
			return result, fmt.Errorf("error pixel hashing %s: %w", filePath1, errPx1)
		}
	}

	if !pixelHashingAttemptedOrUnsupported {
		if size1 != size2 {
			result.Reason = ReasonSizeMismatch
			result.HashType = ""
			return result, nil
		}
	}

	result.HashType = HashTypeFile

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
		result.Reason = ReasonFileHashMismatch
	}
	return result, nil
}
