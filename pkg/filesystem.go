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

// imageExtensions maps common image file extensions to true.
// Used by ScanSourceDirectory and IsImageExtension.
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

// ScanSourceDirectory recursively scans the source directory for image files
// based on the imageExtensions map.
// See docs/technical_details.md for more details on scanning and error handling.
func ScanSourceDirectory(sourceDir string) ([]string, error) {
	var imageFiles []string

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
		return nil, fmt.Errorf("error walking through source directory '%s': %w", sourceDir, err)
	}

	if imageFiles == nil {
		return []string{}, nil // Return empty slice instead of nil
	}
	return imageFiles, nil
}

// CreateTargetDirectory creates the year/month directory structure (YYYY/MM)
// within the target base directory.
func CreateTargetDirectory(targetBaseDir string, date time.Time) (string, error) {
	yearDir := filepath.Join(targetBaseDir, date.Format("2006"))
	monthDir := filepath.Join(yearDir, date.Format("01")) // 01 for MM (two digits)

	if err := os.MkdirAll(monthDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory %s: %w", monthDir, err)
	}
	return monthDir, nil
}

// GetPhotoCreationDate extracts the creation date from a photo's EXIF data.
// It prioritizes DateTimeOriginal and falls back to DateTimeDigitized.
// Returns ErrNoExifDate if no suitable date tag is found.
// See docs/technical_details.md for more on EXIF parsing logic.
func GetPhotoCreationDate(photoPath string) (time.Time, error) {
	file, err := os.Open(photoPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to open file %s: %w", photoPath, err)
	}
	defer file.Close()

	x, err := exif.Decode(file)
	if err != nil {
		// Broadly categorizing EXIF decoding issues.
		return time.Time{}, fmt.Errorf("failed to decode EXIF data from %s: %w", photoPath, err)
	}

	dateTag, err := x.Get(exif.DateTimeOriginal)
	if err == nil {
		return parseExifDateTime(dateTag)
	}

	dateTag, err = x.Get(exif.DateTimeDigitized)
	if err == nil {
		return parseExifDateTime(dateTag)
	}

	return time.Time{}, ErrNoExifDate
}

// parseExifDateTime is a helper to parse EXIF datetime string.
// Handles "YYYY:MM:DD HH:MM:SS" and fallback "YYYY:MM:DD".
// See docs/technical_details.md for more on EXIF date parsing.
func parseExifDateTime(tag *tiff.Tag) (time.Time, error) {
	if tag == nil {
		return time.Time{}, fmt.Errorf("tag is nil")
	}
	dateStr, err := tag.StringVal() // Handles potential null terminators.
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get string value from EXIF date tag: %w", err)
	}

	layout := "2006:01:02 15:04:05" // Common EXIF datetime format
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		// Fallback for date-only EXIF tags
		layoutDateOnly := "2006:01:02"
		t, errDateOnly := time.Parse(layoutDateOnly, dateStr)
		if errDateOnly != nil {
			return time.Time{}, fmt.Errorf("failed to parse EXIF date string '%s' with layout '%s' or '%s': %w", dateStr, layout, layoutDateOnly, err)
		}
		return t, nil
	}
	return t, nil
}

// FindPotentialTargetConflicts returns a list of file paths in targetMonthDir
// that could conflict with a new base name (e.g., "YYYY-MM-DD-HHMMSS.jpg", "YYYY-MM-DD-HHMMSS-1.jpg").
// See docs/technical_details.md for details on the filename matching logic.
func FindPotentialTargetConflicts(targetMonthDir, baseNameWithoutExt, extension string) ([]string, error) {
	var conflictingFiles []string

	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	normalizedExtension := strings.ToLower(extension)

	entries, err := os.ReadDir(targetMonthDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil // Directory doesn't exist, so no conflicts
		}
		return nil, fmt.Errorf("failed to read target directory %s: %w", targetMonthDir, err)
	}

	prefix := baseNameWithoutExt
	suffix := normalizedExtension

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		entryName := entry.Name()
		entryNameLower := strings.ToLower(entryName)

		if strings.HasPrefix(entryNameLower, strings.ToLower(prefix)) && strings.HasSuffix(entryNameLower, suffix) {
			middlePart := entryName[len(prefix) : len(entryName)-len(extension)]

			if middlePart == "" {
				conflictingFiles = append(conflictingFiles, filepath.Join(targetMonthDir, entryName))
				continue
			}

			if strings.HasPrefix(middlePart, "-") {
				versionStr := middlePart[1:]
				if versionStr == "" {
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

// IsImageExtension checks if the given filePath has a known image extension
// by comparing its lowercased extension against the imageExtensions map.
func IsImageExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	_, exists := imageExtensions[ext]
	return exists
}
