package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

// ErrNoExifDate is returned when EXIF data is found but no suitable date tag is present.
var ErrNoExifDate = fmt.Errorf("no EXIF date tag found")

var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".raw":  true,
	".cr2":  true,
	".nef":  true,
	".arw":  true,
	".orf":  true,
	".rw2":  true,
	".pef":  true,
	".dng":  true,
	// Add more extensions if needed
}

// ScanSourceDirectory recursively scans the source directory for image files.
func ScanSourceDirectory(sourceDir string) ([]string, error) {
	var imageFiles []string

	// Check if the source directory exists and is readable
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("source directory '%s' does not exist", sourceDir)
		}
		return nil, fmt.Errorf("error accessing source directory '%s': %w", sourceDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path '%s' is not a directory", sourceDir)
	}

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files/directories that can't be read, but log the error
			fmt.Printf("Warning: Error accessing path %q: %v\n", path, err)
			return nil // Returning nil continues the walk
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if imageExtensions[ext] {
				imageFiles = append(imageFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		// This error would be from filepath.Walk itself, not the callback.
		return nil, fmt.Errorf("error walking through source directory '%s': %w", sourceDir, err)
	}

	if imageFiles == nil {
		return []string{}, nil // Return empty slice instead of nil
	}
	return imageFiles, nil
}

// CreateTargetDirectory creates the year/month directory structure within the target base directory.
// Example: targetBaseDir/YYYY/MM
func CreateTargetDirectory(targetBaseDir string, date time.Time) (string, error) {
	yearDir := filepath.Join(targetBaseDir, date.Format("2006"))
	monthDir := filepath.Join(yearDir, date.Format("01")) // 01 for MM

	// Create the year directory if it doesn't exist
	if err := os.MkdirAll(monthDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory %s: %w", monthDir, err)
	}
	return monthDir, nil // Return the YYYY/MM path
}

// GetPhotoCreationDate extracts the creation date from a photo's EXIF data.
// It looks for DateTimeOriginal, CreateDate, or DateTimeDigitized tags.
// If no EXIF date is found, it returns ErrNoExifDate.
// If the file cannot be opened or EXIF data cannot be decoded, other errors are returned.
func GetPhotoCreationDate(photoPath string) (time.Time, error) {
	file, err := os.Open(photoPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to open file %s: %w", photoPath, err)
	}
	defer file.Close()

	x, err := exif.Decode(file)
	if err != nil {
		// If it's a "no EXIF data" error, we can return a more specific error
		// or handle it as a non-critical issue (e.g., fallback to mod time).
		// For now, let's check if it's a known "no EXIF" scenario.
		// The exif library might return io.EOF or other parsing errors for non-EXIF files.
		// We'll treat any decoding error as "EXIF data not usable".
		return time.Time{}, fmt.Errorf("failed to decode EXIF data from %s: %w", photoPath, err)
	}

	// Preferred tag: DateTimeOriginal
	dateTag, err := x.Get(exif.DateTimeOriginal)
	if err == nil {
		return parseExifDateTime(dateTag)
	}

	// Fallback tag: DateTimeDigitized
	dateTag, err = x.Get(exif.DateTimeDigitized)
	if err == nil {
		return parseExifDateTime(dateTag)
	}

	return time.Time{}, ErrNoExifDate // No suitable date tag found
}

// parseExifDateTime is a helper to parse EXIF datetime string.
// EXIF datetime format is "YYYY:MM:DD HH:MM:SS".
func parseExifDateTime(tag *tiff.Tag) (time.Time, error) {
	if tag == nil {
		return time.Time{}, fmt.Errorf("tag is nil")
	}
	// The string value of the tag might be enclosed in quotes or have null terminators.
	// exif.Tag.StringVal() handles this.
	dateStr, err := tag.StringVal()
	if err != nil {
		// Removed tag.Name from the error as it's not available.
		return time.Time{}, fmt.Errorf("failed to get string value from EXIF date tag: %w", err)
	}

	// EXIF standard date/time format: "YYYY:MM:DD HH:MM:SS"
	// Sometimes it can also have timezone information, or be just a date.
	// For simplicity, we'll try to parse the common format first.
	layout := "2006:01:02 15:04:05"
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		// Try parsing without time if the first parse fails - some cameras might only store date
		layoutDateOnly := "2006:01:02"
		t, errDateOnly := time.Parse(layoutDateOnly, dateStr)
		if errDateOnly != nil {
			// Return the original error if date-only parsing also fails
			return time.Time{}, fmt.Errorf("failed to parse EXIF date string '%s' with layout '%s' or '%s': %w", dateStr, layout, layoutDateOnly, err)
		}
		return t, nil
	}
	return t, nil
}

// FindPotentialTargetConflicts returns a list of file paths in targetMonthDir
// that could conflict with newBaseName (e.g., "2023-10-27-153000.jpg",
// "2023-10-27-153000-1.jpg", etc.)
// baseNameWithoutExt should not include the extension.
// extension should include the dot (e.g., ".jpg").
func FindPotentialTargetConflicts(targetMonthDir, baseNameWithoutExt, extension string) ([]string, error) {
	var conflictingFiles []string

	// Ensure extension starts with a dot
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	// Normalize extension to lowercase for case-insensitive comparison
	normalizedExtension := strings.ToLower(extension)

	entries, err := os.ReadDir(targetMonthDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil // Directory doesn't exist, so no conflicts
		}
		return nil, fmt.Errorf("failed to read target directory %s: %w", targetMonthDir, err)
	}

	// Regex pattern: baseNameWithoutExt + "(-[0-9]+)?" + extension
	// Example: "2023-10-27-153000" + "(-[0-9]+)?" + ".jpg"
	// This will match:
	// 2023-10-27-153000.jpg
	// 2023-10-27-153000-1.jpg
	// 2023-10-27-153000-123.jpg
	// It will not match:
	// 2023-10-27-153000-abc.jpg (non-digit version)
	// 2023-10-27-153000--1.jpg (double hyphen)
	//
	// We need to escape baseNameWithoutExt and extension for regex, though typically they won't have special chars.
	// For simplicity and given the controlled nature of these inputs, direct string matching is safer and clearer.

	prefix := baseNameWithoutExt
	suffix := normalizedExtension

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		entryName := entry.Name()
		entryNameLower := strings.ToLower(entryName)

		if strings.HasPrefix(entryNameLower, strings.ToLower(prefix)) && strings.HasSuffix(entryNameLower, suffix) {
			// Check the part between prefix and suffix
			middlePart := entryName[len(prefix) : len(entryName)-len(extension)] // Use original case for slicing

			if middlePart == "" { // Exact match, e.g., baseNameWithoutExt.jpg
				conflictingFiles = append(conflictingFiles, filepath.Join(targetMonthDir, entryName))
				continue
			}

			// Check for -v pattern, e.g., baseNameWithoutExt-1.jpg
			if strings.HasPrefix(middlePart, "-") {
				versionStr := middlePart[1:] // remove the leading '-'
				if versionStr == "" {        // e.g. imagename-.jpg
					continue
				}
				allDigits := true
				for _, r := range versionStr {
					if r < '0' || r > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					conflictingFiles = append(conflictingFiles, filepath.Join(targetMonthDir, entryName))
				}
			}
		}
	}

	return conflictingFiles, nil
}

// IsImageExtension checks if the given filePath has a known image extension.
// It uses the internal imageExtensions map.
func IsImageExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	_, exists := imageExtensions[ext]
	return exists
}
