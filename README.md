# Photo Sorter

## Overview
Photo Sorter is a command-line tool written in Go to help you organize your photo library. It copies photos from a source directory, sorts them into a new directory structure based on their creation date (YYYY/MM/DD), and identifies duplicate files.

## Features
- **Date-Based Sorting:** Organizes photos into `YYYY/MM/DD` folders based on EXIF creation date, falling back to file modification time if EXIF date is unavailable. Photos will be renamed to the format image-YYYY-MM-DD.<original_extension>
- **Advanced Duplicate Detection:**
  - For supported image types (e.g., JPEG, PNG, GIF), duplicates are primarily identified by comparing the SHA-256 hash of their raw pixel data (ignoring metadata). This helps find visually identical images.
  - For file types where pixel data extraction is not supported (e.g., certain RAW files, non-image files) or if pixel extraction fails, duplicate detection falls back to comparing the SHA-256 hash of the entire file content.
- **Resolution Preference:** When duplicates with matching pixel data are found, the tool attempts to keep the version with the highest image resolution.
- **Reporting:** Generates a `report.txt` in the target directory detailing files processed, copied, duplicates found (including which files were kept/discarded and why), and lists any files for which pixel data could not be extracted for hashing.
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
* `-targetDir`: (Required) The base directory where the sorted photos will be copied. Photos will be organized into `YYYY/MM/DD` subfolders within this directory.

## Duplicate Handling and Report
The tool employs a two-tiered approach for identifying duplicate photos:
- **Pixel-Data Hashing:** For common image formats like JPEG, PNG, and GIF, the tool calculates a SHA-256 hash of the raw pixel data. This allows identification of images that are visually identical, even if their metadata differs.
- **Full File Content Hashing (Fallback):** If pixel data cannot be extracted from a file (e.g., unsupported image format, non-image file, or corrupted image data), the tool falls back to calculating a SHA-256 hash of the entire file content. This hash is then used to find duplicates among other files that were also processed using this fallback method.
The report will indicate if pixel data extraction was not possible for a given file.
If multiple files are found to be duplicates based on their pixel data hash, the tool attempts to keep the version with the highest image resolution. Resolution is determined by decoding common image formats. If resolution cannot be determined or if duplicates are identified by full file hash, the first encountered version is typically kept.
A detailed report named `report.txt` is generated in the root of the target directory. This report lists:
    - A summary of total files scanned, files identified for copying, files successfully copied, and duplicate files found.
    - Specific details for each duplicate pair, indicating which file path was kept, which was discarded, and the reason for the decision (e.g., higher resolution, pixel hash match, full file hash match).
    - A list of any files for which pixel data could not be extracted for hashing.

## Development / Technical Constraints
* Written in Go (version 1.21+).
* Runs on Windows, macOS, and Linux.
* Source and target folders are provided via command-line arguments.
* A GitHub Actions workflow is used to build the project and run tests on pull requests and pushes to the main branch.
* Unit tests are provided for core functionalities and must pass for code changes.

## License
This project is licensed under the BSD 2-Clause "Simplified" License. See the [LICENSE](LICENSE) file for details.
