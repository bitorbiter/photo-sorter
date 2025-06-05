# Photo Sorter

## Overview
Photo Sorter is a command-line tool written in Go to help you organize your photo library. It copies photos from a source directory, sorts them into a new directory structure based on their creation date (YYYY/MM), and identifies duplicate files.

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
This project uses the `goexif` library to extract EXIF data from images.
- **goexif**: [https://github.com/rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)
  - Authors: Robert Carlsen & Contributors
  - License: BSD 2-Clause "Simplified" License

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
* `-sourceDir`: (Required) The directory containing the photos you want to sort. The tool will scan this directory recursively for image files (common formats like JPG, PNG, GIF, and various RAW types are supported for scanning).
* `-targetDir`: (Required) The base directory where the sorted photos will be copied. Photos will be organized into `YYYY/MM` subfolders within this directory.

## Duplicate Handling and Report
The tool uses an enhanced multi-stage approach to efficiently identify duplicate files:

1.  **File Size Comparison:**
    *   The first and quickest check. Files with different sizes are immediately considered non-duplicates. This avoids unnecessary processing for obviously different files.

2.  **EXIF Data Signature (for images):**
    *   If file sizes match and the files are identified as image formats that typically contain EXIF data (e.g., JPEG, some RAW types), the application attempts to generate and compare a signature.
    *   This signature is created from a combination of key EXIF tags, such as `DateTimeOriginal` (creation timestamp), `Make` and `Model` (camera information), and `ImageWidth` and `ImageHeight` (dimensions).
    *   If these generated signatures differ, the files are considered non-duplicates. This step helps differentiate images taken at different times or with different cameras/settings even if their content or size might coincidentally be similar.
    *   This step is skipped if EXIF data is not available, not supported for the file type, or if the files are not images.

3.  **Pixel-Data Hashing (Primary for images):**
    *   If the above checks are inconclusive or passed (e.g., same size, same EXIF signature, or EXIF not applicable), the tool proceeds to pixel-data hashing for supported image formats (like JPEG, PNG, GIF).
    *   A SHA-256 hash of the raw pixel data is calculated, deliberately ignoring metadata. This allows the tool to identify images that are visually identical, even if their metadata (like tags, comments, or modification dates) differs.
    *   If pixel hashes match, the files are considered visual duplicates.

4.  **Full File Content Hashing (Fallback/General):**
    *   This is the final stage of comparison and acts as a fallback. It's used for:
        *   Non-image files.
        *   Image files where pixel-data hashing is not supported or failed (e.g., certain RAW formats, corrupted images).
        *   Image files whose pixel-data hashes matched. In this case, a full file hash acts as a definitive confirmation, ensuring that not just the visual data but the entire file (including all metadata) is identical.
    *   A SHA-256 hash of the entire file content is calculated and compared.

This layered strategy ensures that computationally expensive hashing is only performed when necessary, making the duplicate detection process more efficient.

**Duplicate Resolution:**
-   If multiple files are identified as duplicates based on their **pixel-data hash**, the tool attempts to keep the version with the highest image resolution (calculated as width * height from image dimensions).
-   If duplicates are identified by **EXIF signature mismatch** (this flags them as non-duplicates early), **full file hash**, or if image resolution cannot be determined for pixel-data duplicates, the first version of the file encountered and selected for copying is typically kept.

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
