package pkg

import (
	"fmt"
	"os"
	"path/filepath"
)

// DuplicateInfo holds information about a pair of duplicate files.
type DuplicateInfo struct {
	KeptFile      string
	DiscardedFile string
	Reason        string // e.g., "Lower resolution", "Identical to already copied file"
}

// GenerateReport creates a text report summarizing the sorting process.
func GenerateReport(reportPath string, duplicates []DuplicateInfo, copiedFilesCount int, processedFilesCount int, filesToCopyCount int, pixelHashUnsupportedCount int) error {
	// Ensure the directory for the report exists
	reportDir := filepath.Dir(reportPath)
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for report '%s': %w", reportDir, err)
	}

	file, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("failed to create report file '%s': %w", reportPath, err)
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "Photo Sorting Report\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "====================\n\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "Summary:\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "  - Total files scanned: %d\n", processedFilesCount)
	if err != nil {
		return err
	}
	// Files identified for copying is removed as it's redundant with Files successfully copied.
	_, err = fmt.Fprintf(file, "  - Files successfully copied: %d\n", copiedFilesCount)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "  - Duplicate files found and discarded/skipped: %d\n", len(duplicates))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file, "  - Image files where pixel hashing was not supported (fallback to file hash): %d\n", pixelHashUnsupportedCount)
	if err != nil {
		return err
	}

	if len(duplicates) > 0 {
		_, err = fmt.Fprintf(file, "\nDuplicate Details:\n")
		if err != nil {
			return err
		}
		for _, d := range duplicates {
			_, err = fmt.Fprintf(file, "  - Kept: %s\n", d.KeptFile)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(file, "    Discarded: %s\n", d.DiscardedFile)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(file, "    Reason: %s\n\n", d.Reason)
			if err != nil {
				return err
			}
		}
	}

	fmt.Printf("Report generated at %s\n", reportPath)
	return nil
}
