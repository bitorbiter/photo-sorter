package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/user/photo-sorter/cmd/photocp/lib"
)

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
	processed, copied, _, duplicates, pixelHashUnsupported, appErr := photocp.RunApplicationLogic(sourceDir, targetBaseDir, verbose)
	if appErr != nil {
		log.Fatalf("Application Error: %v", appErr)
	}
	fmt.Printf("Run Summary: Processed: %d, Copied: %d, Duplicates Found: %d, Pixel Hash Unsupported (Unique Files): %d\n",
		processed, copied, len(duplicates), pixelHashUnsupported)
}
