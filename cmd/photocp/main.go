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
// type FileInfo struct { // This local FileInfo might be removed or heavily adapted
// 	Path   string
// 	Width  int
// 	Height int
// }

// DuplicateInfo is defined in the pkg package (specifically in pkg/reporter.go).
// main.go will use pkg.DuplicateInfo.

func runApplicationLogic(sourceDir string, targetBaseDir string) (processedFiles int, copiedFiles int, filesToCopy int, duplicates []pkg.DuplicateInfo, pixelHashUnsupported int, err error) {
	reportFilePath := filepath.Join(targetBaseDir, "report.txt")
	fmt.Printf("Photo Sorter Initializing...\nSource: %s\nTarget: %s\nReport: %s\n", sourceDir, targetBaseDir, reportFilePath)

	// --- Initialize counters and tracking structures ---
	// processedFiles, copiedFiles are return values
	// pixelHashUnsupported is also a return value
	// filesSelectedForCopy is removed. Copy decisions are made per file.
	// duplicates list is still used.
	sourceFilesThatUsedFileHash := make(map[string]bool) // For accurate pixelHashUnsupported count
	keptFileSourceToTargetMap := make(map[string]string) // Tracks source path to its final target path for report updates

	// --- Target Directory Validation ---
	// Ensure targetBaseDir exists or can be created.
	// This is crucial before any processing, especially before FindPotentialTargetConflicts.
	if _, err := os.Stat(targetBaseDir); os.IsNotExist(err) {
		fmt.Printf("Target directory %s does not exist, attempting to create it.\n", targetBaseDir)
		if err := os.MkdirAll(targetBaseDir, 0755); err != nil {
			return 0, 0, 0, nil, 0, fmt.Errorf("failed to create target base directory '%s': %w", targetBaseDir, err)
		}
	} else if err != nil {
		return 0, 0, 0, nil, 0, fmt.Errorf("error accessing target base directory '%s': %w", targetBaseDir, err)
	}


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

		// 1.a Determine photoDate and dateSource
		var photoDate time.Time
		var dateSource string
		exifDate, dateErr := pkg.GetPhotoCreationDate(currentSourceFilepath)
		if dateErr == nil {
			photoDate = exifDate
			dateSource = "EXIF"
		} else {
			fileInfoStat, statErr := os.Stat(currentSourceFilepath)
			if statErr != nil {
				log.Printf("  - Error getting file info for %s: %v. Skipping this file.\n", currentSourceFilepath, statErr)
				continue // Skip this source file
			}
			photoDate = fileInfoStat.ModTime()
			dateSource = "FileModTime"
		}
		fmt.Printf("  - Determined date (%s) for %s: %s\n", dateSource, currentSourceFilepath, photoDate.Format("2006-01-02 15:04:05"))

		// 1.b Create target directory path based on date
		targetMonthDir, err := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if err != nil {
			log.Printf("  - Error creating/accessing target month directory for %s (date: %s): %v. Skipping.\n", currentSourceFilepath, photoDate, err)
			continue
		}

		// 1.c Determine exact target filename and path
		originalExtension := filepath.Ext(currentSourceFilepath)
		baseNameWithoutExt := photoDate.In(time.UTC).Format("2006-01-02-150405")
		targetFileName := baseNameWithoutExt + originalExtension
		exactTargetPath := filepath.Join(targetMonthDir, targetFileName)

		fmt.Printf("  - Proposed target path: %s\n", exactTargetPath)

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentSourceFilepath)
		if errRes != nil {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Proceeding with 0x0 resolution.\n", currentSourceFilepath, errRes)
			currentWidth = 0
			currentHeight = 0
		} else {
			fmt.Printf("  - Source resolution: %dx%d\n", currentWidth, currentHeight)
		}
		// currentSourceInfo := FileInfo{Path: currentSourceFilepath, Width: currentWidth, Height: currentHeight} // FileInfo struct might be removed

		// 2. Check if a file already exists at the exact target path.
		targetExists := false
		_, statErr := os.Stat(exactTargetPath)
		if statErr == nil {
			targetExists = true
			fmt.Printf("  - File already exists at target path: %s\n", exactTargetPath)
		} else if !os.IsNotExist(statErr) {
			log.Printf("  - Error checking target path %s: %v. Skipping source file %s.\n", exactTargetPath, statErr, currentSourceFilepath)
			continue // Skip this source file due to error accessing target
		}

		if !targetExists {
			// No conflict, copy directly
			fmt.Printf("  - Target path %s is empty. Copying %s directly.\n", exactTargetPath, currentSourceFilepath)
			if copyErr := pkg.CopyFile(currentSourceFilepath, exactTargetPath); copyErr != nil {
				log.Printf("  - Error copying file %s to %s: %v.\n", currentSourceFilepath, exactTargetPath, copyErr)
			} else {
				fmt.Printf("  - Successfully copied %s to %s\n", currentSourceFilepath, exactTargetPath)
				copiedFiles++
				keptFileSourceToTargetMap[currentSourceFilepath] = exactTargetPath // Track for report
			}
			continue // nextSourceFileIteration (implicitly)
		} else {
			// Conflict: File exists at exactTargetPath. Compare source with this specific target file.
			fmt.Printf("    - Comparing source %s with existing target %s\n", currentSourceFilepath, exactTargetPath)
			compResult, errComp := pkg.AreFilesPotentiallyDuplicate(currentSourceFilepath, exactTargetPath)

			if errComp != nil {
				log.Printf("      - Error comparing source %s with target %s: %v. Assuming target is kept.\n", currentSourceFilepath, exactTargetPath, errComp)
				// Record source as discarded, target as kept.
				duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: "Comparison error, existing target kept"})
				if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath) { // compResult might be partially populated
					sourceFilesThatUsedFileHash[currentSourceFilepath] = true
				}
				continue // nextSourceFileIteration
			}

			if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath) {
				sourceFilesThatUsedFileHash[currentSourceFilepath] = true
			}

			if compResult.AreDuplicates {
				fmt.Printf("      - Duplicate found: Source %s and Target %s. Reason: %s\n", currentSourceFilepath, exactTargetPath, compResult.Reason)
				targetResolutionBetterOrEqual := true // Assume target is better unless source proves otherwise

				if compResult.Reason == pkg.ReasonPixelHashMatch {
					targetWidth, targetHeight, errResTarget := pkg.GetImageResolution(exactTargetPath)
					if errResTarget != nil {
						log.Printf("      - Warning: Could not get resolution for target %s: %v. Source might replace if it has resolution.\n", exactTargetPath, errResTarget)
						if currentWidth*currentHeight > 0 { // Source has resolution, target doesn't (error)
							targetResolutionBetterOrEqual = false
						} else {
							// Neither has resolution or source also has error, keep target.
							duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + " (existing target kept - resolution error for target, source has no resolution or also error)"})
							fmt.Printf("      - Target %s kept (pixel hash match, resolution error for target and source has no resolution).\n", exactTargetPath)
							continue // nextSourceFileIteration
						}
					} else { // Target resolution is known
						fmt.Printf("      - Target resolution: %dx%d\n", targetWidth, targetHeight)
						if currentWidth*currentHeight > targetWidth*targetHeight {
							targetResolutionBetterOrEqual = false
						}
					}
				} // For other duplicate reasons (e.g. file hash), target is considered "better or equal" by default.

				if !targetResolutionBetterOrEqual { // Source is better
					fmt.Printf("      - Source %s (%dx%d) is better than target %s. Replacing target.\n", currentSourceFilepath, currentWidth, currentHeight, exactTargetPath)
					duplicates = append(duplicates, pkg.DuplicateInfo{
						KeptFile:      currentSourceFilepath, // Will be updated to exactTargetPath by the end if copy succeeds
						DiscardedFile: exactTargetPath,
						Reason:        compResult.Reason + " (source is better resolution)",
					})
					// Overwrite the target file
					if copyErr := pkg.CopyFile(currentSourceFilepath, exactTargetPath); copyErr != nil {
						log.Printf("      - Error overwriting target file %s with source %s: %v. Original target remains.\n", exactTargetPath, currentSourceFilepath, copyErr)
						// Need to revert the duplicate entry if copy fails, or adjust KeptFile
						// For simplicity, if overwrite fails, we assume the original target was "kept" and source "discarded"
						// Remove the last duplicate entry and add a new one.
						if len(duplicates) > 0 { duplicates = duplicates[:len(duplicates)-1] }
						duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: "Attempted replacement failed, original target kept"})
					} else {
						fmt.Printf("      - Successfully overwrote %s with %s\n", exactTargetPath, currentSourceFilepath)
						copiedFiles++ // Counts as a copy operation
						keptFileSourceToTargetMap[currentSourceFilepath] = exactTargetPath // Track for report
					}
				} else { // Target is better or same resolution, or not a pixel hash match (e.g. file hash match)
					reasonSuffix := ""
					if compResult.Reason == pkg.ReasonPixelHashMatch {
						reasonSuffix = " (existing target kept - resolution)"
					} else {
						reasonSuffix = " (existing target kept)"
					}
					duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + reasonSuffix})
					fmt.Printf("      - Target %s kept (source %s discarded). Reason: %s\n", exactTargetPath, currentSourceFilepath, compResult.Reason + reasonSuffix)
				}
			} else { // Not duplicates
				// This case should ideally not happen if target path naming is strictly by date-time.
				// If AreFilesPotentiallyDuplicate says they are not duplicates, it means their content differs significantly
				// despite having the same name (which implies same date-time to the second).
				// This would be an edge case, perhaps different files modified at the exact same second.
				// The requirement is to copy to `targetBaseDir/YYYY/MM/YYYY-MM-DD-HHMMSS.ext`.
				// If a file exists there, it's a conflict. If `AreFilesPotentiallyDuplicate` says they are different,
				// this implies a hash mismatch. The current logic implies we'd overwrite if source is better,
				// or discard source if target is better.
				// However, the problem implies exact path collision means they *should* be duplicates or one replacing other.
				// Let's assume if `AreFilesPotentiallyDuplicate` returns false, we treat source as "different but not better" and discard it to avoid data loss of original target.
				// Or, this implies versioning is needed, which is outside current scope of "exact target path".
				// For now, if they are not duplicates, we log it and discard the source to protect the existing target.
				log.Printf("      - Source %s and target %s are deemed different by content comparison, but share the same target path. Discarding source to protect existing target.\n", currentSourceFilepath, exactTargetPath)
				duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: "Content different, but name collision; existing target preserved"})
			}
			continue // nextSourceFileIteration
		}
		// Removed the `nextSourceFileIteration:` label as it's no longer needed here.
		// The loop will naturally continue.
	}

	pixelHashUnsupported = len(sourceFilesThatUsedFileHash)

	// The separate copying phase is removed. Copying is done within the main loop.
	// fmt.Printf("\n--- Starting Copying Phase ---\nFound %d files to copy.\n", len(filesSelectedForCopy)) // filesSelectedForCopy removed
	// keptFileSourceToTargetMap := make(map[string]string) // Moved to top of function

	// Loop for copying ( `for _, finalInfoToCopy := range filesSelectedForCopy` ) is removed.

	// Update KeptFile paths in duplicates report
	for i, dup := range duplicates {
		// If KeptFile was a source path that got copied, update it to its new target path
		if targetPath, ok := keptFileSourceToTargetMap[dup.KeptFile]; ok {
			duplicates[i].KeptFile = targetPath
		}
		// If KeptFile was already a target path (e.g. "existing target kept"), it remains as is.
		// If KeptFile was a source path that got replaced by another source path (which was then copied),
		// its entry in keptFileSourceToTargetMap would point to the new target path of the replacement.
		// This logic also handles cases where KeptFile was a placeholder like potentialTargetMonthDir + newBaseTargetFileName
		// It will be updated if currentSourceFilepath (which matches the placeholder's source intention) is in keptFileSourceToTargetMap.
		// Example: duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: filepath.Join(potentialTargetMonthDir, baseNameWithoutExt+originalExtension), ...})
		// Here, dup.KeptFile is a constructed path. We need to map it based on the source file that *would* have become this.
		// This part is tricky if the KeptFile in the report is a *predicted* target path.
		// The current code adds source paths to KeptFile for "source replaces selected" or "source replaces target"
		// So, this map lookup should work for those cases.

		// A specific check for placeholders used for "source is better than target"
		// The KeptFile was set to something like: filepath.Join(potentialTargetMonthDir, baseNameWithoutExt+originalExtension)
		// We need to find which source file this corresponds to.
		// This requires matching `DiscardedFile` (which is a target path) back to a source file decision.
		// This is becoming complex. Simpler: if KeptFile is not an absolute path already from target,
		// and it matches a source path that was copied, update it.
		// The placeholder `filepath.Join(potentialTargetMonthDir, baseNameWithoutExt+originalExtension)` needs robust handling.
		// Let's refine the duplicate entry logic: KeptFile should always be the *source path* if the source is kept.
		// Then this loop correctly updates it to the *final target path*.

		// If a duplicate entry has KeptFile as a placeholder like "targetDir/YYYY/MM/file.ext"
		// and this placeholder corresponds to a `currentSourceFilepath` that was eventually copied,
		// we need to update it.
		// This is implicitly handled if the `KeptFile` was set to `currentSourceFilepath` during the duplicate decision phase
		// for "source is better" scenarios.
		// Let's re-check the logic where `duplicates` are added for `sourceFileIsBetterDuplicate`.
		// `duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: currentSourceInfo.Path, ...})` is better.
		// The current code uses: `KeptFile: filepath.Join(potentialTargetMonthDir, baseNameWithoutExt+originalExtension)`
		// This needs to be `currentSourceInfo.Path` for the map to work.
		// Assuming this change is made when adding to `duplicates` for "source better than target".
	}


	fmt.Println("\n--- Photo Sorting Process Completed ---")
	// filesToCopy is now the same as copiedFiles as decisions are immediate.
	filesToCopy = copiedFiles
	if genErr := pkg.GenerateReport(reportFilePath, duplicates, copiedFiles, processedFiles, filesToCopy, pixelHashUnsupported); genErr != nil {
		return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, fmt.Errorf("failed to generate final report: %w", genErr)
	}
	return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, nil
}

// This is the main application entry point.
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
	processed, copied, _, duplicates, pixelHashUnsupported, appErr := runApplicationLogic(sourceDir, targetBaseDir) // filesToCopy is now internal to runApplicationLogic or same as copied
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
	// The third returned value from runApplicationLogic (old filesToCopy) is now the same as copied.
	// So, for the log, we can use 'copied' for "Selected To Copy" or just report "Copied".
	// Let's adjust the log message to reflect that "Selected To Copy" isn't a separate concept anymore.
	log.Printf("Run Summary: Processed: %d, Copied: %d, Duplicates Found: %d, Pixel Hash Unsupported (Unique Files): %d\n",
		processed, copied, len(duplicates), pixelHashUnsupported)
}
