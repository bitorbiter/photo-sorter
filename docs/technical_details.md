# Technical Details of Photo Sorter

This document provides more in-depth explanations of the internal workings, algorithms, and design choices for the Photo Sorter application.

## Duplicate Detection Algorithm (`AreFilesPotentiallyDuplicate`)

The primary function responsible for comparing two files is `AreFilesPotentiallyDuplicate`. It employs a multi-stage algorithm designed to be efficient by using cheaper comparisons first.

**Initial Checks:**
1.  **Target File Existence**: The function first checks if the second file path (`filePath2`, typically the target or existing file) exists. If not, it returns immediately with `ReasonTargetNotFound`.
2.  **Zero-Byte Files**: If both files are zero bytes in size, they are considered duplicates with `ReasonFileHashMatch` (as their "content" is identically empty). This check is performed early. If one is zero bytes and the other is not, they will be distinguished by the size check in the non-image path, or proceed to hashing in the image path (where hashes will differ).

**Main Comparison Logic:**
The comparison path diverges based on whether both files are identified as image types (using `pkg.IsImageExtension` based on file extensions).

### Image vs. Image Path
If both files are determined to be image types:
1.  **EXIF Data Signature (`HashTypeExif`)**:
    *   An attempt is made to generate a signature from key EXIF tags for both images. These tags typically include `DateTimeOriginal`, `Make`, `Model`, `ImageWidth`, and `ImageHeight`. (Consideration: More tags like `LensModel`, `FNumber`, `ExposureTime`, `ISOSpeedRatings` could be added for finer granularity if needed).
    *   The "signature" is a string formed by concatenating the values of these tags. It's not a cryptographic hash.
    *   If both images have valid EXIF signatures and these signatures differ, the files are considered non-duplicates (`ReasonExifMismatch`).
    *   If EXIF data is missing from one or both files (`ErrNoExif`), or if an actual error occurs during EXIF parsing for one file that isn't simply `ErrNoExif`, the EXIF comparison is deemed inconclusive, and the process moves to the next stage. Warnings may be logged for actual parsing errors.

2.  **Pixel-Data Hashing (`HashTypePixel`)**:
    *   This stage is reached if the EXIF comparison was conclusive (signatures matched) or inconclusive (e.g., missing EXIF).
    *   A SHA-256 hash of the raw pixel data is calculated for each image. This process decodes the image and then hashes the sequence of pixel values (e.g., RGBA data for each pixel).
    *   **Important Limitation**: This method identifies images with bit-for-bit identical pixel data. This means images must typically have the same dimensions and color profile for their pixel hashes to match. It will **not** identify visually similar images that have been resized, reformatted (e.g., JPG vs. PNG with different compression), or had minor edits, as these operations alter the raw pixel data.
    *   If pixel hashes are successfully generated for both images:
        *   If hashes match: Files are duplicates (`ReasonPixelHashMatch`).
        *   If hashes differ: Files are non-duplicates (`ReasonPixelHashMismatch`).
    *   **Fallback Conditions**:
        *   If pixel hashing is unsupported for the first image (e.g., `ErrUnsupportedForPixelHashing` due to an unknown image format or corrupted image data that `image.Decode` cannot handle), the process falls back to Full File Content Hashing for both files.
        *   If pixel hashing succeeds for the first image but is unsupported for the second, it also falls back to Full File Content Hashing for both.
        *   If pixel hashing succeeds for the first image but encounters a critical error (not "unsupported") for the second, an error is returned.
        *   If pixel hashing for the first image encounters a critical error, an error is returned.

3.  **Full File Content Hashing (Fallback for Images - `HashTypeFile`)**:
    *   This is used if the pixel hashing stage determined a fallback was necessary (e.g., due to unsupported image types for pixel analysis).
    *   A SHA-256 hash of the entire file content (including all metadata and the raw file structure) is calculated for both files.
    *   If these full file hashes match, they are duplicates (`ReasonFileHashMatch`). Otherwise, they are non-duplicates (`ReasonFileHashMismatch`).

