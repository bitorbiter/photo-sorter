# photo-sorter
Sorts photos by date and removes duplicates.

## Requirements - Use cases
* copies photos from a source directory (recursive) to target directories
* The target directories are in the structure ./<year>/<month>/<day> e.g. ./2025/02/18
* duplicate photos are not copied but listed in a report txt
* If there are multiple photos with the same content, the one with the best resolution and the most complete exif information is taken. If there is a raw version and a jpeg version with the same content, both photos should be copied.
* If there are multiple versions of a photo with different resolutions, but best resolution should be taken
* photo-sorter can find duplicates even if there are different resolutions of a photo

## Requirements - technical contraints
* written in GO
* runs on Windows, MacOS and Linux
* Source and target folder should be given via command line

