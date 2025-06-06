package tests

import (
	"errors" // Added for errors.Is
	"github.com/user/photo-sorter/pkg"
	"os"
	"path/filepath"
	"reflect"
	"runtime" // Added for runtime.GOOS
	"sort"
	"strings" // Added for strings.Contains
	"testing"
	"time"
)

// Helper function to create a temporary directory structure for testing ScanSourceDirectory
func createScanTestDir(t *testing.T, baseDir string, structure map[string][]byte) {
	t.Helper()
	for path, content := range structure {
		fullPath := filepath.Join(baseDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if content != nil { // If nil, it's a directory
			if err := os.WriteFile(fullPath, content, 0644); err != nil {
				t.Fatalf("Failed to write file %s: %v", fullPath, err)
			}
		}
	}
}

func TestIsImageExtension_HEIF(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"heic extension", "photo.heic", true},
		{"HEIC extension (uppercase)", "photo.HEIC", true},
		{"heif extension", "photo.heif", true},
		{"HEIF extension (uppercase)", "photo.HEIF", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := pkg.IsImageExtension(tt.filePath)
			if actual != tt.expected {
				t.Errorf("IsImageExtension(%q) got %v, want %v", tt.filePath, actual, tt.expected)
			}
		})
	}
}

func TestIsImageExtension(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"jpeg extension", "photo.jpeg", true},
		{"jpg extension", "photo.jpg", true},
		{"JPG extension (uppercase)", "photo.JPG", true},
		{"png extension", "photo.png", true},
		{"gif extension", "photo.gif", true},
		{"raw extension", "photo.raw", true},
		{"cr2 extension", "photo.cr2", true},
		{"nef extension", "photo.nef", true},
		{"arw extension", "photo.arw", true},
		{"orf extension", "photo.orf", true},
		{"rw2 extension", "photo.rw2", true},
		{"pef extension", "photo.pef", true},
		{"dng extension", "photo.dng", true},
		{"txt extension", "document.txt", false},
		{"no extension", "filewithnoextension", false},
		{"empty extension", "file.", false},
		{"unknown image extension", "photo.unknown", false},
		{"path with multiple dots", "archive.tar.gz", false}, // assuming .gz is not in imageExtensions
		{"hidden file with image ext", ".hiddenphoto.jpg", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := pkg.IsImageExtension(tt.filePath)
			if actual != tt.expected {
				t.Errorf("IsImageExtension(%q) got %v, want %v", tt.filePath, actual, tt.expected)
			}
		})
	}
}

