package photocp

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time" // time.Time is used for photoDate variable type and other time operations

	_ "github.com/vegidio/heif-go" // Register HEIF/HEVC decoder
	_ "image/gif"                  // Register GIF decoder
	_ "image/jpeg"                 // Register JPEG decoder
	_ "image/png"                  // Register PNG decoder

	"github.com/user/photo-sorter/pkg"
)

// scanSourceDirectory scans the source directory for image files.
func scanSourceDirectory(sourceDir string, verbose bool) ([]string, error) {
	// This message should always print, using fmt for cleaner output.
	fmt.Printf("Scanning source directory: %s\n", sourceDir)
	imageFiles, scanErr := pkg.ScanSourceDirectory(sourceDir)
	if scanErr != nil {
		// This warning is conditional on verbose.
		if verbose {
			log.Printf("Warning during scanning source directory '%s': %v. Attempting to continue with any found files.\n", sourceDir, scanErr)
		}
		if imageFiles == nil { // If the error was critical and no files could be read
			// This is a critical error, always show.
			return nil, fmt.Errorf("critical error: No files could be read from source directory '%s'", sourceDir)
		}
	}
	return imageFiles, nil
}

// ensureTargetDirectory ensures the target base directory exists, creating it if necessary.
func ensureTargetDirectory(targetBaseDir string, verbose bool) error {
	if _, err := os.Stat(targetBaseDir); os.IsNotExist(err) {
		fmt.Printf("Target directory %s does not exist, attempting to create it.\n", targetBaseDir)
		if errMkdir := os.MkdirAll(targetBaseDir, 0755); errMkdir != nil {
			// This is a critical error, always show.
			return fmt.Errorf("failed to create target base directory '%s': %w", targetBaseDir, errMkdir)
		}
	} else if err != nil {
		// This is a critical error, always show.
		return fmt.Errorf("error accessing target base directory '%s': %w", targetBaseDir, err)
	}
	return nil
}

// determinePhotoDateAndDateSource tries to get the date from EXIF, falling back to file modification time.
func determinePhotoDateAndDateSource(currentSourceFilepath string, verbose bool) (photoDate time.Time, dateSource string, err error) {
	exifDate, dateErr := pkg.GetPhotoCreationDate(currentSourceFilepath)
	if dateErr == nil {
		photoDate = exifDate
		dateSource = "EXIF"
	} else {
		fileInfoStat, statErr := os.Stat(currentSourceFilepath)
		if statErr != nil {
			if verbose {
				log.Printf("  - Error getting file info for %s: %v. Skipping this file.\n", currentSourceFilepath, statErr)
			}
			return time.Time{}, "", fmt.Errorf("error getting file info: %w", statErr)
		}
		photoDate = fileInfoStat.ModTime()
		dateSource = "FileModTime"
	}
	if verbose {
		log.Printf("  - Determined date (%s) for %s: %s\n", dateSource, currentSourceFilepath, photoDate.Format("2006-01-02 15:04:05"))
	}
	return photoDate, dateSource, nil
}

// determineTargetPath creates the target directory path and filename.
func determineTargetPath(targetBaseDir string, photoDate time.Time, sourceFilePath string, verbose bool) (exactTargetPath string, targetMonthDir string, err error) {
	targetMonthDir, err = pkg.CreateTargetDirectory(targetBaseDir, photoDate)
	if err != nil {
		if verbose {
			log.Printf("  - Error creating/accessing target month directory for %s (date: %s): %v. Skipping.\n", sourceFilePath, photoDate, err)
		}
		return "", "", fmt.Errorf("error creating target month directory: %w", err)
	}

	originalExtension := filepath.Ext(sourceFilePath)
	baseNameWithoutExt := photoDate.In(time.UTC).Format("2006-01-02-150405")
	targetFileName := baseNameWithoutExt + originalExtension
	exactTargetPath = filepath.Join(targetMonthDir, targetFileName)

	if verbose {
		log.Printf("  - Proposed target path: %s\n", exactTargetPath)
	}
	return exactTargetPath, targetMonthDir, nil
}

