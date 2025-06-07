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
	mknote "github.com/rwcarlsen/goexif/mknote"
)

// compareByExif attempts to compare two files using their EXIF signatures.
// match: true if EXIF signatures are present and identical.
// conclusive: true if this comparison is enough to determine the outcome (e.g., EXIF mismatch).
// err: any error encountered during EXIF processing (not ErrNoExif).
// sig1, sig2: the EXIF signatures if obtained.
func compareByExif(filePath1, filePath2 string) (match bool, conclusive bool, err error, sig1 string, sig2 string) {
	exifSig1, errExif1 := getExifSignature(filePath1)
	exifSig2, errExif2 := getExifSignature(filePath2)

	// Case 1: Both files have EXIF data.
	if errExif1 == nil && errExif2 == nil {
		sig1 = exifSig1
		sig2 = exifSig2
		if exifSig1 == exifSig2 {
			return true, false, nil, sig1, sig2 // Match, but inconclusive (need further checks)
		}
		// EXIF signatures differ.
		return false, true, nil, sig1, sig2 // No match, conclusive
	}

	// Case 2: Errors encountered that are not ErrNoExif.
	if (errExif1 != nil && errExif1 != ErrNoExif) || (errExif2 != nil && errExif2 != ErrNoExif) {
		// Construct a combined error message if both failed with actual errors.
		if (errExif1 != nil && errExif1 != ErrNoExif) && (errExif2 != nil && errExif2 != ErrNoExif) {
			return false, false, fmt.Errorf("EXIF read error for %s (%v) and %s (%v)", filePath1, errExif1, filePath2, errExif2), "", ""
		} else if errExif1 != nil && errExif1 != ErrNoExif {
			return false, false, fmt.Errorf("EXIF read error for %s: %w", filePath1, errExif1), "", ""
		}
		return false, false, fmt.Errorf("EXIF read error for %s: %w", filePath2, errExif2), "", ""
	}

	// Case 3: One or both files have no EXIF data (ErrNoExif).
	// This scenario is considered inconclusive for matching based on EXIF alone.
	// We still return any valid signatures obtained.
	if errExif1 == nil {
		sig1 = exifSig1
	}
	if errExif2 == nil {
		sig2 = exifSig2
	}
	// If one has EXIF and the other doesn't, or if both don't, it's not a match, and not conclusive.
	return false, false, nil, sig1, sig2
}

// compareByPixelHash attempts to compare two image files using their pixel data hashes.
// match: true if pixel hashes were successfully computed for both and they are identical.
// conclusive: true if this comparison is enough to determine the outcome (e.g., pixel hashes match or mismatch).
//             False if pixel hashing was not supported for one or both files.
// attempted: true if pixel hashing was attempted.
// err: any critical error encountered during pixel hashing (not ErrUnsupportedForPixelHashing).
// hash1, hash2: the pixel hashes if obtained.
func compareByPixelHash(filePath1, filePath2 string) (match bool, conclusive bool, attempted bool, err error, hash1 string, hash2 string) {
	attempted = true // Mark that we are attempting pixel hash comparison.

	pxHash1, errPx1 := CalculatePixelDataHash(filePath1)
	if errPx1 != nil {
		if strings.Contains(errPx1.Error(), ErrUnsupportedForPixelHashing.Error()) {
			fmt.Printf("Info: Pixel hash unsupported for %s.\n", filePath1)
			// Store "unsupported" for hash1 to indicate attempt? For now, leave empty.
			// Try to hash filePath2 to see if it's also unsupported.
			pxHash2, errPx2 := CalculatePixelDataHash(filePath2)
			if errPx2 != nil && strings.Contains(errPx2.Error(), ErrUnsupportedForPixelHashing.Error()) {
				// Both unsupported, not conclusive for pixel hash, no match here.
				return false, false, true, nil, "", ""
			} else if errPx2 == nil {
				// File1 unsupported, File2 supported. Not conclusive for pixel hash.
				return false, false, true, nil, "", pxHash2
			}
			// File1 unsupported, File2 had a different error.
			return false, false, true, errPx2, "", "" // Return error from filePath2
		}
		// Critical error for filePath1.
		return false, false, true, errPx1, "", ""
	}
	hash1 = pxHash1 // Store successful hash for filePath1

	pxHash2, errPx2 := CalculatePixelDataHash(filePath2)
	if errPx2 != nil {
		if strings.Contains(errPx2.Error(), ErrUnsupportedForPixelHashing.Error()) {
			fmt.Printf("Info: Pixel hash for %s succeeded, but unsupported for %s.\n", filePath1, filePath2)
			// FilePath1 hashed, FilePath2 unsupported. Not conclusive by pixel hash.
			return false, false, true, nil, hash1, "" // hash2 can be empty or "unsupported"
		}
		// Critical error for filePath2 after filePath1 succeeded.
		return false, false, true, errPx2, hash1, ""
	}
	hash2 = pxHash2 // Store successful hash for filePath2

	// Both pixel hashes calculated successfully.
	if hash1 == hash2 {
		return true, true, true, nil, hash1, hash2 // Match, conclusive.
	}
	// Pixel hashes differ.
	return false, true, true, nil, hash1, hash2 // No match, conclusive.
}

