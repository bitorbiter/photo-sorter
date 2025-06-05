package main

import (
	"flag"
	"fmt"
	"log"
	"errors"
	"os"
	"path/filepath"
	"time" // time.Time is used for photoDate variable type and other time operations

	"github.com/user/photo-sorter/pkg"
)

// FileInfo holds path, resolution and hash for a file.
// This FileInfo is specific to main's processing logic before deciding to copy.
// The pkg.FileInfo might be different or used internally by pkg functions.
// For now, keeping this local, assuming it's distinct or will be reconciled
// if pkg.GenerateReport or other functions expect pkg.FileInfo.
type FileInfo struct {
	Path      string
	Width     int
	Height    int
	PixelHash string // SHA-256 hash of pixel data
	FileHash  string // SHA-256 hash of full file content
	HashType  string // "pixel", "file", or "none"
}

// DuplicateInfo is defined in the pkg package (specifically in pkg/reporter.go).
// main.go will use pkg.DuplicateInfo.

func main() {
	// --- Command-line flags ---
	sourceDirFlag := flag.String("sourceDir", "", "Source directory containing photos to sort (required)")
	targetDirFlag := flag.String("targetDir", "", "Target directory to store sorted photos (required)")
	helpFlg := flag.Bool("help", false, "Show help message and license information")
	flag.Parse()

	if *helpFlg {
		fmt.Println("Usage: photocp -sourceDir <source_directory> -targetDir <target_directory>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults() // Prints all defined flags, including -help
		fmt.Println("\nLicense Information:")
		fmt.Println("  This application is licensed under the BSD 2-Clause License.")
		fmt.Println("  See the LICENSE file in the repository for the full license text.")
		fmt.Println("\nDependency Information:")
		fmt.Println("  This application uses the goexif library (https://github.com/rwcarlsen/goexif),")
		fmt.Println("  which is licensed under the BSD 2-Clause License.")
		fmt.Println("  Copyright (c) 2012, Robert Carlsen & Contributors.")
		os.Exit(0)
	}

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

	// Store FileInfo for files successfully pixel-hashed, keyed by their pixel hash
	pixelHashedCandidates := make(map[string]FileInfo)
	// Store FileInfo for files that fall back to full file hash, keyed by their file hash
	fileHashedCandidates := make(map[string]FileInfo)
	var duplicateReportEntries []pkg.DuplicateInfo // Uses pkg.DuplicateInfo
	pixelHashUnsupportedCounter := 0

	// --- Scanning ---
	fmt.Printf("Scanning source directory: %s\n", sourceDir)
	imageFiles, err := pkg.ScanSourceDirectory(sourceDir)
	if err != nil {
		// Log non-fatal error from pkg.ScanSourceDirectory if it's just about unreadable files,
		// but Fatal if the directory itself is bad (already handled by Stat above mostly)
		log.Printf("Warning during scanning source directory '%s': %v. Attempting to continue with any found files.\n", sourceDir, err)
		if imageFiles == nil { // If the error was critical and no files could be read
			log.Fatalf("Critical error: No files could be read from source directory '%s'. Exiting.", sourceDir)
		}
	}

	if len(imageFiles) == 0 {
		fmt.Println("No image files found in source directory.")
		// Assuming GenerateReport expects the local DuplicateInfo type for now.
		// This will be confirmed/fixed when we check pkg/reporter.go's GenerateReport signature.
		if genErr := pkg.GenerateReport(reportFilePath, duplicateReportEntries, copiedFilesCounter, processedFilesCounter, filesToCopyCount); genErr != nil {
			log.Fatalf("Failed to generate final report: %v", genErr)
		}
		return
	}

	fmt.Printf("Found %d image file(s) to process.\n", len(imageFiles))
	processedFilesCounter = len(imageFiles)

	// --- Main processing loop ---
	for _, currentFilePath := range imageFiles {
		fmt.Printf("\nProcessing: %s\n", currentFilePath)
		processThisFile := false // Flag to determine if the current file should be copied

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentFilePath)
		if errRes != nil {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Resolution comparison may be skipped or impacted.\n", currentFilePath, errRes)
			currentWidth, currentHeight = 0, 0 // Ensure zero values if resolution failed
		} else {
			fmt.Printf("  - Resolution: %dx%d\n", currentWidth, currentHeight)
		}

		currentFileInfo := FileInfo{
			Path:   currentFilePath,
			Width:  currentWidth,
			Height: currentHeight,
		}

		// Attempt Pixel Hashing
		pixelHash, errPixel := pkg.CalculatePixelDataHash(currentFilePath)
		logPixelHashAttemptNeeded := true // Used to control logging for fallback messages

		if errPixel == nil {
			currentFileInfo.PixelHash = pixelHash
			currentFileInfo.HashType = "pixel"
			fmt.Printf("  - Pixel Hash: %s\n", pixelHash)
			logPixelHashAttemptNeeded = false

			if existingPixelFileInfo, found := pixelHashedCandidates[pixelHash]; found {
				fmt.Printf("  - Pixel hash collision: Potential duplicate of %s\n", existingPixelFileInfo.Path)
				reason := fmt.Sprintf("Pixel hash match with %s", existingPixelFileInfo.Path)
				keptFileInfo := existingPixelFileInfo
				discardedFileInfo := currentFileInfo

				// Resolution comparison logic for pixel hash duplicates
				canCompareResolutions := errRes == nil && existingPixelFileInfo.Width > 0 && existingPixelFileInfo.Height > 0 && currentFileInfo.Width > 0 && currentFileInfo.Height > 0

				if canCompareResolutions {
					currentPixels := currentFileInfo.Width * currentFileInfo.Height
					existingPixels := existingPixelFileInfo.Width * existingPixelFileInfo.Height

					if (float64(currentPixels) > float64(existingPixels)*1.1) || (existingPixels == 0 && currentPixels > 0) {
						log.Printf("    - Resolution Check (Pixel Hash): Current file %s (%dx%d) is better than existing %s (%dx%d).\n",
							currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height,
							existingPixelFileInfo.Path, existingPixelFileInfo.Width, existingPixelFileInfo.Height)
						reason = fmt.Sprintf("Pixel hash match, current file %s (%dx%d) has higher resolution than %s (%dx%d)",
							currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height, existingPixelFileInfo.Path, existingPixelFileInfo.Width, existingPixelFileInfo.Height)
						keptFileInfo = currentFileInfo
						discardedFileInfo = existingPixelFileInfo
						pixelHashedCandidates[pixelHash] = currentFileInfo
						processThisFile = true
						fmt.Printf("  - Replacing previous pixel candidate %s with %s due to better resolution.\n", discardedFileInfo.Path, keptFileInfo.Path)
					} else {
						log.Printf("    - Resolution Check (Pixel Hash): Current file %s (%dx%d) is NOT significantly better. Existing version kept: %s.\n",
							currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height, existingPixelFileInfo.Path)
						processThisFile = false
						reason = fmt.Sprintf("Pixel hash match, existing file %s (%dx%d) kept due to similar or better resolution than %s (%dx%d)",
							existingPixelFileInfo.Path, existingPixelFileInfo.Width, existingPixelFileInfo.Height, currentFileInfo.Path, currentFileInfo.Width, currentFileInfo.Height)
					}
				} else {
					log.Printf("    - Resolution Check (Pixel Hash): Could not reliably compare resolutions. Existing version kept: %s.\n", existingPixelFileInfo.Path)
					processThisFile = false // Keep the first one encountered if resolutions can't be compared
					reason = fmt.Sprintf("Pixel hash match with %s, resolution comparison skipped or failed, kept existing.", existingPixelFileInfo.Path)
				}
				duplicateReportEntries = append(duplicateReportEntries, pkg.DuplicateInfo{
					KeptFile:      keptFileInfo.Path,
					DiscardedFile: discardedFileInfo.Path,
					Reason:        reason,
				})
				if !processThisFile {
					fmt.Printf("  - Skipping pixel duplicate: %s (Kept: %s)\n", currentFilePath, keptFileInfo.Path)
					// continue // This continue was causing an issue; logic flow handles it.
				}
			} else {
				pixelHashedCandidates[pixelHash] = currentFileInfo
				processThisFile = true
				fmt.Printf("  - Unique pixel content. Marked for processing.\n")
			}
		} else {
			// Pixel hash failed, decide action based on error type
			if errors.Is(errPixel, pkg.ErrUnsupportedForPixelHashing) {
				if logPixelHashAttemptNeeded {
					log.Printf("  - Info: Pixel data hashing not supported for %s: %v. Falling back to full file hash.\n", currentFilePath, errPixel)
				}
				pixelHashUnsupportedCounter++
				currentFileInfo.HashType = "file"

				// Attempt Full File Hashing (Fallback)
				fileHash, errFile := pkg.CalculateFileHash(currentFilePath)
				if errFile == nil {
					currentFileInfo.FileHash = fileHash
					fmt.Printf("  - File Hash (fallback): %s\n", fileHash)

					if existingFileHashInfo, found := fileHashedCandidates[fileHash]; found {
						fmt.Printf("  - File hash collision (fallback): Duplicate of %s\n", existingFileHashInfo.Path)
						processThisFile = false // Keep first encountered for file hash duplicates
						reason := fmt.Sprintf("File hash match (pixel hashing not supported) with %s. Kept existing.", existingFileHashInfo.Path)
						duplicateReportEntries = append(duplicateReportEntries, pkg.DuplicateInfo{
							KeptFile:      existingFileHashInfo.Path,
							DiscardedFile: currentFileInfo.Path,
							Reason:        reason,
						})
						fmt.Printf("  - Skipping file hash duplicate (fallback): %s (Kept: %s)\n", currentFilePath, existingFileHashInfo.Path)
					} else {
						fileHashedCandidates[fileHash] = currentFileInfo
						processThisFile = true
						fmt.Printf("  - Unique file content (fallback). Marked for processing.\n")
					}
				} else {
					log.Printf("  - Error calculating file hash for %s: %v. Skipping.\n", currentFilePath, errFile)
					currentFileInfo.HashType = "none"
					processThisFile = false
				}
			} else {
				// Other error from pixel hashing (e.g., file read error during pixel hashing)
				log.Printf("  - Error calculating pixel hash for %s: %v. Skipping.\n", currentFilePath, errPixel)
				currentFileInfo.HashType = "none"
				processThisFile = false
			}
		}

		// If, after all hashing and duplicate checks, the file is still marked for processing
		if !processThisFile { // If decided not to process (e.g. it's a duplicate and not better)
			fmt.Printf("  - Final decision: Skipping %s\n", currentFilePath)
			continue // Move to the next file
		}
		// Ensure that if processThisFile is true, we actually proceed.
		// The if processThisFile { filesToCopyCount++ ... } block should follow directly.

		// Now, if processThisFile is true, proceed with copying.
		if processThisFile {
			filesToCopyCount++

			var photoDate time.Time
			var dateSource string

			exifDate, err := pkg.GetPhotoCreationDate(currentFilePath)
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

			targetDayDir, err := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
			if err != nil {
				log.Printf("  - Error creating target directory for %s (date: %s, source: %s): %v. Skipping copy.\n",
					currentFilePath, photoDate.Format("2006-01-02"), dateSource, err)
				filesToCopyCount--
				continue
			}

			originalExtension := filepath.Ext(currentFilePath)
			dateStr := photoDate.Format("2006-01-02")
			newFileName := fmt.Sprintf("image-%s%s", dateStr, originalExtension)
			destPath := filepath.Join(targetDayDir, newFileName)
			fmt.Printf("  - Preparing to copy to: %s\n", destPath)

			if err := pkg.CopyFile(currentFilePath, destPath); err != nil {
				log.Printf("  - Error copying file %s to %s: %v.\n", currentFilePath, destPath, err)
				filesToCopyCount--
			} else {
				fmt.Printf("  - Successfully copied %s to %s\n", currentFilePath, destPath)
				copiedFilesCounter++
			}
		}
	}

	fmt.Println("\n--- Photo Sorting Process Completed ---")
	// Assuming GenerateReport expects the local DuplicateInfo type.
	if err := pkg.GenerateReport(reportFilePath, duplicateReportEntries, copiedFilesCounter, processedFilesCounter, filesToCopyCount, pixelHashUnsupportedCounter); err != nil {
		log.Fatalf("Failed to generate final report: %v", err)
	}
}
