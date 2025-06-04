package pkg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/photo-sorter/pkg"
)

func TestGenerateReport(t *testing.T) {
	tmpDir := t.TempDir()

	reportFilePathRegular := filepath.Join(tmpDir, "report_regular.txt")
	reportFilePathNoDuplicates := filepath.Join(tmpDir, "report_no_duplicates.txt")
	invalidReportDir := "/proc/cannot_write_here" // Directory where file creation should fail
	invalidReportFilePath := filepath.Join(invalidReportDir, "report_invalid.txt")

	duplicateEntries := []pkg.DuplicateInfo{
		{KeptFile: "path/to/kept1.jpg", DiscardedFile: "path/to/discarded1.jpg", Reason: "Higher resolution"},
		{KeptFile: "path/to/kept2.png", DiscardedFile: "path/to/discarded2.png", Reason: "Duplicate content"},
	}

	tests := []struct {
		name                string
		reportPath          string
		duplicates          []pkg.DuplicateInfo
		copiedFilesCount    int
		processedFilesCount int
		filesToCopyCount    int
		expectErr           bool
		expectedSubstrings  []string // Substrings to check for in the report
	}{
		{
			name:                "report with duplicates",
			reportPath:          reportFilePathRegular,
			duplicates:          duplicateEntries,
			copiedFilesCount:    5,
			processedFilesCount: 10,
			filesToCopyCount:    7, // 5 copied + 2 from duplicates (kept versions)
			expectErr:           false,
			expectedSubstrings: []string{
				"Total files scanned: 10",
				"Files identified for copying (unique or better): 7",
				"Files successfully copied: 5",
				"Duplicate files found and discarded/skipped: 2",
				"Kept: path/to/kept1.jpg",
				"Discarded: path/to/discarded1.jpg",
				"Reason: Higher resolution",
				"Kept: path/to/kept2.png",
				"Discarded: path/to/discarded2.png",
				"Reason: Duplicate content",
			},
		},
		{
			name:                "report with no duplicates",
			reportPath:          reportFilePathNoDuplicates,
			duplicates:          []pkg.DuplicateInfo{},
			copiedFilesCount:    8,
			processedFilesCount: 8,
			filesToCopyCount:    8,
			expectErr:           false,
			expectedSubstrings: []string{
				"Total files scanned: 8",
				"Files identified for copying (unique or better): 8",
				"Files successfully copied: 8",
				"Duplicate files found and discarded/skipped: 0",
			},
		},
		{
			name:                "invalid report path (unwritable)",
			reportPath:          invalidReportFilePath,
			duplicates:          []pkg.DuplicateInfo{},
			copiedFilesCount:    0,
			processedFilesCount: 0,
			filesToCopyCount:    0,
			expectErr:           true,
			expectedSubstrings:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pkg.GenerateReport(tt.reportPath, tt.duplicates, tt.copiedFilesCount, tt.processedFilesCount, tt.filesToCopyCount)

			if (err != nil) != tt.expectErr {
				t.Errorf("pkg.GenerateReport() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr {
				if _, statErr := os.Stat(tt.reportPath); os.IsNotExist(statErr) {
					t.Fatalf("pkg.GenerateReport() did not create report file %s", tt.reportPath)
				}
				content, readErr := os.ReadFile(tt.reportPath)
				if readErr != nil {
					t.Fatalf("Failed to read report file %s: %v", tt.reportPath, readErr)
				}
				reportContent := string(content)
				for _, sub := range tt.expectedSubstrings {
					if !strings.Contains(reportContent, sub) {
						t.Errorf("pkg.GenerateReport() report content missing substring '%s'.\nFull report:\n%s", sub, reportContent)
					}
				}
			}
		})
	}
}
