package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/user/photo-sorter/pkg"
)

// FileInfo holds path and resolution for a file being processed.
type FileInfo struct {
	Path   string
	Width  int
	Height int
}

// runApplicationLogic orchestrates the photo sorting process.
// See docs/technical_details.md for detailed algorithm explanation.
func runApplicationLogic(sourceDir string, targetBaseDir string) (processedFiles int, copiedFiles int, filesToCopy int, duplicates []pkg.DuplicateInfo, pixelHashUnsupported int, err error) {
	reportFilePath := filepath.Join(targetBaseDir, "report.txt")
	fmt.Printf("Photo Sorter Initializing...\nSource: %s\nTarget: %s\nReport: %s\n", sourceDir, targetBaseDir, reportFilePath)

	filesSelectedForCopy := []FileInfo{}
	sourceFilesThatUsedFileHash := make(map[string]bool)

	if _, errStat := os.Stat(targetBaseDir); os.IsNotExist(errStat) {
		fmt.Printf("Target directory %s does not exist, attempting to create it.\n", targetBaseDir)
		if errMkdir := os.MkdirAll(targetBaseDir, 0755); errMkdir != nil {
			return 0, 0, 0, nil, 0, fmt.Errorf("failed to create target base directory '%s': %w", targetBaseDir, errMkdir)
		}
	} else if errStat != nil {
		return 0, 0, 0, nil, 0, fmt.Errorf("error accessing target base directory '%s': %w", targetBaseDir, errStat)
	}

	fmt.Printf("Scanning source directory: %s\n", sourceDir)
	imageFiles, scanErr := pkg.ScanSourceDirectory(sourceDir)
	if scanErr != nil {
		log.Printf("Warning during scanning source directory '%s': %v. Attempting to continue with any found files.\n", sourceDir, scanErr)
		if imageFiles == nil {
			return 0, 0, 0, nil, 0, fmt.Errorf("critical error: No files could be read from source directory '%s'", sourceDir)
		}
	}

	if len(imageFiles) == 0 {
		fmt.Println("No image files found in source directory.")
		if genErr := pkg.GenerateReport(reportFilePath, duplicates, copiedFiles, processedFiles, 0, pixelHashUnsupported); genErr != nil {
			return 0, 0, 0, duplicates, pixelHashUnsupported, fmt.Errorf("failed to generate final report: %w", genErr)
		}
		return 0, 0, 0, duplicates, pixelHashUnsupported, nil
	}

	fmt.Printf("Found %d image file(s) to process.\n", len(imageFiles))
	processedFiles = len(imageFiles)

	for _, currentSourceFilepath := range imageFiles {
		fmt.Printf("\nProcessing: %s\n", currentSourceFilepath)

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
				continue
			}
			photoDate = fileInfoStat.ModTime()
			dateSource = "FileModTime"
		}
		fmt.Printf("  - Determined date (%s): %s\n", dateSource, photoDate.Format("2006-01-02 15:04:05"))

		potentialTargetMonthDir, errDir := pkg.CreateTargetDirectory(targetBaseDir, photoDate)
		if errDir != nil {
			log.Printf("  - Error creating/accessing target month directory for %s (date: %s): %v. Skipping.\n", currentSourceFilepath, photoDate, errDir)
			continue
		}

		originalExtension := filepath.Ext(currentSourceFilepath)
		baseNameWithoutExt := photoDate.In(time.UTC).Format("2006-01-02-150405")

		currentWidth, currentHeight, errRes := pkg.GetImageResolution(currentSourceFilepath)
		if errRes != nil {
			log.Printf("  - Warning: Could not get resolution for %s: %v. Proceeding with 0x0 resolution.\n", currentSourceFilepath, errRes)
			currentWidth = 0
			currentHeight = 0
		} else {
			fmt.Printf("  - Resolution: %dx%d\n", currentWidth, currentHeight)
		}
		currentSourceInfo := FileInfo{Path: currentSourceFilepath, Width: currentWidth, Height: currentHeight}

		isDuplicateOfTargetFile := false
		sourceFileIsBetterDuplicate := false

		fmt.Printf("  - Checking for existing files in target: %s (base: %s, ext: %s)\n", potentialTargetMonthDir, baseNameWithoutExt, originalExtension)
		existingTargetCandidates, errFind := pkg.FindPotentialTargetConflicts(potentialTargetMonthDir, baseNameWithoutExt, originalExtension)
		if errFind != nil {
			log.Printf("  - Error scanning for target conflicts for %s: %v. Assuming no conflicts.\n", currentSourceFilepath, errFind)
		}

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
						if currentSourceInfo.Width*currentSourceInfo.Height > 0 { // Source has valid resolution
							sourceFileIsBetterDuplicate = true
							duplicates = append(duplicates, pkg.DuplicateInfo{
								KeptFile:      currentSourceInfo.Path,
								DiscardedFile: targetCandidatePath,
								Reason:        compResult.Reason + " (source is better resolution than existing target, target resolution unknown)",
							})
							fmt.Printf("      - Source %s is considered better than target %s (target resolution error).\n", currentSourceFilepath, targetCandidatePath)
							break
						} else { // Neither source nor target has easily obtainable resolution, or source doesn't
							duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: targetCandidatePath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason + " (existing target kept - resolution error for source/both)"})
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
					duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: targetCandidatePath, DiscardedFile: currentSourceFilepath, Reason: compResult.Reason})
					fmt.Printf("      - Target %s kept (reason: %s).\n", targetCandidatePath, compResult.Reason)
					goto nextSourceFileIteration
				}
			}
		}

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
					} else {
						fmt.Printf("    - Duplicate (Reason: %s) with selected %s. Keeping existing selected file.\n", compResult.Reason, existingSelectedInfo.Path)
						duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: existingSelectedInfo.Path, DiscardedFile: currentSourceInfo.Path, Reason: compResult.Reason})
						goto nextSourceFileIteration
					}
				}
			}

			if shouldReplaceExistingSelected {
				discardedInfo := filesSelectedForCopy[indexOfExistingToReplace]
				duplicates = append(duplicates, pkg.DuplicateInfo{KeptFile: currentSourceInfo.Path, DiscardedFile: discardedInfo.Path, Reason: reasonForReplacement})
				filesSelectedForCopy[indexOfExistingToReplace] = currentSourceInfo
				fmt.Printf("  - Marked for replacement in selection list: %s will replace %s (Reason: %s)\n", currentSourceInfo.Path, discardedInfo.Path, reasonForReplacement)
			} else if !isDuplicateOfSelected {
				filesSelectedForCopy = append(filesSelectedForCopy, currentSourceInfo)
				fmt.Printf("  - Marked for copy: %s (unique or better than target, and unique among selected)\n", currentSourceInfo.Path)
			}
		}
		nextSourceFileIteration:
	}

	pixelHashUnsupported = len(sourceFilesThatUsedFileHash)

	fmt.Printf("\n--- Starting Copying Phase ---\nFound %d files to copy.\n", len(filesSelectedForCopy))
	keptFileSourceToTargetMap := make(map[string]string)

	for _, finalInfoToCopy := range filesSelectedForCopy {
		fmt.Printf("Preparing to copy selected file: %s\n", finalInfoToCopy.Path)
		var photoDateCopy time.Time // Use a different variable name to avoid conflict if needed
		var dateSourceCopy string

		exifDateCopy, dateErrCopy := pkg.GetPhotoCreationDate(finalInfoToCopy.Path)
		if dateErrCopy == nil {
			photoDateCopy = exifDateCopy
			dateSourceCopy = "EXIF"
			fmt.Printf("  - Extracted date (%s) for %s: %s\n", dateSourceCopy, finalInfoToCopy.Path, photoDateCopy.Format("2006-01-02"))
		} else {
			log.Printf("  - Warning: Could not get EXIF date for %s: %v. Using file modification time.\n", finalInfoToCopy.Path, dateErrCopy)
			fileInfoStatCopy, statErrCopy := os.Stat(finalInfoToCopy.Path)
			if statErrCopy != nil {
				log.Printf("    - Error getting file info for %s: %v. Skipping copy.\n", finalInfoToCopy.Path, statErrCopy)
				continue
			}
			photoDateCopy = fileInfoStatCopy.ModTime()
			dateSourceCopy = "FileModTime"
			fmt.Printf("  - Using fallback date (%s) for %s: %s\n", dateSourceCopy, finalInfoToCopy.Path, photoDateCopy.Format("2006-01-02-150405"))
		}

		targetMonthDirCopy, dirErrCopy := pkg.CreateTargetDirectory(targetBaseDir, photoDateCopy)
		if dirErrCopy != nil {
			log.Printf("  - Error creating target directory for %s (date: %s, source: %s): %v. Skipping copy.\n",
				finalInfoToCopy.Path, photoDateCopy.Format("2006-01-02-150405"), dateSourceCopy, dirErrCopy)
			continue
		}

		originalExtensionCopy := filepath.Ext(finalInfoToCopy.Path)
		baseNameCopy := photoDateCopy.In(time.UTC).Format("2006-01-02-150405")

		newFileName := baseNameCopy + originalExtensionCopy
		destPath := filepath.Join(targetMonthDirCopy, newFileName)
		version := 1
		originalDestPathForLog := destPath

		for {
			_, statErr := os.Stat(destPath)
			if os.IsNotExist(statErr) {
				if destPath != originalDestPathForLog && version > 1 {
					fmt.Printf("  - Path %s existed. Using versioned name: %s\n", filepath.Join(targetMonthDirCopy, fmt.Sprintf("%s-%d%s", baseNameCopy, version-1, originalExtensionCopy)), destPath)
				} else if destPath != originalDestPathForLog && version == 1 && strings.Contains(originalDestPathForLog, "-0") {
					// This case is for future proofing if base name could contain "-0"
				}
				break
			} else if statErr != nil {
				log.Printf("  - Error stating file %s: %v. Skipping copy of %s.\n", destPath, statErr, finalInfoToCopy.Path)
				goto nextFileToCopyLoop
			}
			newFileName = fmt.Sprintf("%s-%d%s", baseNameCopy, version, originalExtensionCopy)
			destPath = filepath.Join(targetMonthDirCopy, newFileName)
			version++
		}
		fmt.Printf("  - Preparing to copy %s to: %s\n", finalInfoToCopy.Path, destPath)

		if copyErr := pkg.CopyFile(finalInfoToCopy.Path, destPath); copyErr != nil {
			log.Printf("  - Error copying file %s to %s: %v.\n", finalInfoToCopy.Path, destPath, copyErr)
		} else {
			fmt.Printf("  - Successfully copied %s to %s\n", finalInfoToCopy.Path, destPath)
			copiedFiles++
			keptFileSourceToTargetMap[finalInfoToCopy.Path] = destPath
		}
		nextFileToCopyLoop:
	}

	for i, dup := range duplicates {
		if targetPath, ok := keptFileSourceToTargetMap[dup.KeptFile]; ok {
			duplicates[i].KeptFile = targetPath
		}
	}

	fmt.Println("\n--- Photo Sorting Process Completed ---")
	filesToCopy = len(filesSelectedForCopy)
	if genErr := pkg.GenerateReport(reportFilePath, duplicates, copiedFiles, processedFiles, filesToCopy, pixelHashUnsupported); genErr != nil {
		return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, fmt.Errorf("failed to generate final report: %w", genErr)
	}
	return processedFiles, copiedFiles, filesToCopy, duplicates, pixelHashUnsupported, nil
}

// main is the application entry point. It parses flags and calls runApplicationLogic.
// See docs/technical_details.md for more about CLI and help message.
func main() {
	sourceDirFlag := flag.String("sourceDir", "", "Source directory containing photos to sort (required)")
	targetDirFlag := flag.String("targetDir", "", "Target directory to store sorted photos (required)")
	helpFlg := flag.Bool("help", false, "Show help message and license information")
	flag.Parse()

	if *helpFlg {
		fmt.Println("Usage: photocp -sourceDir <source_directory> -targetDir <target_directory>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
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

	processed, copied, filesToCopyResult, duplicatesResult, pixelHashUnsupportedResult, appErr := runApplicationLogic(sourceDir, targetBaseDir)
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
	log.Printf("Run Summary: Processed: %d, Copied: %d, Selected To Copy: %d, Duplicates Found: %d, Pixel Hash Unsupported (Unique Files): %d\n",
		processed, copied, filesToCopyResult, len(duplicatesResult), pixelHashUnsupportedResult)
}
