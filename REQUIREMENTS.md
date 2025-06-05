# Photo Sorter Requirements

This document lists the implemented features and requirements for the Photo Sorter application.

## Core Functionality

-   **Date-Based Sorting:**
    -   Organizes photos into a directory structure of `YYYY/MM/DD`.
    -   Uses the photo's EXIF creation date if available.
    -   Falls back to the file's last modification time if EXIF date is not found or is invalid.

-   **File Renaming:**
    -   Photos copied to the target directory are renamed.
    -   The new filename format is `image-YYYY-MM-DD.<original_extension>` (e.g., `image-2023-10-27.jpg`).

-   **Advanced Duplicate Detection:**
    -   Employs a two-tiered approach to identify duplicate files:
        1.  **Pixel-Data Hashing (Primary):**
            -   For supported image formats (currently JPEG, PNG, GIF), the application calculates a SHA-256 hash of the raw pixel data, ignoring metadata.
            -   This method identifies visually identical images.
        2.  **Full File Content Hashing (Fallback):**
            -   For file types where pixel data extraction is not supported (e.g., certain RAW formats, non-image files) or if pixel data extraction fails for any reason, the application falls back to calculating a SHA-256 hash of the entire file content.
            -   This hash is used to find duplicates among other files that were also processed using this fallback method.

-   **Duplicate Resolution:**
    -   **Resolution Preference (for Pixel-Data Duplicates):** If multiple files are identified as duplicates based on their pixel-data hash, the tool attempts to keep the version with the highest image resolution (calculated as width * height).
    -   **First Encountered (for File-Hash Duplicates or Undetermined Resolution):** If duplicates are identified by full file hash, or if image resolution cannot be determined for pixel-data duplicates, the first version of the file encountered during processing is typically kept.

## Reporting

-   **Report File:**
    -   Generates a text file named `report.txt` in the root of the target directory.
-   **Report Content:**
    -   **Summary:**
        -   Total files scanned.
        -   Number of files identified for copying (unique or better resolution).
        -   Number of files successfully copied.
        -   Total number of duplicate files found and discarded/skipped.
        -   Number of files for which pixel-data hashing was not supported (and therefore used full file content hashing).
    -   **Duplicate Details:**
        -   For each duplicate pair, lists the path of the file that was kept.
        -   Lists the path of the file that was discarded.
        -   Provides a reason for the decision (e.g., "Pixel hash match, higher resolution kept", "File hash match - pixel hashing not supported").
    -   The report will implicitly show files for which pixel data could not be extracted if they appear in duplicate entries with a "file hash match - pixel hashing not supported" reason, or if they are listed in the summary count of such files.

## Technical Requirements

-   **Platform Support:**
    -   Designed to run on Windows, macOS, and Linux.
-   **Command-Line Interface (CLI):**
    -   The application is operated via command-line arguments.
    -   **`-sourceDir`**: (Required) Specifies the source directory containing the photos to be sorted.
    -   **`-targetDir`**: (Required) Specifies the base directory where the sorted photos will be copied.
    -   **`-help`**: Displays usage information, command-line options, and license details.

## Dependencies
- Go (version 1.21 or later) for building.
- `goexif` library (github.com/rwcarlsen/goexif) for EXIF data extraction.