// compareByFileHash compares two files using their full file content hashes.
// match: true if file hashes were successfully computed for both and they are identical.
// err: any critical error encountered during file hashing.
// hash1, hash2: the file hashes if obtained.
func compareByFileHash(filePath1, filePath2 string) (match bool, err error, hash1 string, hash2 string) {
	fHash1, errFf1 := CalculateFileHash(filePath1)
	if errFf1 != nil {
		return false, fmt.Errorf("error full file hashing for %s: %w", filePath1, errFf1), "", ""
	}
	hash1 = fHash1

	fHash2, errFf2 := CalculateFileHash(filePath2)
	if errFf2 != nil {
		return false, fmt.Errorf("error full file hashing for %s: %w", filePath2, errFf2), hash1, ""
	}
	hash2 = fHash2

	if hash1 == hash2 {
		return true, nil, hash1, hash2 // Match
	}
	return false, nil, hash1, hash2 // No match
}

const (
	ReasonSizeMismatch          = "size_mismatch"
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
		exifMatch, exifConclusive, exifErr, exifSig1, exifSig2 := compareByExif(filePath1, filePath2)
		result.Hash1 = exifSig1 // Store whatever EXIF sigs were found
		result.Hash2 = exifSig2
		result.HashType = HashTypeExif // Default to EXIF hash type if this stage is entered

		if exifErr != nil {
			// An actual error occurred during EXIF processing.
			// Log it and treat EXIF comparison as inconclusive, then proceed to pixel hash.
			// Alternatively, could return the error: result.Reason = ReasonError; return result, exifErr;
			fmt.Printf("Warning: EXIF comparison error for %s, %s: %v. Proceeding to pixel hash.\n", filePath1, filePath2, exifErr)
			result.Reason = ReasonNotCompared // EXIF check was inconclusive due to error
		} else if exifConclusive {
			if !exifMatch { // EXIF mismatch, conclusive
				result.Reason = ReasonExifMismatch
				// AreDuplicates remains false
				return result, nil
			}
			// This case (exifConclusive and exifMatch) shouldn't happen based on compareByExif logic
			// as a match is currently considered inconclusive. If it did, it means EXIF matched and is conclusive.
			// For now, assume it implies proceeding.
		}
		// If EXIF matched (exifMatch is true, exifConclusive is false),
		// or if EXIF was inconclusive (e.g., one or both missing EXIF, exifMatch is false, exifConclusive is false),
		// we proceed to pixel hash.
		// result.Reason will be updated by pixel/file hash if EXIF was not a mismatch.
		// If EXIF matched, Hash1, Hash2, and HashType are already set.

		// 3.b Pixel Data Hash Comparison (for images)
		pxMatch, pxConclusive, pxAttempted, pxErr, pxSig1, pxSig2 := compareByPixelHash(filePath1, filePath2)
		pixelHashingAttemptedOrUnsupported = pxAttempted // Update based on whether pixel hash was attempted

		if pxErr != nil {
			result.Reason = ReasonError
			return result, fmt.Errorf("error during pixel hash comparison for %s and %s: %w", filePath1, filePath2, pxErr)
		}

		result.Hash1 = pxSig1 // Store pixel hash attempt for file1 (even if partial or only one file hashed)
		result.Hash2 = pxSig2 // Store pixel hash attempt for file2

		if pxConclusive {
			result.HashType = HashTypePixel
			result.AreDuplicates = pxMatch
			if pxMatch {
				result.Reason = ReasonPixelHashMatch
			} else {
				result.Reason = ReasonPixelHashMismatch
			}
			return result, nil // Pixel hash comparison was conclusive
		}
		// If pixel hash was not conclusive (e.g., unsupported format for one/both)
		// pixelHashingAttemptedOrUnsupported is true.
		// Hash1 and Hash2 from compareByPixelHash might contain one valid hash if the other was unsupported.
		// The EXIF hashes are still in result.Hash1, result.Hash2 from the EXIF stage if EXIF was present.
		// We must ensure correct hashes are carried to file hash stage if needed.
		// If EXIF was present and matched, result.Hash1/2 are EXIF. If pixel hash was attempted,
		// pxSig1/2 from compareByPixelHash should be preferred if available.
		// For now, if pxConclusive is false, we fall through to file hashing.
		// The existing logic for pixelHashingAttemptedOrUnsupported will guide the file size check.
		// Ensure HashType reflects that we are now likely falling back or decided by non-pixel method.
		if result.HashType == HashTypeExif { // If EXIF was the last thing stored, and pixel hash wasn't conclusive
			// We might want to clear Hash1/Hash2 if they are EXIF, as file hash is next.
			// Or, let file hash overwrite them. Current logic overwrites.
		}
		result.Reason = ReasonPixelHashNotAttempted // Or more specific if one was unsupported
		// Ensure HashType is set to File if we are falling through.
		// This will be set later before full file hash.
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

	fileMatch, fileErr, fSig1, fSig2 := compareByFileHash(filePath1, filePath2)
	result.Hash1 = fSig1
	result.Hash2 = fSig2
	result.HashType = HashTypeFile // Set hash type for this stage

	if fileErr != nil {
		result.Reason = ReasonError
		return result, fileErr // Propagate error from file hashing
	}

	if fileMatch {
		result.AreDuplicates = true
		result.Reason = ReasonFileHashMatch
	} else {
		result.AreDuplicates = false // Explicitly set, though default
		result.Reason = ReasonFileHashMismatch
	}
	return result, nil
}
