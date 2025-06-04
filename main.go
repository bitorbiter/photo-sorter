package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time" // time.Time is used for photoDate variable type and other time operations
)

// FileInfo holds path, resolution and hash for a file.
type FileInfo struct {
	Path   string
	Width  int
	Height int
	Hash   string
}

func main() {
	// --- Command-line flags ---
	sourceDirFlag := flag.String("sourceDir", "", "Source directory containing photos to sort (required)")
	targetDirFlag := flag.String("targetDir", "", "Target directory to store sorted photos (required)")
	flag.Parse()

	sourceDir := *sourceDirFlag
	targetBaseDir := *targetDirFlag
	reportFilePath := filepath.Join(targetBaseDir, "report.txt")

	// --- Validate Flags ---
	if sourceDir == "" {
		log.Fatal("Error: -sourceDir flag is required.")
	}
	if targetBaseDir == "" {
		log.Fatal("Error: -targetDir flag is required.")
	}

	sourceInfo, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("Error: Source directory '%s' does not exist.", sourceDir)
		}
		log.Fatalf("Error: Could not stat source directory '%s': %v", sourceDir, err)
	}
	if !sourceInfo.IsDir() {
		log.Fatalf("Error: Source path '%s' is not a directory.", sourceDir)
	}

	fmt.Printf("Photo Sorter Initializing...\nSource: %s\nTarget: %s\nReport: %s\n", sourceDir, targetBaseDir, reportFilePath)

	// --- Initialize counters and tracking structures ---
	processedFilesCounter := 0
	copiedFilesCounter := 0
	var filesToCopyCount int // Number of files deemed unique or better resolution

	// Store FileInfo of files that are candidates for copying (keyed by hash)
	candidateFilesByHash := make(map[string]FileInfo)
	var duplicateReportEntries []DuplicateInfo

	// --- Scanning ---
	fmt.Printf("Scanning source directory: %s\n", sourceDir)
	imageFiles, err := ScanSourceDirectory(sourceDir)
	if err != nil {
		// Log non-fatal error from ScanSourceDirectory if it's just about unreadable files,
		// but Fatal if the directory itself is bad (already handled by Stat above mostly)
		log.Printf("Warning during scanning source directory '%s': %v. Attempting to continue with any found files.\n", sourceDir, err)
		if imageFiles == nil { // If the error was critical and no files could be read
			log.Fatalf("Critical error: No files could be read from source directory '%s'. Exiting.", sourceDir)
		}
	}

	if len(imageFiles) == 0 {
		fmt.Println("No image files found in source directory.")
		if genErr := GenerateReport(reportFilePath, duplicateReportEntries, copiedFilesCounter, processedFilesCounter, filesToCopyCount); genErr != nil {
			log.Fatalf("Failed to generate final report: %v", genErr)
		}
		return
	}

	fmt.Printf("Found %d image file(s) to process.\n", len(imageFiles))
	processedFilesCounter = len(imageFiles)

	// --- Main processing loop ---
	for _, currentFilePath := range imageFiles {
		fmt.Printf("\nProcessing: %s\n", currentFilePath)

		currentFileHash, err := CalculateFileHash(currentFilePath)
		if err != nil {
			log.Printf("  - Error calculating hash for %s: %v. Skipping.\n", currentFilePath, err)
			continue
		}
		fmt.Printf("  - Hash: %s\n", currentFileHash)

		currentWidth, currentHeight, errRes := GetImageResolution(currentFilePath)
		if errRes != nil {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Resolution comparison will be skipped.\n", currentFilePath, errRes)
			currentWidth, currentHeight = 0, 0 // Ensure zero values if resolution failed
		} else {
			fmt.Printf("  - Resolution: %dx%d\n", currentWidth, currentHeight)
		}

		currentFileInfo := FileInfo{
			Path:   currentFilePath,
			Width:  currentWidth,
			Height: currentHeight,
			Hash:   currentFileHash,
		}

		processThisFile := true // Flag to determine if the current file should be copied

		if existingFileInfo, found := candidateFilesByHash[currentFileHash]; found {
			fmt.Printf("  - Hash collision: Potential duplicate of %s\n", existingFileInfo.Path)

			reason := fmt.Sprintf("Duplicate content (hash match with %s)", existingFileInfo.Path)
			keptFileInfo := existingFileInfo
			discardedFileInfo := currentFileInfo

			canCompareResolutions := errRes == nil && existingFileInfo.Width > 0 && existingFileInfo.Height > 0 && currentFileInfo.Width > 0 && currentFileInfo.Height > 0

			if canCompareResolutions {
				currentPixels := currentFileInfo.Width * currentFileInfo.Height
				existingPixels := existingFileInfo.Width * existingFileInfo.Height

				if float64(currentPixels) > float64(existingPixels)*1.1 {
					log.Printf("    - Resolution Check: Current file %s (%dx%d) is significantly LARGER than existing %s (%dx%d).\n",
						currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height,
						existingFileInfo.Path, existingFileInfo.Width, existingFileInfo.Height)
					reason = fmt.Sprintf("Higher resolution than previously kept file %s (%dx%d vs %dx%d)",
										existingFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height, existingFileInfo.Width, existingFileInfo.Height)
					keptFileInfo = currentFileInfo
					discardedFileInfo = existingFileInfo
					candidateFilesByHash[currentFileHash] = currentFileInfo
					processThisFile = true
					fmt.Printf("  - Replacing previous candidate %s with %s due to better resolution.\n", discardedFileInfo.Path, keptFileInfo.Path)
				} else {
					log.Printf("    - Resolution Check: Current file %s (%dx%d) is NOT significantly larger. Existing version kept: %s.\n",
						currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height, existingFileInfo.Path)
					processThisFile = false
				}
			} else {
				log.Printf("    - Resolution Check: Could not reliably compare resolutions (one or both failed or were zero). Existing version kept: %s.\n", existingFileInfo.Path)
				processThisFile = false
			}

			duplicateReportEntries = append(duplicateReportEntries, DuplicateInfo{
				KeptFile:      keptFileInfo.Path,
				DiscardedFile: discardedFileInfo.Path,
				Reason:        reason,
			})

			if !processThisFile {
				fmt.Printf("  - Skipping duplicate: %s (Kept: %s)\n", currentFilePath, keptFileInfo.Path) // Use keptFileInfo here
				continue
			}
		} else {
			candidateFilesByHash[currentFileHash] = currentFileInfo
			processThisFile = true
			fmt.Printf("  - Unique content (new hash). Marked for processing.\n")
		}

		if processThisFile {
			filesToCopyCount++

			var photoDate time.Time
			var dateSource string

			exifDate, err := GetPhotoCreationDate(currentFilePath)
			if err == nil {
				photoDate = exifDate
				dateSource = "EXIF"
				fmt.Printf("  - Extracted date (%s): %s\n", dateSource, photoDate.Format("2006-01-02"))
			} else {
				log.Printf("  - Warning: Could not get EXIF date for %s: %v. Using file modification time.\n", currentFilePath, err)
				fileInfoStat, statErr := os.Stat(currentFilePath)
				if statErr != nil {
					log.Printf("    - Error getting file info for %s: %v. Skipping copy.\n", currentFilePath, statErr)
					filesToCopyCount--
					continue
				}
				photoDate = fileInfoStat.ModTime()
				dateSource = "FileModTime"
				fmt.Printf("  - Using fallback date (%s): %s\n", dateSource, photoDate.Format("2006-01-02"))
			}

			targetDayDir, err := CreateTargetDirectory(targetBaseDir, photoDate)
			if err != nil {
				log.Printf("  - Error creating target directory for %s (date: %s, source: %s): %v. Skipping copy.\n",
					currentFilePath, photoDate.Format("2006-01-02"), dateSource, err)
				filesToCopyCount--
				continue
			}

			destPath := filepath.Join(targetDayDir, filepath.Base(currentFilePath))
			fmt.Printf("  - Preparing to copy to: %s\n", destPath)

			if err := CopyFile(currentFilePath, destPath); err != nil {
				log.Printf("  - Error copying file %s to %s: %v.\n", currentFilePath, destPath, err)
				filesToCopyCount--
			} else {
				fmt.Printf("  - Successfully copied %s to %s\n", currentFilePath, destPath)
				copiedFilesCounter++
			}
		}
	}

	fmt.Println("\n--- Photo Sorting Process Completed ---")
	if err := GenerateReport(reportFilePath, duplicateReportEntries, copiedFilesCounter, processedFilesCounter, filesToCopyCount); err != nil {
		log.Fatalf("Failed to generate final report: %v", err)
	}
}