// checkAndCopyIfTargetEmpty checks if the target path is empty and copies the file if it is.
// Returns true if copied, false if target existed or copy error. Error is returned for system/copy errors.
func checkAndCopyIfTargetEmpty(sourceFilePath string, exactTargetPath string, verbose bool) (copied bool, err error) {
	_, statErr := os.Stat(exactTargetPath)
	if statErr == nil { // File exists
		if verbose {
			log.Printf("  - File already exists at target path: %s\n", exactTargetPath)
		}
		return false, nil // Not copied by this function, target exists
	} else if !os.IsNotExist(statErr) { // Other stat error
		if verbose {
			log.Printf("  - Error checking target path %s: %v. Skipping source file %s.\n", exactTargetPath, statErr, sourceFilePath)
		}
		return false, fmt.Errorf("error checking target path %s: %w", exactTargetPath, statErr)
	}

	// Target does not exist (os.IsNotExist(statErr) is true)
	if verbose {
		log.Printf("  - Target path %s is empty. Copying %s directly.\n", exactTargetPath, sourceFilePath)
	}
	if copyErr := pkg.CopyFile(sourceFilePath, exactTargetPath); copyErr != nil {
		if verbose {
			log.Printf("  - Error copying file %s to %s: %v.\n", sourceFilePath, exactTargetPath, copyErr)
		}
		return false, fmt.Errorf("error copying file %s to %s: %w", sourceFilePath, exactTargetPath, copyErr)
	}
	if verbose {
		log.Printf("  - Successfully copied %s to %s\n", sourceFilePath, exactTargetPath)
	}
	return true, nil // Copied successfully
}

