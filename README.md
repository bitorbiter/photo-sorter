# Photo Sorter

## Overview
Photo Sorter is a command-line tool written in Go to help you organize your photo library. It copies photos from a source directory, sorts them into a new directory structure based on their creation date (YYYY/MM/DD), and identifies duplicate files.

## Features
- **Date-Based Sorting:** Organizes photos into `YYYY/MM/DD` folders based on EXIF creation date, falling back to file modification time if EXIF date is unavailable.
- **Duplicate Detection:** Identifies duplicate photos based on file content (SHA-256 hash).
- **Resolution Preference:** When duplicates are found, the tool attempts to keep the version with the highest image resolution.
- **Reporting:** Generates a `report.txt` in the target directory detailing files processed, copied, and duplicates found (including which files were kept/discarded and why).
- **Cross-Platform:** Designed to run on Windows, macOS, and Linux.

## Prerequisites
- Go (version 1.21 or later) is required to build the tool from source.

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
- The tool identifies duplicate photos based on their content by calculating a SHA-256 hash for each file.
- If multiple files share the same hash (i.e., they are content-identical duplicates), the tool attempts to keep the version with the highest image resolution. Resolution is determined by decoding common image formats (JPEG, PNG, GIF). If resolution cannot be determined for one or both files, the first encountered version is typically kept.
- A detailed report named `report.txt` is generated in the root of the target directory. This report lists:
    - A summary of total files scanned, files identified for copying, files successfully copied, and duplicate files found.
    - Specific details for each duplicate pair, indicating which file path was kept, which was discarded, and the reason for the decision.

## Development / Technical Constraints
* Written in Go (version 1.21+).
* Runs on Windows, macOS, and Linux.
* Source and target folders are provided via command-line arguments.
* A GitHub Actions workflow is used to build the project and run tests on pull requests and pushes to the main branch.
* Unit tests are provided for core functionalities and must pass for code changes.

*(Note: The requirement "If there is a raw version and a jpeg version with the same content, both photos should be copied" from the original README is not currently implemented if "same content" means pixel data. The current duplicate detection is based on file hash, so if a RAW and its derived JPEG have different file contents, they will be treated as distinct files. If they somehow had the same hash but different names/extensions, the current logic would treat them as duplicates and keep one based on resolution if determinable.)*
