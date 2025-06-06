# Photo Sorter

## Overview
Photo Sorter is a command-line tool written in Go to help you organize your photo library. It scans photos from a source directory, identifies unique files or preferred versions by detecting and resolving duplicates, and then copies these selected files into a new, sorted directory structure based on their creation date (YYYY/MM).

## Features
- **Date-Based Sorting:** Organizes photos into `YYYY/MM` folders based on EXIF creation date, falling back to file modification time if EXIF date is unavailable. Photos will be renamed to the format `YYYY-MM-DD-HHMMSS(-v).<original_extension>` (e.g., `2023-10-27-153000.jpg` or `2023-10-27-153000-1.jpg` if a conflict occurs).
- **Advanced Duplicate Detection:** Employs an efficient multi-stage process:
  1.  **File Size Check:** Quick initial comparison; different sizes mean non-duplicates.
  2.  **EXIF Signature (Images):** For images of the same size, a signature from key EXIF tags (e.g., creation date, camera model, image dimensions) is compared. Mismatches indicate non-duplicates.
  3.  **Pixel-Data Hashing (Images):** For images still considered potential duplicates, their visual content is compared using a SHA-256 hash of raw pixel data (ignoring metadata).
  4.  **Full File Content Hashing:** For non-image files, or as a final check for images if previous stages are inconclusive (e.g., EXIF missing, pixel hashes match), the entire file content is hashed using SHA-256.
- **Resolution Preference:** When visually identical image duplicates (matched by pixel data) are found, the tool attempts to keep the version with the highest image resolution.
- **Reporting:** Generates a `report.txt` in the target directory detailing files processed, copied, duplicates found (including which files were kept/discarded and why, reflecting the stage of detection), and lists any files for which pixel data could not be extracted for hashing.
- **Cross-Platform:** Designed to run on Windows, macOS, and Linux.

## Prerequisites
- Go (version 1.21 or later) is required to build the tool from source.

## Dependencies

This project relies on the following Go modules:

### Direct Dependencies
- **goexif**: `github.com/rwcarlsen/goexif`
  - Purpose: Used to extract EXIF data from image files.
  - License: BSD 2-Clause "Simplified" License
  - Copyright: Copyright (c) 2012, Robert Carlsen & Contributors
- **heif-go**: `github.com/vegidio/heif-go`
  - Purpose: HEIF/HEIC image decoding. Provides support for `.heic` and `.heif` files.
  - License: MIT License

### Indirect Dependencies
These libraries are included by the direct dependencies or by the testing framework. While not directly imported by the application's core logic, they are part of the overall project build and test environment.
- **go-spew**: `github.com/davecgh/go-spew`
  - Purpose: Used for deep pretty printing of Go data structures (often for debugging, likely pulled in by a testing dependency).
  - License: ISC License
  - Copyright: Copyright (c) 2012-2016 Dave Collins <dave@davec.name>
- **go-difflib**: `github.com/pmezard/go-difflib`
  - Purpose: Provides data comparison utilities (likely pulled in by a testing dependency for diffing text).
  - License: BSD 3-Clause License
  - Copyright: Copyright (c) 2013, Patrick Mezard
- **testify**: `github.com/stretchr/testify`
  - Purpose: A set of packages that provide common assertions and tools for Go tests.
  - License: MIT License
  - Copyright: Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors
- **yaml.v3**: `gopkg.in/yaml.v3` (Source: `github.com/go-yaml/yaml/tree/v3`)
  - Purpose: YAML support for Go (likely pulled in by a testing or utility dependency).
  - License: MIT License and Apache License 2.0
  - Copyright: Copyright (c) 2006-2010 Kirill Simonov (MIT portions), Copyright (c) 2011-2019 Canonical Ltd (Apache portions)

Please refer to the respective repositories for full license texts.

## Building from Source
1. Clone the repository:
   ```bash
   git clone https://github.com/user/photo-sorter.git
   cd photo-sorter
   ```
2. Build the executable:
   ```bash
   go build -v ./cmd/photocp/...
   ```
This will create a `photocp` (or `photocp.exe` on Windows) executable in the current directory.

## Running Tests
To run all the package tests, use the following command:
```bash
go test -v ./...
```
This command will execute all tests found in the project and provide verbose output.

## Usage
Run the tool from the command line, specifying the source and target directories:

```bash
./photo-sorter -sourceDir /path/to/your/photos -targetDir /path/to/sorted/output
```
Or on Windows:
```bash
.\photo-sorter.exe -sourceDir C:\path\to\your\photos -targetDir C:\path\to\sorted\output
```