// handleTargetConflict deals with situations where a file already exists at the target path.
func handleTargetConflict(currentSourceFilepath string, exactTargetPath string, currentWidth int, currentHeight int, verbose bool) (copied bool, finalTargetPath string, duplicateInfo *pkg.DuplicateInfo, usedFileHash bool, err error) {
	if verbose {
		log.Printf("    - Comparing source %s with existing target %s\n", currentSourceFilepath, exactTargetPath)
	}
	compResult, errComp := pkg.AreFilesPotentiallyDuplicate(currentSourceFilepath, exactTargetPath)
	currentUsedFileHash := compResult.HashType == pkg.HashTypeFile && pkg.IsImageExtension(currentSourceFilepath)

	if errComp != nil {
		if verbose {
			log.Printf("      - Error comparing source %s with target %s: %v. Assuming target is kept.\n", currentSourceFilepath, exactTargetPath, errComp)
		}
		dupInfo := pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: "Comparison error, existing target kept"}
		return false, exactTargetPath, &dupInfo, currentUsedFileHash, nil // Not an error that stops processing other files, but report duplicate.
	}

	if !compResult.AreDuplicates {
		if verbose {
			log.Printf("      - Source %s and target %s are deemed different by content comparison, but share the same target path. Discarding source to protect existing target.\n", currentSourceFilepath, exactTargetPath)
		}
		dupInfo := pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: "Content different, but name collision; existing target preserved"}
		return false, exactTargetPath, &dupInfo, currentUsedFileHash, nil
	}

	// Files are duplicates
	if verbose {
		log.Printf("      - Duplicate found: Source %s and Target %s. Reason: %s\n", currentSourceFilepath, exactTargetPath, compResult.Reason)
	}
	targetResolutionBetterOrEqual := true

	if compResult.Reason == pkg.ReasonPixelHashMatch {
		targetWidth, targetHeight, errResTarget := pkg.GetImageResolution(exactTargetPath)
		if errResTarget != nil {
			if verbose {
				log.Printf("      - Warning: Could not get resolution for target %s: %v. Source might replace if it has resolution.\n", exactTargetPath, errResTarget)
			}
			if currentWidth*currentHeight > 0 { // Source has valid resolution
				targetResolutionBetterOrEqual = false
			} else { // Source also has resolution error or 0x0
				dupInfo := pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + " (existing target kept - resolution error for target, source has no resolution or also error)"}
				if verbose {
					log.Printf("      - Target %s kept (pixel hash match, resolution error for target and source has no resolution).\n", exactTargetPath)
				}
				return false, exactTargetPath, &dupInfo, currentUsedFileHash, nil
			}
		} else { // Target resolution is available
			if verbose {
				log.Printf("      - Target resolution: %dx%d\n", targetWidth, targetHeight)
			}
			if currentWidth*currentHeight > targetWidth*targetHeight {
				targetResolutionBetterOrEqual = false
			}
		}
	}

	if !targetResolutionBetterOrEqual { // Source is better resolution
		if verbose {
			log.Printf("      - Source %s (%dx%d) is better than target %s. Replacing target.\n", currentSourceFilepath, currentWidth, currentHeight, exactTargetPath)
		}
		dupInfo := pkg.DuplicateInfo{
			KeptFile:      currentSourceFilepath, // Source is kept, will be copied to exactTargetPath
			DiscardedFile: exactTargetPath,
			Reason:        compResult.Reason + " (source is better resolution)",
		}
		if copyErr := pkg.CopyFile(currentSourceFilepath, exactTargetPath); copyErr != nil {
			if verbose {
				log.Printf("      - Error overwriting target file %s with source %s: %v. Original target remains.\n", exactTargetPath, currentSourceFilepath, copyErr)
			}
			// If overwrite fails, the original target was kept. Adjust DuplicateInfo.
			dupInfo.KeptFile = exactTargetPath
			dupInfo.DiscardedFile = currentSourceFilepath
			dupInfo.Reason = "Attempted replacement failed, original target kept"
			return false, exactTargetPath, &dupInfo, currentUsedFileHash, nil // Not an error for runApplicationLogic, but a handled duplicate.
		}
		if verbose {
			log.Printf("      - Successfully overwrote %s with %s\n", exactTargetPath, currentSourceFilepath)
		}
		// Successfully replaced, so copied is true, finalTargetPath is exactTargetPath
		return true, exactTargetPath, &dupInfo, currentUsedFileHash, nil
	}

	// Target is better or same resolution, or not a pixel hash match (e.g. file hash match, where resolution is not the primary factor for replacement)
	reasonSuffix := ""
	if compResult.Reason == pkg.ReasonPixelHashMatch { // Only add resolution suffix if it was a pixel hash match and target was kept due to resolution
		reasonSuffix = " (existing target kept - resolution)"
	} else {
		reasonSuffix = " (existing target kept)"
	}
	dupInfo := pkg.DuplicateInfo{KeptFile: exactTargetPath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + reasonSuffix}
	if verbose {
		log.Printf("      - Target %s kept (source %s discarded). Reason: %s\n", exactTargetPath, currentSourceFilepath, compResult.Reason+reasonSuffix)
	}
	return false, exactTargetPath, &dupInfo, currentUsedFileHash, nil
}

// processSingleFile handles the logic for processing one image file.
// It returns whether the file was copied, the path it was copied to (if applicable),
// any duplicate information, if file hash was used, and any error.
func processSingleFile(currentSourceFilepath string, targetBaseDir string, verbose bool, existingTargetFiles map[string]string) (copied bool, finalTargetPath string, duplicateInfo *pkg.DuplicateInfo, usedFileHash bool, err error) {
	if verbose {
		log.Printf("\nProcessing: %s\n", currentSourceFilepath)
	}

	// 1.a Determine photoDate and dateSource
	photoDate, _, err := determinePhotoDateAndDateSource(currentSourceFilepath, verbose)
	if err != nil {
		// The error is already logged by determinePhotoDateAndDateSource if verbose.
		// Return the error to be handled by the caller.
		return false, "", nil, false, err
	}

	// 1.b Determine target path
	var exactTargetPath string // Declare exactTargetPath
	exactTargetPath, _, err = determineTargetPath(targetBaseDir, photoDate, currentSourceFilepath, verbose)
	if err != nil {
		// Error is already logged by determineTargetPath if verbose.
		return false, "", nil, false, err
	}

	currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentSourceFilepath)
	if errRes != nil {
		if verbose {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Proceeding with 0x0 resolution.\n", currentSourceFilepath, errRes)
		}
		currentWidth = 0
		currentHeight = 0
		// Not returning an error here as we proceed with 0x0 resolution
	} else {
		if verbose {
			log.Printf("  - Source resolution: %dx%d\n", currentWidth, currentHeight)
		}
	}

	// 2. Check if target is empty and copy if so
	wasCopied, copyErr := checkAndCopyIfTargetEmpty(currentSourceFilepath, exactTargetPath, verbose)
	if copyErr != nil {
		// Propagate error from checkAndCopyIfTargetEmpty
		return false, "", nil, false, copyErr
	}
	if wasCopied {
		// File was successfully copied to an empty target path
		return true, exactTargetPath, nil, false, nil
	}

	// Conflict: File exists at exactTargetPath. Call conflict resolution.
	return handleTargetConflict(currentSourceFilepath, exactTargetPath, currentWidth, currentHeight, verbose)
}