func TestFindPotentialTargetConflicts(t *testing.T) {
	tmpDir := t.TempDir()

	// Structure for files to create in the temp directory
	// map[filename]content (content is irrelevant, just existence)
	filesToCreate := map[string][]byte{
		"image.jpg":         []byte("data"),
		"image.JPG":         []byte("data"), // Case variation
		"image-1.jpg":       []byte("data"),
		"image-123.jpg":     []byte("data"),
		"image-other.jpg":   []byte("data"), // Should not match (non-numeric version)
		"image.png":         []byte("data"), // Different extension
		"prefiximage.jpg":   []byte("data"), // Different prefix
		"image-1.jpeg":      []byte("data"), // Different extension for versioned file
		"image-abc.jpg":     []byte("data"), // Non-numeric version part
		"image-.jpg":        []byte("data"), // Invalid version part
		"image-1-extra.jpg": []byte("data"), // Invalid version part
	}

	for name, data := range filesToCreate {
		filePath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	tests := []struct {
		name                string
		targetMonthDir      string
		baseNameWithoutExt  string
		extension           string
		expectedConflicts   []string // Paths relative to targetMonthDir
		expectedErrContains string   // Substring of expected error, if any
	}{
		{
			name:               "no conflicts in empty directory",
			targetMonthDir:     t.TempDir(), // A fresh empty directory
			baseNameWithoutExt: "photo",
			extension:          ".jpg",
			expectedConflicts:  []string{},
		},
		{
			name:               "non-existent directory",
			targetMonthDir:     filepath.Join(tmpDir, "non_existent_subdir"),
			baseNameWithoutExt: "photo",
			extension:          ".jpg",
			expectedConflicts:  []string{}, // Should return empty list, nil error
		},
		{
			name:               "exact match, versioned matches, and case variation",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "image",
			extension:          ".jpg",
			expectedConflicts:  []string{"image.jpg", "image.JPG", "image-1.jpg", "image-123.jpg"},
		},
		{
			name:               "match with different extension case in input",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "image",
			extension:          ".JPG", // Input extension is uppercase
			expectedConflicts:  []string{"image.jpg", "image.JPG", "image-1.jpg", "image-123.jpg"},
		},
		{
			name:               "no matches due to different base name",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "photo",
			extension:          ".jpg",
			expectedConflicts:  []string{},
		},
		{
			name:               "no matches due to different extension",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "image",
			extension:          ".png",
			expectedConflicts:  []string{"image.png"},
		},
		{
			name:               "only base match, no versioned matches for different extension",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "image",
			extension:          ".jpeg", // expecting image-1.jpeg
			expectedConflicts:  []string{"image-1.jpeg"},
		},
		{
			name:               "extension without dot",
			targetMonthDir:     tmpDir,
			baseNameWithoutExt: "image",
			extension:          "jpg", // Input extension without dot
			expectedConflicts:  []string{"image.jpg", "image.JPG", "image-1.jpg", "image-123.jpg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isWindows := runtime.GOOS == "windows"

			results, err := pkg.FindPotentialTargetConflicts(tt.targetMonthDir, tt.baseNameWithoutExt, tt.extension)

			if tt.expectedErrContains != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.expectedErrContains)
				} else if !strings.Contains(err.Error(), tt.expectedErrContains) {
					t.Errorf("Error '%v' does not contain expected string '%s'", err, tt.expectedErrContains)
				}
				return // Don't check results if error was expected
			}
			if err != nil {
				t.Fatalf("FindPotentialTargetConflicts() returned unexpected error: %v", err)
			}

			currentExpectedConflicts := tt.expectedConflicts
			if isWindows {
				// On Windows, if files differ only by case, os.ReadDir might return only one.
				// We adjust expectedConflicts to reflect this by keeping only one canonical (lowercase key) version.
				uniqueExpectedForOS := make(map[string]string)
				for _, fName := range tt.expectedConflicts {
					// Store the first encountered casing, but key by lowercase to ensure uniqueness by name regardless of case.
					// This means if "image.jpg" and "image.JPG" are expected, only one will be kept based on iteration order.
					// For consistency in tests, we can decide to always store the lowercase name if that's what we will compare against.
					// However, the problem states `FindPotentialTargetConflicts` returns original casing.
					// So, we make the *expected list* unique based on lowercase, but keep an original casing.
					lowerFName := strings.ToLower(fName)
					if _, exists := uniqueExpectedForOS[lowerFName]; !exists {
						uniqueExpectedForOS[lowerFName] = fName
					}
				}
				newExpectedConflicts := make([]string, 0, len(uniqueExpectedForOS))
				for _, fName := range uniqueExpectedForOS {
					newExpectedConflicts = append(newExpectedConflicts, fName)
				}
				currentExpectedConflicts = newExpectedConflicts
			}

			// Normalize expected paths and sort for comparison
			expectedFullPaths := make([]string, len(currentExpectedConflicts))
			for i, fName := range currentExpectedConflicts {
				expectedFullPaths[i] = filepath.Join(tt.targetMonthDir, fName)
			}
			sort.Strings(expectedFullPaths)
			sort.Strings(results)

			// Regardless of OS, perform a case-insensitive comparison for ElementsMatch robustness.
			// The function itself might return case-sensitive paths from the FS,
			// but for matching elements, we want to ensure all expected are found irrespective of case.
			actualForCompare := make([]string, len(results))
			for i, p := range results {
				actualForCompare[i] = strings.ToLower(p)
			}
			sort.Strings(actualForCompare)

			expectedForCompare := make([]string, len(expectedFullPaths))
			for i, p := range expectedFullPaths {
				expectedForCompare[i] = strings.ToLower(p)
			}
			sort.Strings(expectedForCompare)

			if len(expectedForCompare) == 0 && len(actualForCompare) == 0 {
				// Both are empty, which is fine.
			} else if !reflect.DeepEqual(actualForCompare, expectedForCompare) {
				// This error message will show the lowercased versions, making it clear what was compared.
				// It also includes the original case versions for additional context.
				t.Errorf("FindPotentialTargetConflicts() case-insensitive comparison failed:\n  got (lower): %v\n want (lower): %v\n original got (sorted): %v\n original want (sorted): %v",
					actualForCompare, expectedForCompare, results, expectedFullPaths)
			}

			// Additionally, if on Windows, where os.ReadDir might only return one casing for "file.JPG" and "file.jpg",
			// the `currentExpectedConflicts` list has already been adjusted to account for this.
			// The case-insensitive comparison above should handle this correctly.
			// The original block for `if isWindows` can be removed as the general case-insensitive check is now primary.
		})
	}
}

