package pkg_test

import (
	"errors" // Added for errors.Is
	"github.com/user/photo-sorter/pkg"
	"os"
	"path/filepath"
	"reflect"
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