// processImageFiles iterates over image files, processes them, and collects results.
func processImageFiles(imageFiles []string, targetBaseDir string, verbose bool, existingTargetFiles map[string]string) (
	copiedCount int,
	duplicatesList []pkg.DuplicateInfo,
	sourceFilesThatUsedFileHash map[string]bool,
	keptFileSourceToTargetMap map[string]string,
	processingErrors []error,
) {
	// Initialize return values
	sourceFilesThatUsedFileHash = make(map[string]bool)
	keptFileSourceToTargetMap = make(map[string]string)
	duplicatesList = []pkg.DuplicateInfo{} // Ensure it's not nil
	processingErrors = []error{}           // Ensure it's not nil

	numImageFiles := len(imageFiles)
	progressInterval := numImageFiles / 10
	if progressInterval == 0 && numImageFiles > 0 {
		progressInterval = 1
	}
	if numImageFiles < 10 {
		progressInterval = 1
	}

	for i, currentSourceFilepath := range imageFiles {
		copied, finalTargetPath, dupInfo, usedFH, processErr := processSingleFile(currentSourceFilepath, targetBaseDir, verbose, existingTargetFiles)

		if processErr != nil {
			processingErrors = append(processingErrors, processErr)
			// Error for this specific file is logged verbosely within processSingleFile if verbose.
			// Continue processing other files.
		}

		if usedFH {
			sourceFilesThatUsedFileHash[currentSourceFilepath] = true
		}
		if copied {
			copiedCount++
			if finalTargetPath == "" {
				if verbose {
					log.Printf("Internal error: file %s reported as copied but no finalTargetPath returned.", currentSourceFilepath)
				}
				// Optionally, add to processingErrors or handle as a specific type of error
			} else {
				keptFileSourceToTargetMap[currentSourceFilepath] = finalTargetPath
			}
		}

		if dupInfo != nil {
			duplicatesList = append(duplicatesList, *dupInfo)
		}

		if !verbose && progressInterval > 0 && (i+1)%progressInterval == 0 && (i+1) != numImageFiles {
			fmt.Printf("Processed %d of %d files...\n", i+1, numImageFiles)
		}
	}

	if !verbose && numImageFiles > 0 {
		fmt.Println("All files processed.")
	}
	return
}