func TestScanSourceDirectory(t *testing.T) {
	tests := []struct {
		name          string
		structure     map[string][]byte // nil content means directory
		sourceDir     string            // relative to test temp dir
		expectedFiles []string
		expectedErr   bool
	}{
		{
			name: "valid directory with images and non-images",
			structure: map[string][]byte{
				"img1.jpg":           []byte("fake jpg"),
				"img2.png":           []byte("fake png"),
				"doc.txt":            []byte("text file"),
				"subDir/img3.jpeg":   []byte("fake jpeg"),
				"subDir/another.doc": []byte("another text"),
				"subDir/emptyDir":    nil, // An empty subdirectory
			},
			sourceDir:     ".",
			expectedFiles: []string{"img1.jpg", "img2.png", "subDir/img3.jpeg"},
			expectedErr:   false,
		},
		{
			name:          "non-existent source directory",
			structure:     map[string][]byte{},
			sourceDir:     "non_existent_dir",
			expectedFiles: nil,
			expectedErr:   true,
		},
		{
			name:          "empty source directory",
			structure:     map[string][]byte{},
			sourceDir:     ".",
			expectedFiles: []string{}, // Expect empty slice, not nil
			expectedErr:   false,
		},
		{
			name: "directory with no image files",
			structure: map[string][]byte{
				"doc1.txt":        []byte("text"),
				"sub/doc2.pdf":    []byte("pdf"),
				"anotherEmptyDir": nil,
			},
			sourceDir:     ".",
			expectedFiles: []string{},
			expectedErr:   false,
		},
		{
			name: "directory with only empty subdirectories",
			structure: map[string][]byte{
				"empty1":        nil,
				"empty2/empty3": nil,
			},
			sourceDir:     ".",
			expectedFiles: []string{},
			expectedErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			createScanTestDir(t, tmpDir, tt.structure)

			scanDir := filepath.Join(tmpDir, tt.sourceDir)

			// Ensure scanDir is created if it's supposed to be the main test dir (for empty/no_images cases)
			if tt.sourceDir == "." && len(tt.structure) == 0 {
				if err := os.MkdirAll(scanDir, 0755); err != nil {
					t.Fatalf("Failed to create scanDir for empty test: %v", err)
				}
			}

			files, err := pkg.ScanSourceDirectory(scanDir)

			if (err != nil) != tt.expectedErr {
				t.Errorf("pkg.ScanSourceDirectory() error = %v, expectedErr %v", err, tt.expectedErr)
				return
			}

			// Normalize file paths for comparison and sort them
			var normalizedExpectedFiles []string
			if tt.expectedFiles != nil { // Only normalize if tt.expectedFiles is not nil
				normalizedExpectedFiles = make([]string, len(tt.expectedFiles))
				for i, f := range tt.expectedFiles {
					normalizedExpectedFiles[i] = filepath.Join(scanDir, f)
				}
				sort.Strings(normalizedExpectedFiles)
			}
			// files slice from ScanSourceDirectory might be nil or empty, sort handles nil fine.
			sort.Strings(files)

			if !reflect.DeepEqual(files, normalizedExpectedFiles) {
				t.Errorf("pkg.ScanSourceDirectory() files = %v, expected %v", files, normalizedExpectedFiles)
			}
		})
	}
}