**Command-line Flags:**
* `-sourceDir`: (Required) The directory containing the photos you want to sort. The tool will scan this directory recursively for image files (common formats like JPG, PNG, GIF, HEIF/HEVC (e.g., ".heic, .heif"), and various RAW types are supported for scanning).
* `-targetDir`: (Required) The base directory where the sorted photos will be copied. Photos will be organized into `YYYY/MM` subfolders within this directory.

## Duplicate Handling and Report
For each source file, its exact target path (based on date and original extension) is determined. The tool first checks if a file already exists at this specific target path.
- If the target path is empty, the source file is copied directly to this path.
- If a file *already exists* at the target path, the source file is then compared *only* against this single, existing target file using the multi-stage process detailed below (EXIF, Pixel Hash, File Hash).
This optimized approach avoids unnecessary comparisons against a list of other source files, improving efficiency.

The multi-stage comparison process is as follows:

**For Image-vs-Image Comparisons:**
If both files are identified as image types (e.g., based on extension like .jpg, .png, .gif, .heic, .heif):
1.  **EXIF Data Signature:** An attempt is made to generate a signature from key EXIF tags (e.g., `DateTimeOriginal`, `Make`, `Model`, `ImageWidth`, `ImageHeight`). If these signatures differ, the files are considered non-duplicates. This step helps differentiate images taken at different times or with different camera settings. For HEIF/HEVC (.heic, .heif) files, EXIF data extraction is currently limited, and the application will primarily rely on file modification time for date-based sorting for these formats.
2.  **Pixel-Data Hashing:** If EXIF signatures match, are absent in one or both files, or if this check is otherwise inconclusive, the tool calculates a SHA-256 hash of the raw pixel data for supported image formats (e.g., JPEG, PNG, GIF, HEIC, HEIF), deliberately ignoring all metadata.
    *   If these pixel-data hashes match, the images are considered duplicates at this stage (i.e., their image sensor data is identical).
    *   **Important Note on Pixel-Data Hashing:** This method identifies images with *bit-for-bit identical pixel data*. It is very effective for finding exact duplicates where only metadata might have changed. However, it will **not** identify images as duplicates if they have been resized, re-encoded (e.g., saving a PNG as a JPG), or undergone even minor visual edits, as these operations alter the raw pixel data.
3.  **Full File Content Hashing (Fallback for Images):** If pixel-data hashing is unsupported for one or both image types, or if an error occurs that prevents pixel hashing (and it's not due to one file being unsupported after the other was successfully hashed or also unsupported), the tool falls back to calculating a SHA-256 hash of the entire file content. If these full file hashes match, they are considered duplicates.

**For Non-Image or Mixed-Type Comparisons:**
If one or both files are not identified as image types:
1.  **File Size Comparison:** The files are first compared by size. If their sizes differ, they are immediately considered non-duplicates.
2.  **Full File Content Hashing:** If the file sizes are identical, the tool calculates a SHA-256 hash of the entire file content. If these hashes match, the files are considered duplicates.

This layered strategy ensures that computationally expensive hashing is only performed when necessary.

**Duplicate Resolution:**
-   If two images are identified as duplicates based on their **pixel-data hash** (meaning their raw pixel data and dimensions are identical), the tool aims to keep the best quality version. If `main.go` determines the source is better (e.g., due to more complete metadata or if one file's resolution metadata was previously misread, though typically pixel-identical files will have identical resolutions), the source might replace the target.
-   For other duplicate types (like **file hash match** where content is identical but they aren't images, or for images where pixel hashing isn't conclusive due to errors or unsupported formats), the existing target file is preserved if it's identical to the source. If the source file is different but maps to the same target name (e.g. different content but same date/time), the existing target file is also preserved and the source file is typically discarded to prevent accidental data loss, as versioning of different files at the same exact path is not currently implemented.

**Reporting:**
A detailed report named `report.txt` is generated in the root of the target directory. This report lists:
    - A summary of total files scanned, files identified for copying, files successfully copied, and duplicate files found.
    - Specific details for each duplicate pair, indicating which file path was kept, which was discarded, and the reason for the decision (e.g., "size_mismatch", "exif_mismatch", "pixel_hash_match (higher resolution kept)", "file_hash_match").
    - An approximate count of files for which pixel-data hashing was not supported and therefore used full file content hashing (if applicable).

## Development / Technical Constraints
* Written in Go (version 1.21+).
* Runs on Windows, macOS, and Linux.
* Source and target folders are provided via command-line arguments.
* A GitHub Actions workflow is used to build the project and run tests on pull requests and pushes to the main branch.
* Unit tests are provided for core functionalities and must pass for code changes.

## License
This project is licensed under the BSD 2-Clause "Simplified" License. See the [LICENSE](LICENSE) file for details.