// RunApplicationLogic is the core processing function for the photo sorter.
// It scans the source directory, processes each image file, handles duplicates,
// and copies files to the target directory, generating a report of its actions.
// It is exported for use in tests.
func RunApplicationLogic(sourceDir string, targetBaseDir string, verbose bool) (processedFilesCount int, copiedFilesCount int, filesToCopyCount int, duplicatesList []pkg.DuplicateInfo, pixelHashUnsupportedCount int, err error) {
	reportFilePath := filepath.Join(targetBaseDir, "report.txt")
	fmt.Printf("Photo Sorter Initializing...\nSource: %s\nTarget: %s\nReport: %s\n", sourceDir, targetBaseDir, reportFilePath)

	// existingTargetFiles is declared for processSingleFile, but might remain unused if os.Stat is preferred.
	existingTargetFiles := make(map[string]string)

	if err := ensureTargetDirectory(targetBaseDir, verbose); err != nil {
		return 0, 0, 0, nil, 0, err
	}

	imageFiles, scanErr := scanSourceDirectory(sourceDir, verbose)
	if scanErr != nil {
		return 0, 0, 0, nil, 0, scanErr
	}

	processedFilesCount = len(imageFiles)

	if processedFilesCount == 0 {
		fmt.Println("No image files found in source directory.")
		if genErr := pkg.GenerateReport(reportFilePath, duplicatesList, 0, 0, 0, 0); genErr != nil {
			return 0, 0, 0, nil, 0, fmt.Errorf("failed to generate final report: %w", genErr)
		}
		return 0, 0, 0, nil, 0, nil
	}

	fmt.Printf("Found %d image file(s) to process.\n", processedFilesCount)

	var processingErrors []error
	var sourceFilesThatUsedFileHash map[string]bool
	var keptFileSourceToTargetMap map[string]string

	copiedFilesCount, duplicatesList, sourceFilesThatUsedFileHash, keptFileSourceToTargetMap, processingErrors = processImageFiles(imageFiles, targetBaseDir, verbose, existingTargetFiles)

	// Log any non-critical processing errors encountered during the loop
	if len(processingErrors) > 0 && verbose {
		log.Printf("Encountered %d non-critical errors during file processing:", len(processingErrors))
		for _, procErr := range processingErrors {
			log.Printf("  - %v", procErr)
		}
	}

	pixelHashUnsupportedCount = len(sourceFilesThatUsedFileHash)

	// Update KeptFile paths in duplicates report
	for i, dup := range duplicatesList {
		if targetPath, ok := keptFileSourceToTargetMap[dup.KeptFile]; ok {
			duplicatesList[i].KeptFile = targetPath
		}
	}

	fmt.Println("\n--- Photo Sorting Process Completed ---")
	filesToCopyCount = copiedFilesCount
	if genErr := pkg.GenerateReport(reportFilePath, duplicatesList, copiedFilesCount, processedFilesCount, filesToCopyCount, pixelHashUnsupportedCount); genErr != nil {
		return processedFilesCount, copiedFilesCount, filesToCopyCount, duplicatesList, pixelHashUnsupportedCount, fmt.Errorf("failed to generate final report: %w", genErr)
	}
	return processedFilesCount, copiedFilesCount, filesToCopyCount, duplicatesList, pixelHashUnsupportedCount, nil
}

// This is the main application entry point.
func main() {
	// --- Command-line flags ---
	sourceDirFlag := flag.String("sourceDir", "", "Source directory containing photos to sort (e.g., common formats like JPG, PNG, GIF, HEIC, and various RAW types) (required)")
	targetDirFlag := flag.String("targetDir", "", "Target directory to store sorted photos (required)")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output for detailed processing information.")
	helpFlg := flag.Bool("help", false, "Show help message and license information")
	flag.Parse()

	if *helpFlg {
		fmt.Println("Usage: photocp -sourceDir <source_directory> -targetDir <target_directory> [-verbose]")
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
		fmt.Println("  - heif-go (github.com/vegidio/heif-go)")
		fmt.Println("    - Purpose: Used to decode HEIF/HEIC image files.")
		fmt.Println("    - License: MIT License")
		fmt.Println("    - Copyright: Copyright (c) Vinicius Egidio")
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
	verbose := *verboseFlag

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
	processed, copied, _, duplicates, pixelHashUnsupported, appErr := RunApplicationLogic(sourceDir, targetBaseDir, verbose) // filesToCopy is now internal to runApplicationLogic or same as copied
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
	// The third returned value from runApplicationLogic (old filesToCopy) is now the same as copied.
	// So, for the log, we can use 'copied' for "Selected To Copy" or just report "Copied".
	// Let's adjust the log message to reflect that "Selected To Copy" isn't a separate concept anymore.
	// Final summary message, should always print, use fmt for cleaner output.
	fmt.Printf("Run Summary: Processed: %d, Copied: %d, Duplicates Found: %d, Pixel Hash Unsupported (Unique Files): %d\n",
		processed, copied, len(duplicates), pixelHashUnsupported)
}