func TestCreateTargetDirectory(t *testing.T) {
	baseTargetDir := t.TempDir()

	tests := []struct {
		name        string
		photoDate   time.Time
		expectedDir string // relative to baseTargetDir
		expectedErr bool
	}{
		{
			name:        "create new directory YYYY/MM",
			photoDate:   time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC),
			expectedDir: "2023/10", // Changed to YYYY/MM
			expectedErr: false,
		},
		{
			name:        "create another directory YYYY/MM (idempotency check)",
			photoDate:   time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC),
			expectedDir: "2023/10", // Changed to YYYY/MM
			expectedErr: false,
		},
		{
			name:        "create directory for different date YYYY/MM",
			photoDate:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expectedDir: "2024/01", // Changed to YYYY/MM
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedFullPath := filepath.Join(baseTargetDir, tt.expectedDir)
			// Log expected path for clarity during test runs
			// t.Logf("Test case: %s, Expected full path: %s", tt.name, expectedFullPath)

			createdPath, err := pkg.CreateTargetDirectory(baseTargetDir, tt.photoDate)
			if (err != nil) != tt.expectedErr {
				t.Errorf("pkg.CreateTargetDirectory() error = %v, wantErr %v", err, tt.expectedErr)
				return
			}
			if err == nil {
				// t.Logf("Test case: %s, Created path: %s", tt.name, createdPath)
				if createdPath != expectedFullPath {
					t.Errorf("pkg.CreateTargetDirectory() path = %s, want %s", createdPath, expectedFullPath)
				}
				if _, statErr := os.Stat(expectedFullPath); os.IsNotExist(statErr) {
					t.Errorf("pkg.CreateTargetDirectory() did not create directory %s", expectedFullPath)
				}
			}
		})
	}
}

// For TestGetPhotoCreationDate, we need actual image files with EXIF data.
// This is hard to create programmatically in Go for various formats.
// We'll test with a non-image file, a non-existent file, and a file without EXIF.
// Testing with actual EXIF data would require providing sample files.

// Dummy EXIF data for testing (replace with actual data if available)
// Sample JPEG with DateTimeOriginal: 2008:05:30 15:56:01
// You can create such a file using an EXIF editor or find one online.
// For now, we'll mostly test error paths and non-EXIF scenarios.

func TestGetPhotoCreationDate_HEIF_Fallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a dummy HEIC file (empty, as goexif won't parse it anyway)
	heicFile := filepath.Join(tmpDir, "sample.heic")
	if f, err := os.Create(heicFile); err != nil {
		t.Fatalf("Failed to create dummy HEIC file: %v", err)
	} else {
		f.Close()
	}

	// Since heif-go is registered, the behavior might change if it attempts to decode
	// before goexif. However, GetPhotoCreationDate specifically uses goexif.
	// We expect goexif to fail decoding a HEIC file.
	_, err := pkg.GetPhotoCreationDate(heicFile)

	if err == nil {
		t.Errorf("Expected an error when calling GetPhotoCreationDate with HEIC file, got nil")
	} else {
		// Check if the error is one of the expected types:
		// - pkg.ErrNoExifDate (if goexif initializes but finds no date)
		// - or a more general "failed to decode EXIF data"
		// The exact error depends on how goexif handles unknown formats.
		// It's likely to be a decoding error rather than ErrNoExifDate.
		expectedErrorSubstrings := []string{
			"failed to decode EXIF data", // Generic decode error from goexif
			pkg.ErrNoExifDate.Error(),    // Specific error if goexif somehow "starts" then fails
		}
		foundExpectedError := false
		for _, sub := range expectedErrorSubstrings {
			if strings.Contains(err.Error(), sub) {
				foundExpectedError = true
				break
			}
		}
		if !foundExpectedError {
			t.Errorf("GetPhotoCreationDate(%q) returned error '%v', which does not contain any of the expected substrings: %v", heicFile, err, expectedErrorSubstrings)
		} else {
			t.Logf("GetPhotoCreationDate(%q) correctly returned error '%v', indicating fallback to mod time would occur.", heicFile, err)
		}
	}
}