### Non-Image or Mixed-Type Path
If one or both files are not identified as image types, or if the image path fell through from pixel hashing without a conclusive pixel hash comparison (e.g., one was unsupported, the other had an error that wasn't "unsupported"):
1.  **File Size Check**:
    *   The sizes of both files are compared.
    *   If the file sizes differ, they are immediately considered non-duplicates (`ReasonSizeMismatch`). This check is only primary when not on the image-both path or when pixel hashing was not fully attempted for images.
2.  **Full File Content Hashing (`HashTypeFile`)**:
    *   If the file sizes are identical, the tool calculates a SHA-256 hash of the entire file content for both files.
    *   If these hashes match, they are duplicates (`ReasonFileHashMatch`). Otherwise, they are non-duplicates (`ReasonFileHashMismatch`).

This structured approach ensures efficiency by using less computationally intensive checks first and reserving full file hashing for when it's essential.

## Comparison Result Reasons (`ReasonXXX` Constants)

The `Reason` field in `ComparisonResult` explains the outcome of the duplicate check:
-   `ReasonSizeMismatch`: Files have different sizes.
-   `ReasonExifMismatch`: Both are images and their EXIF signatures differ.
-   `ReasonPixelHashMatch`: Both are images and their raw pixel data hashes match.
-   `ReasonPixelHashMismatch`: Both are images and their raw pixel data hashes differ.
-   `ReasonFileHashMatch`: Files have identical full content hashes.
-   `ReasonFileHashMismatch`: Files have different full content hashes.
-   `ReasonError`: An unexpected error occurred during comparison (e.g., file I/O error).
-   `ReasonNotCompared`: Comparison did not proceed to a definitive stage, or a stage was inconclusive (e.g., EXIF data missing from one or both files, then pixel hashes also didn't match). This often serves as an initial state or an intermediate state before a more definitive reason (like pixel hash mismatch) is found.
-   `ReasonTargetNotFound`: The specified target file for comparison does not exist.
-   `ReasonPixelHashNotAttempted`: Used internally when pixel hashing is skipped (e.g., fallback from image path to general file path before full file hash).

## Hash Types (`HashTypeXXX` Constants)

The `HashType` field in `ComparisonResult` indicates the primary comparison method that led to the result or was last attempted:
-   `HashTypeExif`: EXIF signature comparison. Note: This is a generated signature string, not a cryptographic hash.
-   `HashTypePixel`: SHA-256 hash of raw pixel data.
-   `HashTypeFile`: SHA-256 hash of the full file content.

## Main Application Logic (`runApplicationLogic`)

The `runApplicationLogic` function in `cmd/photocp/main.go` orchestrates the entire photo sorting process.

**Initialization Phase:**
1.  **Report File Path**: Determines the path for `report.txt`.
2.  **Counters and Data Structures**: Initializes counters for processed files, copied files, etc., and slicers for storing files selected for copy and duplicate information. A map `sourceFilesThatUsedFileHash` is used to track unique source image files that ended up using full file hash if pixel hashing failed or was unsupported, for accurate reporting of `pixelHashUnsupported`.
3.  **Target Directory Validation**: Ensures the main target directory (`targetBaseDir`) exists, creating it if necessary. This is crucial before any processing begins.

**Scanning Phase:**
1.  **Scan Source Directory**: Calls `pkg.ScanSourceDirectory` to find all processable image files in the source directory.
2.  **Handle Scan Errors**: If `ScanSourceDirectory` returns an error, a warning is logged. If no files could be read at all (e.g., critical error accessing the source directory), the application may terminate early.
3.  **Empty Source Handling**: If no image files are found, a report is still generated (mostly empty) and the application exits gracefully.

**Main Processing Loop (Iterating Through Source Files):**
For each source file identified:
1.  **Date Determination**:
    *   Attempts to get the creation date from EXIF data using `pkg.GetPhotoCreationDate`.
    *   If EXIF date extraction fails (e.g., no EXIF data, or corrupted EXIF), it falls back to using the file's last modification time as the `photoDate`.
2.  **Target Path Generation**: Determines the potential target month directory (e.g., `targetBaseDir/YYYY/MM`) using `pkg.CreateTargetDirectory` (which also creates it if it doesn't exist). The base name for the new file (e.g., `YYYY-MM-DD-HHMMSS`) is also determined.
3.  **Image Resolution**: Attempts to get the image resolution using `pkg.GetImageResolution`. If this fails, it logs a warning and proceeds with 0x0 resolution.
4.  **Duplicate Checking - Phase 1 (Against Target Directory)**:
    *   The function `pkg.FindPotentialTargetConflicts` is called to find any files in the `potentialTargetMonthDir` that could be versions of the current source file (based on the calculated date-based name).
    *   The current source file is compared against each of these `existingTargetCandidates` using `pkg.AreFilesPotentiallyDuplicate`.
    *   **If a duplicate is found with a target file**:
        *   If it's a `ReasonPixelHashMatch`: The resolutions are compared.
            *   If the source file has better resolution, it's marked with `sourceFileIsBetterDuplicate = true`. A duplicate entry is recorded noting that the target file will be superseded (conceptually, no actual deletion occurs at this stage). The loop over target candidates breaks as a decision is made.
            *   If the target file has better or equal resolution, the source file is discarded (duplicate entry recorded), and processing for this source file stops.
        *   If it's any other type of duplicate (e.g., `ReasonFileHashMatch`), the source file is discarded (duplicate entry recorded), and processing stops for this source file.
    *   If any comparison results in `compResult.HashType == pkg.HashTypeFile` for an image file (meaning pixel hash failed or was skipped, and file hash was used), the source file is added to the `sourceFilesThatUsedFileHash` map for accurate `pixelHashUnsupported` reporting.
5.  **Duplicate Checking - Phase 2 (Against Selected-for-Copy Source Files)**:
    *   This phase is executed only if the source file was *not* found to be a non-better duplicate of an existing target file (i.e., `!isDuplicateOfTargetFile || sourceFileIsBetterDuplicate`).
    *   The current source file is compared against all files already in the `filesSelectedForCopy` list using `pkg.AreFilesPotentiallyDuplicate`.
    *   **If a duplicate is found with an already selected source file**:
        *   If it's a `ReasonPixelHashMatch`: Resolutions are compared.
            *   If the current source file is better, it replaces the existing file in `filesSelectedForCopy` (duplicate entry recorded).
            *   If the existing selected file is better or same resolution, the current source file is discarded (duplicate entry recorded), and processing stops.
        *   If it's any other type of duplicate, the current source file is discarded (duplicate entry recorded), and processing stops.
    *   Again, `sourceFilesThatUsedFileHash` is updated if file hashing was used for an image.
6.  **Selection for Copy**:
    *   If, after all checks, the file is not a duplicate of anything, or it's a better version of a previously selected source file, it's either added to `filesSelectedForCopy` or replaces an existing entry.
    *   If `sourceFileIsBetterDuplicate` was true (meaning it's better than an existing target file), it will proceed through Phase 2 and potentially be added to `filesSelectedForCopy`.

**Copying Phase:**
1.  Iterates through `filesSelectedForCopy`.
2.  For each file:
    *   Re-determines `photoDate` and `dateSource` (EXIF or FileModTime).
    *   Creates the final `targetMonthDir` if it doesn't exist.
    *   Constructs the new filename `YYYY-MM-DD-HHMMSS.<original_extension>`.
    *   **Versioning**: Checks if a file with this name already exists in the target. If so, it appends a version number (e.g., `-1`, `-2`) until a unique name is found. This handles genuine name collisions and cases where a `sourceFileIsBetterDuplicate` is copied when a lower-res version existed.
    *   Copies the file using `pkg.CopyFile`.
    *   On successful copy, the `copiedFiles` counter is incremented.
    *   A map `keptFileSourceToTargetMap` is maintained to track the final target path of any source file that was kept and copied. This is used to update the `KeptFile` path in the `duplicates` report for accuracy.

**Final Reporting Phase:**
1.  **`pixelHashUnsupported` Count**: The final count is derived from the size of the `sourceFilesThatUsedFileHash` map.
2.  **Generate Report**: Calls `pkg.GenerateReport` with all collected data to create `report.txt`.

## Command-Line Interface (`main` function)

The `main` function handles command-line argument parsing, validation, and invoking the core application logic.

**Flags:**
-   `-sourceDir` (string, required): Specifies the source directory.
-   `-targetDir` (string, required): Specifies the target directory.
-   `-help` (bool, optional): Displays usage information.

**Help Message (`-help`):**
The help message is structured to provide:
1.  Basic usage syntax.
2.  A list of all available command-line options with their descriptions (via `flag.PrintDefaults()`).
3.  License information for the application itself.
4.  Detailed dependency information, including:
    *   Direct dependencies (e.g., `goexif`).
    *   Indirect dependencies (pulled in by direct ones or testing frameworks like `testify`, `go-spew`, etc.).
    *   For each, its purpose, license type, and copyright information are listed to ensure compliance and transparency.

**Initial Validations:**
-   Ensures required flags (`-sourceDir`, `-targetDir`) are provided.
-   Validates that the `sourceDir` exists and is a directory using `os.Stat`.

Finally, it calls `runApplicationLogic` and logs any application errors or a summary of the run.

## Filesystem Operations (`pkg/filesystem.go`)

This package handles interactions with the file system, including scanning directories, creating target paths, and extracting date information from files.

### Image File Identification (`imageExtensions`, `IsImageExtension`)
- A predefined map `imageExtensions` stores common image file extensions (e.g., `.jpg`, `.png`, `.gif`, various RAW types like `.cr2`, `.nef`).
- The `IsImageExtension(filePath string) bool` function checks if a given file's extension (case-insensitively) exists in this map.
- **Note**: The list of extensions can be expanded by modifying the `imageExtensions` map directly in the source code if support for more image types is needed for initial scanning.

### Directory Scanning (`ScanSourceDirectory`)
- This function recursively walks the specified source directory.
- It identifies files based on the `imageExtensions` map.
- If errors occur while accessing specific paths during the walk (e.g., permission issues), a warning is printed, and the function attempts to continue processing other files.
- If the source directory itself doesn't exist or isn't a directory, an error is returned.
- It returns an empty slice if no image files are found, rather than a `nil` slice.

### Target Directory Creation (`CreateTargetDirectory`)
- Creates a directory structure of `targetBaseDir/YYYY/MM` based on the provided date.
- The month format `01` is used to ensure two digits (e.g., "03" for March).
- It uses `os.MkdirAll` to create parent directories as needed and doesn't return an error if the directory already exists.

### Photo Creation Date Extraction (`GetPhotoCreationDate`, `parseExifDateTime`)
- **`GetPhotoCreationDate`**:
    -   Attempts to open and decode EXIF data from the photo.
    -   If EXIF decoding fails (e.g., file is not an image, no EXIF data, corrupted EXIF), it returns an error. The function aims to broadly categorize EXIF decoding issues rather than distinguishing specific EXIF library errors like `io.EOF` or `exif: failed to find exif intro marker` at this level, treating any such failure as "EXIF data not usable" for date extraction.
    -   It prioritizes the `DateTimeOriginal` EXIF tag.
    *   If `DateTimeOriginal` is not found, it falls back to the `DateTimeDigitized` tag.
    *   If neither suitable tag is found, it returns `ErrNoExifDate`.
- **`parseExifDateTime`**:
    *   This helper parses the string value obtained from an EXIF date tag.
    *   It primarily expects the EXIF standard format: `"YYYY:MM:DD HH:MM:SS"`.
    *   It handles potential nuances like null terminators in the tag string by using `tag.StringVal()`.
    *   If parsing the full datetime format fails, it attempts a fallback to a date-only format (`"YYYY:MM:DD"`) because some cameras might only store the date part.
    *   It's aware that EXIF date strings can sometimes include timezone information, but the current parsing logic focuses on the common local time representation.

### Finding Potential Target Conflicts (`FindPotentialTargetConflicts`)
- This function lists existing files in a target month directory that could conflict with a new file being generated with a base name (e.g., `YYYY-MM-DD-HHMMSS`).
- **Filename Matching Logic**:
    - The goal is to match files like `baseName.ext`, `baseName-1.ext`, `baseName-123.ext`.
    - It does not use regular expressions for this due to the controlled nature of the inputs and to avoid escaping complexities. Direct string prefix, suffix, and middle-part checking is used.
    - The comparison is case-insensitive for the extension part.
    - It correctly identifies the base part and the versioning part (e.g., `-1`, `-123`).
    - It avoids matching incorrectly formatted versions like `baseName-abc.ext` (non-digit version) or `baseName--1.ext` (double hyphen).
- If the target month directory doesn't exist, it correctly returns an empty list of conflicts without an error.
