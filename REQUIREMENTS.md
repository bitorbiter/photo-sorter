# Photo Sorter Requirements

This document lists the implemented features and requirements for the Photo Sorter application.

## Core Functionality

-   **REQ-CF-DS-01:** **Date-Based Sorting:**
    -   **REQ-CF-DS-02:** Organizes photos into a directory structure of `YYYY/MM`.
    -   **REQ-CF-DS-03:** Uses the photo's EXIF creation date if available.
    -   **REQ-CF-DS-04:** Falls back to the file's last modification time if EXIF date is not found or is invalid.

-   **REQ-CF-FR-01:** **File Renaming:**
    -   **REQ-CF-FR-02:** Photos copied to the target directory are renamed.
    -   **REQ-CF-FR-03:** The new filename format is `YYYY-MM-DD-HHMMSS(-v).<original_extension>` (e.g., `2023-10-27-153000.jpg` or `2023-10-27-153000-1.jpg`).

-   **REQ-CF-ADD-01:** **Advanced Duplicate Detection:**
    -   **REQ-CF-ADD-02:** Employs a two-tiered approach to identify duplicate files:
        1.  **REQ-CF-ADD-03:** **Pixel-Data Hashing (Primary):**
            -   **REQ-CF-ADD-04:** For supported image formats (currently JPEG, PNG, GIF), the application calculates a SHA-256 hash of the raw pixel data, ignoring metadata.
            -   **REQ-CF-ADD-05:** This method identifies visually identical images.
        2.  **REQ-CF-ADD-06:** **Full File Content Hashing (Fallback):**
            -   **REQ-CF-ADD-07:** For file types where pixel data extraction is not supported (e.g., certain RAW formats, non-image files) or if pixel data extraction fails for any reason, the application falls back to calculating a SHA-256 hash of the entire file content.
            -   **REQ-CF-ADD-08:** This hash is used to find duplicates among other files that were also processed using this fallback method.

-   **REQ-CF-DR-01:** **Duplicate Resolution:**
    -   **REQ-CF-DR-02:** **Resolution Preference (for Pixel-Data Duplicates):** If multiple files are identified as duplicates based on their pixel-data hash, the tool attempts to keep the version with the highest image resolution (calculated as width * height).
    -   **REQ-CF-DR-03:** **First Encountered (for File-Hash Duplicates or Undetermined Resolution):** If duplicates are identified by full file hash, or if image resolution cannot be determined for pixel-data duplicates, the first version of the file encountered during processing is typically kept.

## Reporting

-   **REQ-RP-RF-01:** **Report File:**
    -   **REQ-RP-RF-02:** Generates a text file named `report.txt` in the root of the target directory.
-   **REQ-RP-RC-01:** **Report Content:**
    -   **REQ-RP-RC-02:** **Summary:**
        -   **REQ-RP-RC-03:** Total files scanned.
        -   **REQ-RP-RC-04:** Number of files identified for copying (unique or better resolution).
        -   **REQ-RP-RC-05:** Number of files successfully copied.
        -   **REQ-RP-RC-06:** Total number of duplicate files found and discarded/skipped.
        -   **REQ-RP-RC-07:** Number of files for which pixel-data hashing was not supported (and therefore used full file content hashing).
    -   **REQ-RP-RC-08:** **Duplicate Details:**
        -   **REQ-RP-RC-09:** For each duplicate pair, lists the path of the file that was kept.
        -   **REQ-RP-RC-10:** Lists the path of the file that was discarded.
        -   **REQ-RP-RC-11:** Provides a reason for the decision (e.g., "Pixel hash match, higher resolution kept", "File hash match - pixel hashing not supported").
    -   **REQ-RP-RC-12:** The report will implicitly show files for which pixel data could not be extracted if they appear in duplicate entries with a "file hash match - pixel hashing not supported" reason, or if they are listed in the summary count of such files.

## Technical Requirements

-   **REQ-TR-PS-01:** **Platform Support:**
    -   **REQ-TR-PS-02:** Designed to run on Windows, macOS, and Linux.
-   **REQ-TR-CLI-01:** **Command-Line Interface (CLI):**
    -   **REQ-TR-CLI-02:** The application is operated via command-line arguments.
    -   **REQ-TR-CLI-03:** **`-sourceDir`**: (Required) Specifies the source directory containing the photos to be sorted.
    -   **REQ-TR-CLI-04:** **`-targetDir`**: (Required) Specifies the base directory where the sorted photos will be copied.
    -   **REQ-TR-CLI-05:** **`-help`**: Displays usage information, command-line options, and license details.

## Dependencies
- **REQ-DP-01:** Go (version 1.21 or later) for building.
- **REQ-DP-02:** `goexif` library (github.com/rwcarlsen/goexif) for EXIF data extraction.
