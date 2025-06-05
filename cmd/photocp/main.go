package main

import (
	"flag"
	"fmt"
	"log"
	// "errors" // No longer directly used in main after refactor
	"os"
	"path/filepath"
	"strings" // Added for checking file extensions
	"time"    // time.Time is used for photoDate variable type and other time operations

	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder

	"github.com/user/photo-sorter/pkg"
)

// FileInfo holds path, resolution and hash for a file.
// This FileInfo is specific to main's processing logic before deciding to copy.
// The pkg.FileInfo might be different or used internally by pkg functions.
// For now, keeping this local, assuming it's distinct or will be reconciled
// if pkg.GenerateReport or other functions expect pkg.FileInfo.
type FileInfo struct {
	Path   string
	Width  int
	Height int
	// Fields like TargetDestPath, PhotoDate, DateSource will be populated before copying,
	// but are not stored in filesSelectedForCopy list initially.
}

// DuplicateInfo is defined in the pkg package (specifically in pkg/reporter.go).
// main.go will use pkg.DuplicateInfo.

func runApplicationLogic(sourceDir string, targetBaseDir string) (processedFiles int, copiedFiles int, filesToCopy int, duplicates []pkg.DuplicateInfo, pixelHashUnsupported int, err error) {
	reportFilePath := filepath.Join(targetBaseDir, "report.txt")
	fmt.Printf("Photo Sorter Initializing...\nSource: %s\nTarget: %s\nReport: %s\n", sourceDir, targetBaseDir, reportFilePath)

	// --- Initialize counters and tracking structures ---
	// processedFilesCounter, copiedFilesCounter, filesToCopyCount are return values
	// pixelHashUnsupportedCounter is also a return value
	filesSelectedForCopy := []FileInfo{}      // Holds FileInfo for files to be copied eventually.
	// duplicateReportEntries is also a return value

	// --- Scanning ---
	fmt.Printf("Scanning source directory: %s\n", sourceDir)
	imageFiles, scanErr := pkg.ScanSourceDirectory(sourceDir)
	if scanErr != nil {
		// Log non-fatal error from pkg.ScanSourceDirectory if it's just about unreadable files,
		// but Fatal if the directory itself is bad (already handled by Stat above mostly)
		log.Printf("Warning during scanning source directory '%s': %v. Attempting to continue with any found files.\n", sourceDir, scanErr)
		if imageFiles == nil { // If the error was critical and no files could be read
			// Return an error instead of Fatalf
			return 0, 0, 0, nil, 0, fmt.Errorf("critical error: No files could be read from source directory '%s'", sourceDir)
		}
	}

	if len(imageFiles) == 0 {
		fmt.Println("No image files found in source directory.")
		if genErr := pkg.GenerateReport(reportFilePath, duplicates, copiedFiles, processedFiles, 0, pixelHashUnsupported); genErr != nil {
			// Return an error instead of Fatalf
			return 0, 0, 0, duplicates, pixelHashUnsupported, fmt.Errorf("failed to generate final report: %w", genErr)
		}
		return 0, 0, 0, duplicates, pixelHashUnsupported, nil // No error, but no files processed
	}

	fmt.Printf("Found %d image file(s) to process.\n", len(imageFiles))
	processedFiles = len(imageFiles)

	// --- Main processing loop ---
	for _, currentSourceFilepath := range imageFiles {
		fmt.Printf("\nProcessing: %s\n", currentSourceFilepath)

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentSourceFilepath)
		if errRes != nil {
			// For non-image files or images where resolution cannot be determined,
			// log a warning but proceed with Width=0, Height=0.
			// These files will be processed using full file hash if they are not images,
			// or if they are images but this step failed.
			log.Printf("  - Warning: Could not get resolution for %s: %v. Proceeding with 0x0 resolution.\n", currentSourceFilepath, errRes)
			currentWidth = 0
			currentHeight = 0
		} else {
			fmt.Printf("  - Resolution: %dx%d\n", currentWidth, currentHeight)
		}

		currentSourceInfo := FileInfo{
			Path:   currentSourceFilepath,
			Width:  currentWidth,
			Height: currentHeight,
		}

		isDuplicateOfExisting := false
		shouldReplaceExistingSelected := false
		indexOfExistingToReplace := -1
		reasonForReplacement := ""

		for idx, existingSelectedInfo := range filesSelectedForCopy {
			fmt.Printf("  - Comparing with selected file: %s\n", existingSelectedInfo.Path)
			compResult, errComp := pkg.AreFilesPotentiallyDuplicate(currentSourceInfo.Path, existingSelectedInfo.Path)

			if errComp != nil {
				log.Printf("    - Error comparing %s and %s: %v. Treating as non-duplicate for this pair.\n", currentSourceInfo.Path, existingSelectedInfo.Path, errComp)
				continue
			}

			if compResult.HashType == pkg.HashTypeFile {
				ext := strings.ToLower(filepath.Ext(currentSourceInfo.Path))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" {
					// This file is an image type but was processed using full file hash.
					// This implies pixel hashing was not applicable or failed.
					pixelHashUnsupported++
				}
			}

			if compResult.AreDuplicates {
				isDuplicateOfExisting = true

				if compResult.Reason == pkg.ReasonPixelHashMatch {
					fmt.Printf("    - Pixel hash match with %s.\n", existingSelectedInfo.Path)
					if currentSourceInfo.Width*currentSourceInfo.Height > existingSelectedInfo.Width*existingSelectedInfo.Height {
						fmt.Printf("    - Current file %s (%dx%d) is better resolution than selected %s (%dx%d).\n",
							currentSourceInfo.Path, currentSourceInfo.Width, currentSourceInfo.Height,
							existingSelectedInfo.Path, existingSelectedInfo.Width, existingSelectedInfo.Height)

						if !shouldReplaceExistingSelected || (currentSourceInfo.Width*currentSourceInfo.Height > filesSelectedForCopy[indexOfExistingToReplace].Width*filesSelectedForCopy[indexOfExistingToReplace].Height) {
							shouldReplaceExistingSelected = true
							indexOfExistingToReplace = idx
							reasonForReplacement = fmt.Sprintf("%s (current %s has better resolution than %s)", compResult.Reason, currentSourceInfo.Path, existingSelectedInfo.Path)
						}
					} else {
						fmt.Printf("    - Existing selected file %s (%dx%d) is better or same resolution as current %s (%dx%d).\n",
							existingSelectedInfo.Path, existingSelectedInfo.Width, existingSelectedInfo.Height,
							currentSourceInfo.Path, currentSourceInfo.Width, currentSourceInfo.Height)
						duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: existingSelectedInfo.Path, DiscardedFile: currentSourceInfo.Path, Reason: compResult.Reason + " (existing kept - resolution)"})
						goto nextSourceFileIteration
					}
				} else {
					fmt.Printf("    - Duplicate (Reason: %s) with %s. Keeping existing selected file.\n", compResult.Reason, existingSelectedInfo.Path)
					duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: existingSelectedInfo.Path, DiscardedFile: currentSourceInfo.Path, Reason: compResult.Reason})
					goto nextSourceFileIteration
				}
			}
		}

		if shouldReplaceExistingSelected {
			discardedInfo := filesSelectedForCopy[indexOfExistingToReplace]
			duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: currentSourceInfo.Path, DiscardedFile: discardedInfo.Path, Reason: reasonForReplacement})
			filesSelectedForCopy[indexOfExistingToReplace] = currentSourceInfo
			fmt.Printf("  - Marked for replacement: %s will replace %s (Reason: %s)\n", currentSourceInfo.Path, discardedInfo.Path, reasonForReplacement)
		} else if !isDuplicateOfExisting {
			filesSelectedForCopy = append(filesSelectedForCopy, currentSourceInfo)
			fmt.Printf("  - Marked for copy: %s (unique so far)\n", currentSourceInfo.Path)
		}

		nextSourceFileIteration:
	}


	fmt.Printf("\n--- Starting Copying Phase ---\nFound %d files to copy.\n", len(filesSelectedForCopy))
	for _, finalInfoToCopy := range filesSelectedForCopy {
		fmt.Printf("Preparing to copy selected file: %s\n", finalInfoToCopy.Path)
		var photoDate time.Time
		var dateSource string

		exifDate, dateErr := pkg.GetPhotoCreationDate(finalInfoToCopy.Path)
		if dateErr == nil {
			photoDate = exifDate
			dateSource = "EXIF"
			fmt.Printf("  - Extracted date (%s) for %s: %s\n", dateSource, finalInfoToCopy.Path, photoDate.Format("2006-01-02"))
		} else {
			log.Printf("  - Warning: Could not get EXIF date for %s: %v. Using file modification time.\n", finalInfoToCopy.Path, dateErr)
			fileInfoStat, statErr := os.Stat(finalInfoToCopy.Path)
			if statErr != nil {
				log.Printf("    - Error getting file info for %s: %v. Skipping copy.\n", finalInfoToCopy.Path, statErr)
				continue
			}
			photoDate = fileInfoStat.ModTime()
			dateSource = "FileModTime"
			fmt.Printf("  - Using fallback date (%s) for %s: %s\n", dateSource, finalInfoToCopy.Path, photoDate.Format("2006-01-02-150405"))
		}

		targetMonthDir, dirErr := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if dirErr != nil {
			log.Printf("  - Error creating target directory for %s (date: %s, source: %s): %v. Skipping copy.\n",
				finalInfoToCopy.Path, photoDate.Format("2006-01-02-150405"), dateSource, dirErr)
			continue
		}

		originalExtension := filepath.Ext(finalInfoToCopy.Path)
		baseName := photoDate.In(time.UTC).Format("2006-01-02-150405")
		newFileName := fmt.Sprintf("%s%s", baseName, originalExtension)
		destPath := filepath.Join(targetMonthDir, newFileName)

		version := 1
		originalDestPathForLog := destPath
		for {
			if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
				if destPath != originalDestPathForLog {
					fmt.Printf("  - Path %s existed. Using versioned name: %s\n", originalDestPathForLog, destPath)
				}
				break
			} else if statErr != nil {
				log.Printf("  - Error stating file %s: %v. Skipping copy of %s.\n", destPath, statErr, finalInfoToCopy.Path)
				goto nextFileToCopyLoop
			}
			newFileName = fmt.Sprintf("%s-%d%s", baseName, version, originalExtension)
			destPath = filepath.Join(targetMonthDir, newFileName)
			version++
		}
		fmt.Printf("  - Preparing to copy %s to: %s\n", finalInfoToCopy.Path, destPath)

		if copyErr := pkg.CopyFile(finalInfoToCopy.Path, destPath); copyErr != nil {
			log.Printf("  - Error copying file %s to %s: %v.\n", finalInfoToCopy.Path, destPath, copyErr)
		} else {
			fmt.Printf("  - Successfully copied %s to %s\n", finalInfoToCopy.Path, destPath)
			copiedFiles++
		}
		nextFileToCopyLoop:
	}

	fmt.Println("\n--- Photo Sorting Process Completed ---")
	filesToCopy = len(filesSelectedForCopy)
	if genErr := pkg.GenerateReport(reportFilePath, duplicates, copiedFiles, processedFiles, filesToCopy, pixelHashUnsupported); genErr != nil {
		return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, fmt.Errorf("failed to generate final report: %w", genErr)
	}
	return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, nil
}

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
		fmt.Println("  This project relies on the following Go modules:")
		fmt.Println("\n  Direct Dependencies:")
		fmt.Println("  - goexif (github.com/rwcarlsen/goexif)")
		fmt.Println("    - Purpose: Used to extract EXIF data from image files.")
		fmt.Println("    - License: BSD 2-Clause \"Simplified\" License")
		fmt.Println("    - Copyright: Copyright (c) 2012, Robert Carlsen & Contributors")
		fmt.Println("\n  Indirect Dependencies:")
		fmt.Println("    These libraries are included by direct dependencies or the testing framework.")
		fmt.Println("  - go-spew (github.com/davecgh/go-spew)")
		fmt.Println("    - License: ISC License (Copyright (c) 2012-2016 Dave Collins <dave@davec.name>)")
		fmt.Println("  - go-difflib (github.com/pmezard/go-difflib)")
		fmt.Println("    - License: BSD 3-Clause License (Copyright (c) 2013, Patrick Mezard)")
		fmt.Println("  - testify (github.com/stretchr/testify)")
		fmt.Println("    - License: MIT License (Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors)")
		fmt.Println("  - yaml.v3 (gopkg.in/yaml.v3 - Source: github.com/go-yaml/yaml/tree/v3)")
		fmt.Println("    - License: MIT License (Copyright (c) 2006-2010 Kirill Simonov) and ")
		fmt.Println("               Apache License 2.0 (Copyright (c) 2011-2019 Canonical Ltd)")
		fmt.Println("\n  Please refer to the respective repositories for full license texts.")
		os.Exit(0)
	}

	sourceDir := *sourceDirFlag
	targetBaseDir := *targetDirFlag

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

	// Call the extracted application logic
	_, _, _, _, _, appErr := runApplicationLogic(sourceDir, targetBaseDir)
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
}