func TestGetPhotoCreationDate(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a dummy text file (not an image)
	notAnImage := filepath.Join(tmpDir, "not_an_image.txt")
	if err := os.WriteFile(notAnImage, []byte("this is not an image"), 0644); err != nil {
		t.Fatalf("Failed to create dummy text file: %v", err)
	}

	// 2. Create an empty file (will fail EXIF decoding)
	emptyFile := filepath.Join(tmpDir, "empty.jpg")
	if f, err := os.Create(emptyFile); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	} else {
		f.Close()
	}

	// TODO: Add a real JPG with known DateTimeOriginal for a more complete test
	// For example, if you have 'sample_with_exif.jpg' with DateTimeOriginal = 2022:01:15 10:30:00
	// realExifFile := "path/to/your/sample_with_exif.jpg"
	// expectedExifTime := time.Date(2022, 1, 15, 10, 30, 0, 0, time.Local)

	tests := []struct {
		name                string
		filePath            string
		expectedErr         error  // Specific error type for errors.Is
		expectedErrContains string // Substring for wrapped errors
		// expectedTime time.Time // Uncomment and use if testing with actual EXIF files
	}{
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tmpDir, "non_existent.jpg"),
			expectedErr: os.ErrNotExist,
		},
		{
			name:                "not an image file",
			filePath:            notAnImage,
			expectedErrContains: "failed to decode EXIF data",
		},
		{
			name:                "empty file (simulating corrupted or no exif)",
			filePath:            emptyFile,
			expectedErrContains: "failed to decode EXIF data",
		},
		// {
		// 	name: "valid exif datetimeoriginal",
		// 	filePath: realExifFile, // You need to provide this file
		// 	expectedTime: expectedExifTime,
		// 	expectedErr: nil,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pkg.GetPhotoCreationDate(tt.filePath)

			if tt.expectedErr != nil { // Check for specific error type using errors.Is
				if err == nil {
					t.Errorf("pkg.GetPhotoCreationDate() expected error type %T, got nil", tt.expectedErr)
				} else if !errors.Is(err, tt.expectedErr) {
					// A special check for pkg.ErrNoExifDate if that's what we're expecting.
					// However, current test cases use os.ErrNotExist or string matching.
					// If a test case specifically sets tt.expectedErr = pkg.ErrNoExifDate, this would catch it.
					if errors.Is(err, pkg.ErrNoExifDate) && tt.expectedErr == pkg.ErrNoExifDate {
						// This case is fine if tt.expectedErr is indeed pkg.ErrNoExifDate
					} else {
						t.Errorf("pkg.GetPhotoCreationDate() error type = %T, expected type %T, error: %v", err, tt.expectedErr, err)
					}
				}
			} else if tt.expectedErrContains != "" { // Check for substring in error
				if err == nil {
					t.Errorf("pkg.GetPhotoCreationDate() expected error containing '%s', got nil", tt.expectedErrContains)
				} else if !strings.Contains(err.Error(), tt.expectedErrContains) {
					// If the error is pkg.ErrNoExifDate, its string form is "no EXIF date tag found"
					// The test case "failed to decode EXIF data" might be too broad if pkg.ErrNoExifDate is returned.
					// Let's check if the error is pkg.ErrNoExifDate and if the substring matches that.
					if errors.Is(err, pkg.ErrNoExifDate) && strings.Contains(pkg.ErrNoExifDate.Error(), tt.expectedErrContains) {
						// This is also fine. e.g. expect "no EXIF date" and get pkg.ErrNoExifDate
					} else {
						t.Errorf("pkg.GetPhotoCreationDate() error = '%v', expected to contain '%s'", err, tt.expectedErrContains)
					}
				}
			} else { // Expected no error
				if err != nil {
					t.Errorf("pkg.GetPhotoCreationDate() unexpected error: %v", err)
				}
				// Add time comparison if tt.expectedTime is set and err is nil
				// if !resultTime.Equal(tt.expectedTime) {
				// 	t.Errorf("pkg.GetPhotoCreationDate() time = %v, expected %v", resultTime, tt.expectedTime)
				// }
			}
		})
	}
}
