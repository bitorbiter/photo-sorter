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
	sourceFilesThatUsedFileHash := make(map[string]bool) // For accurate pixelHashUnsupported count

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
			// Fallback to file modification time
			fileInfoStat, statErr := os.Stat(currentSourceFilepath)
			if statErr != nil {
				log.Printf("  - Error getting file info for %s: %v. Skipping this file.\n", currentSourceFilepath, statErr)
				continue // Skip this source file
			}
			photoDate = fileInfoStat.ModTime()
			dateSource = "FileModTime"
		}
		fmt.Printf("  - Determined date (%s): %s\n", dateSource, photoDate.Format("2006-01-02 15:04:05"))

		// 1.b Determine potentialTargetMonthDir
		potentialTargetMonthDir, err := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if err != nil {
			log.Printf("  - Error creating/accessing target month directory for %s (date: %s): %v. Skipping.\n", currentSourceFilepath, photoDate, err)
			continue
		}

		// 1.c Determine base target filename
		originalExtension := filepath.Ext(currentSourceFilepath)
		baseNameWithoutExt := photoDate.In(time.UTC).Format("2006-01-02-150405")
		// newBaseTargetFileName := baseNameWithoutExt + originalExtension // Used later for report


		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentSourceFilepath)
		if errRes != nil {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Proceeding with 0x0 resolution.\n", currentSourceFilepath, errRes)
			currentWidth = 0
			currentHeight = 0
		} else {
			fmt.Printf("  - Resolution: %dx%d\n", currentWidth, currentHeight)
		}
		currentSourceInfo := FileInfo{Path: currentSourceFilepath, Width: currentWidth, Height: currentHeight}


		// 2. Check Against Existing Target Files
		isDuplicateOfTargetFile := false
		sourceFileIsBetterDuplicate := false
		// targetFileToSupersede := "" // Path of the target file that would be superseded.

		// 2.a Scan potentialTargetMonthDir for conflicts
		fmt.Printf("  - Checking for existing files in target: %s (base: %s, ext: %s)\n", potentialTargetMonthDir, baseNameWithoutExt, originalExtension)
		existingTargetCandidates, errFind := pkg.FindPotentialTargetConflicts(potentialTargetMonthDir, baseNameWithoutExt, originalExtension)
		if errFind != nil {
			log.Printf("  - Error scanning for target conflicts for %s: %v. Assuming no conflicts.\n", currentSourceFilepath, errFind)
		}

		// 2.c Loop through existingTargetCandidates
		for _, targetCandidatePath := range existingTargetCandidates {
			fmt.Printf("    - Comparing source %s with target candidate %s\n", currentSourceFilepath, targetCandidatePath)
			compResult, errComp := pkg.AreFilesPotentiallyDuplicate(currentSourceFilepath, targetCandidatePath)
			if errComp != nil {
				log.Printf("      - Error comparing source %s with target %s: %v. Skipping this target candidate.\n", currentSourceFilepath, targetCandidatePath, errComp)
				if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath) {
					sourceFilesThatUsedFileHash[currentSourceFilepath] = true
				}
				continue
			}
			if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath) {
				sourceFilesThatUsedFileHash[currentSourceFilepath] = true
			}

			if compResult.AreDuplicates {
				isDuplicateOfTargetFile = true
				fmt.Printf("      - Duplicate found: Source %s and Target %s. Reason: %s\n", currentSourceFilepath, targetCandidatePath, compResult.Reason)

				if compResult.Reason == pkg.ReasonPixelHashMatch {
					targetCandidateWidth, targetCandidateHeight, errResTarget := pkg.GetImageResolution(targetCandidatePath)
					if errResTarget != nil {
						log.Printf("      - Warning: Could not get resolution for target %s: %v. Assuming target is not better.\n", targetCandidatePath, errResTarget)
						if currentSourceInfo.Width*currentSourceInfo.Height > 0 {
							sourceFileIsBetterDuplicate = true
							duplicates = append(duplicates, pkg.DuplicateInfo{
								KeptFile:      currentSourceInfo.Path,
								DiscardedFile: targetCandidatePath,
								Reason:        compResult.Reason + " (source is better resolution than existing target, target resolution unknown)",
							})
							fmt.Printf("      - Source %s is considered better than target %s (target resolution error).\n", currentSourceFilepath, targetCandidatePath)
							break
						} else {
							duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: targetCandidatePath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + " (existing target kept - resolution error for both or source)"})
							fmt.Printf("      - Target %s kept (pixel hash match, resolution error for source/both).\n", targetCandidatePath)
							goto nextSourceFileIteration
						}
					}

					if currentSourceInfo.Width*currentSourceInfo.Height > targetCandidateWidth*targetCandidateHeight {
						sourceFileIsBetterDuplicate = true
						duplicates = append(duplicates, pkg.DuplicateInfo{
							KeptFile:      currentSourceInfo.Path,
							DiscardedFile: targetCandidatePath,
							Reason:        compResult.Reason + " (source is better resolution than existing target)",
						})
						fmt.Printf("      - Source %s (%dx%d) is better resolution than target %s (%dx%d).\n", currentSourceFilepath, currentSourceInfo.Width, currentSourceInfo.Height, targetCandidatePath, targetCandidateWidth, targetCandidateHeight)
						break
					} else {
						duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: targetCandidatePath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + " (existing target kept - resolution)"})
						fmt.Printf("      - Target %s (%dx%d) is better or same resolution as source %s (%dx%d).\n", targetCandidatePath, targetCandidateWidth, targetCandidateHeight, currentSourceFilepath, currentSourceInfo.Width, currentSourceInfo.Height)
						goto nextSourceFileIteration
					}
				} else {
					// No need to check compResult.HashTypeSource, already done for sourceFilesThatUsedFileHash
					duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: targetCandidatePath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason})
					fmt.Printf("      - Target %s kept (reason: %s).\n", targetCandidatePath, compResult.Reason)
					goto nextSourceFileIteration
				}
			}
		}

		// 3. Check Against `filesSelectedForCopy` (Existing Logic - Adapt as Needed)
		// This happens if !isDuplicateOfTargetFile OR if sourceFileIsBetterDuplicate
		if !isDuplicateOfTargetFile || sourceFileIsBetterDuplicate {
			isDuplicateOfSelected := false
			shouldReplaceExistingSelected := false
			indexOfExistingToReplace := -1
			reasonForReplacement := ""

			for idx, existingSelectedInfo := range filesSelectedForCopy {
				fmt.Printf("  - Comparing with already selected file for copy: %s\n", existingSelectedInfo.Path)
				compResult, errComp := pkg.AreFilesPotentiallyDuplicate(currentSourceInfo.Path, existingSelectedInfo.Path)

				if errComp != nil {
					log.Printf("    - Error comparing %s and %s: %v. Treating as non-duplicate for this pair.\n", currentSourceInfo.Path, existingSelectedInfo.Path, errComp)
					if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceInfo.Path) {
						sourceFilesThatUsedFileHash[currentSourceInfo.Path] = true
					}
					continue
				}
				if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceInfo.Path) {
					sourceFilesThatUsedFileHash[currentSourceInfo.Path] = true
				}

				if compResult.AreDuplicates {
					if compResult.Reason == pkg.ReasonPixelHashMatch {
						fmt.Printf("    - Pixel hash match with selected %s.\n", existingSelectedInfo.Path)
						if currentSourceInfo.Width*currentSourceInfo.Height > existingSelectedInfo.Width*existingSelectedInfo.Height {
							fmt.Printf("    - Current file %s (%dx%d) is better resolution than selected %s (%dx%d).\n",
								currentSourceInfo.Path, currentSourceInfo.Width, currentSourceInfo.Height,
								existingSelectedInfo.Path, existingSelectedInfo.Width, existingSelectedInfo.Height)
							if !shouldReplaceExistingSelected || (currentSourceInfo.Width*currentSourceInfo.Height > filesSelectedForCopy[indexOfExistingToReplace].Width*filesSelectedForCopy[indexOfExistingToReplace].Height) {
								shouldReplaceExistingSelected = true
								indexOfExistingToReplace = idx
								reasonForReplacement = fmt.Sprintf("%s (current '%s' has better resolution than previously selected '%s')", compResult.Reason, filepath.Base(currentSourceInfo.Path), filepath.Base(existingSelectedInfo.Path))
							}
						} else {
							fmt.Printf("    - Existing selected file %s (%dx%d) is better or same resolution as current %s (%dx%d).\n",
								existingSelectedInfo.Path, existingSelectedInfo.Width, existingSelectedInfo.Height,
								currentSourceInfo.Path, currentSourceInfo.Width, currentSourceInfo.Height)
							duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: existingSelectedInfo.Path, DiscardedFile: currentSourceInfo.Path, Reason: compResult.Reason + " (already selected kept - resolution)"})
							goto nextSourceFileIteration
						}
					} else { // Other duplicate type like file hash
						// if compResult.HashTypeSource == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceInfo.Path) {
						// pixelHashUnsupported++ // Potentially overcounting
						// }
						fmt.Printf("    - Duplicate (Reason: %s) with selected %s. Keeping existing selected file.\n", compResult.Reason, existingSelectedInfo.Path)
						duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: existingSelectedInfo.Path, DiscardedFile: currentSourceInfo.Path, Reason: compResult.Reason})
						goto nextSourceFileIteration
					}
				}
			}

			if shouldReplaceExistingSelected {
				discardedInfo := filesSelectedForCopy[indexOfExistingToReplace]
				// Update duplicate report: KeptFile is currentSourceInfo.Path (its final target name will be determined at copy time)
				// DiscardedFile is the one it replaces from the filesSelectedForCopy list.
				duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: currentSourceInfo.Path, DiscardedFile: discardedInfo.Path, Reason: reasonForReplacement})
				filesSelectedForCopy[indexOfExistingToReplace] = currentSourceInfo
				fmt.Printf("  - Marked for replacement in selection list: %s will replace %s (Reason: %s)\n", currentSourceInfo.Path, discardedInfo.Path, reasonForReplacement)
			} else if !isDuplicateOfSelected {
				filesSelectedForCopy = append(filesSelectedForCopy, currentSourceInfo)
				fmt.Printf("  - Marked for copy: %s (unique or better than target, and unique among selected)\n", currentSourceInfo.Path)
			}
		}

		// Determine if pixelHashUnsupported should be incremented for currentSourceFilepath
		// This is tricky because it depends on the *final* outcome of comparisons for this file.
		// A simpler approach: if a file is an image, and it's added to filesSelectedForCopy,
		// and its comparison (if any occurred leading to its selection over another image)
		// was ultimately based on file hash, then count it.
		// This is still complex. The `AreFilesPotentiallyDuplicate` pkg function now returns HashTypeSource and HashTypeTarget.
		// We can inspect these.
		// The current `pixelHashUnsupported++` calls after `AreFilesPotentiallyDuplicate` are okay if they refer to `compResult.HashTypeSource`.

		nextSourceFileIteration:
	}

	// Consolidate pixelHashUnsupported counting.
	// This is difficult to do accurately without knowing which comparison was "definitive" for a file.
	// The current per-comparison incrementing inside the loop is likely the most practical if a bit noisy.
	// Let's ensure the increments are correctly placed for `compResult.HashTypeSource`.
	// The existing `pixelHashUnsupported++` after each `pkg.AreFilesPotentiallyDuplicate` call,
	// if `compResult.HashTypeSource == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath)`
	// is the most direct way, but it might count a single source file multiple times if it's compared multiple times
	// and uses file hash each time.
	// A better way: maintain a set of source file paths that used file hash when they are images.
	// sourceFilesThatUsedFileHash := make(map[string]bool) // This was moved to the top of the function.

	// Re-iterate just for pixelHashUnsupported counting logic based on compResult.HashTypeSource (this is not efficient)
	// This is too complex. The current incrementing logic for pixelHashUnsupported is likely sufficient for an estimate.
	// The problem statement asks for "Number of files for which pixel-data hashing was not supported (and therefore used full file content hashing)."
	// This implies a per-source-file count.
	// The current implementation with `pixelHashUnsupported++` inside the loops when `HashTypeSource` is `HashTypeFile` for an image
	// will count *comparisons* not *files*.
	//
	// Correct approach for pixelHashUnsupported:
	// Iterate `imageFiles`. For each, if it's an image, simulate its path through the decision logic.
	// If its *final* comparison type (that determined its fate or lack of duplication) was FileHash, increment.
	// This is too much of a refactor for this step.
	// I will leave the `pixelHashUnsupported` logic as is (incrementing on `compResult.HashTypeSource`)
	// and acknowledge it might count comparisons rather than unique files.
	// Or, use the `sourceFilesUsingFileHash` map approach.

	// Let's try the map approach for more accurate `pixelHashUnsupported`
	// This map will be populated during the main loop if a source image file used file hash.
	// The `pixelHashUnsupported++` lines need to add to this map instead.
	// Then, `pixelHashUnsupported = len(sourceFilesUsingFileHash)`.
	// This change will be done in the next iteration if this one is too long.
	// For now, the existing duplicate `pixelHashUnsupported++` calls are removed.
	// A single point of increment will be added if a file makes it to `filesSelectedForCopy`
	// and its last comparison was FileHash, OR if it was discarded due to FileHash. This is still complex.

	// Simplification: The `pkg.AreFilesPotentiallyDuplicate` function's `compResult.HashType` field
	// (renamed to `HashTypeSource` and `HashTypeTarget` in my mental model, but it's just `HashType` in current pkg)
	// should reflect the source file's hashing method if it was the primary one.
	// The current `pixelHashUnsupported++` after each call to AreFilesPotentiallyDuplicate, if HashType == File and IsImage, is the specified logic.
	// I had removed them in the previous diff section, I should ensure they are correctly placed.

	// Correct placement of pixelHashUnsupported increment:
	// Inside the loops, after `AreFilesPotentiallyDuplicate` returns `compResult`:
	// if compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceInfo.Path) {
	//   // Add to a set to count unique files later
	//	 sourceFilesThatUsedFileHash[currentSourceInfo.Path] = true
	// }
	// Then, after the main loop: pixelHashUnsupported = len(sourceFilesThatUsedFileHash)
	// This seems like the best way.

	// sourceFilesThatUsedFileHash := make(map[string]bool) // Moved to before the loop

	// The main processing loop (already exists, just showing context for sourceFilesThatUsedFileHash)
	// for _, currentSourceFilepath := range imageFiles { ...
	//   ...
	//   compResult, errComp := pkg.AreFilesPotentiallyDuplicate(...)
	//   if errComp == nil && compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceInfo.Path) {
	//      sourceFilesThatUsedFileHash[currentSourceInfo.Path] = true
	//   }
	//   ...
	// }
	// After the loop: pixelHashUnsupported = len(sourceFilesThatUsedFileHash)
	// This change will be applied in the next step as it requires modifying the loop structure already changed.
	// For now, I'll ensure the `KeptFile` in duplicate reports is correct.
	pixelHashUnsupported = len(sourceFilesThatUsedFileHash)


	fmt.Printf("\n--- Starting Copying Phase ---\nFound %d files to copy.\n", len(filesSelectedForCopy))
	keptFileSourceToTargetMap := make(map[string]string)

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

		// Determine the final destination path, handling versioning
		newFileName := baseName + originalExtension
		destPath := filepath.Join(targetMonthDir, newFileName)
		version := 1
		originalDestPathForLog := destPath // Store the initial non-versioned path for logging

		// Check for existing files (both original and versioned ones)
		// This re-checks, which is good for robustness, especially if new files appeared.
		for {
			_, statErr := os.Stat(destPath)
			if os.IsNotExist(statErr) {
				if destPath != originalDestPathForLog && version > 1 { // only log if it's a versioned name different from original
					fmt.Printf("  - Path %s existed. Using versioned name: %s\n", filepath.Join(targetMonthDir, fmt.Sprintf("%s-%d%s", baseName, version-1, originalExtension)), destPath)
				} else if destPath != originalDestPathForLog && version == 1 && strings.Contains(originalDestPathForLog, "-0") {
					// This case is tricky, if original had -0. Unlikely with current naming.
				}
				break // Found an available name
			} else if statErr != nil {
				// Serious error checking the destination path (permissions, etc.)
				log.Printf("  - Error stating file %s: %v. Skipping copy of %s.\n", destPath, statErr, finalInfoToCopy.Path)
				goto nextFileToCopyLoop // Skip this file
			}
			// File exists, try next version
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
			keptFileSourceToTargetMap[finalInfoToCopy.Path] = destPath // Store mapping for report update
		}
		nextFileToCopyLoop:
	}

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
	filesToCopy = len(filesSelectedForCopy)
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
	processed, copied, filesToCopy, duplicates, pixelHashUnsupported, appErr := runApplicationLogic(sourceDir, targetBaseDir)
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
	log.Printf("Run Summary: Processed: %d, Copied: %d, Selected To Copy: %d, Duplicates Found: %d, Pixel Hash Unsupported (Unique Files): %d\n",
		processed, copied, filesToCopy, len(duplicates), pixelHashUnsupported)
}
