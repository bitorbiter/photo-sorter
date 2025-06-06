# Requirements

This project relies on the following Go modules:

## Direct Dependencies

-   **goexif**: `github.com/rwcarlsen/goexif`
    -   **Purpose**: Used to extract EXIF data from image files.
    -   **License**: BSD 2-Clause "Simplified" License
    -   **Copyright**: Copyright (c) 2012, Robert Carlsen & Contributors
    -   **Source**: [github.com/rwcarlsen/goexif](https://github.com/rwcarlsen/goexif)

-   **heif-go**: `github.com/vegidio/heif-go`
    -   **Purpose**: HEIF/HEIC image decoding. Provides support for `.heic` and `.heif` files.
    -   **License**: MIT License
    -   **Source**: [github.com/vegidio/heif-go](https://github.com/vegidio/heif-go)

## Indirect Dependencies

These libraries are included by the direct dependencies or by the testing framework. While not directly imported by the application's core logic, they are part of the overall project build and test environment.

-   **go-spew**: `github.com/davecgh/go-spew`
    -   **Purpose**: Used for deep pretty printing of Go data structures (often for debugging, likely pulled in by a testing dependency).
    -   **License**: ISC License
    -   **Copyright**: Copyright (c) 2012-2016 Dave Collins <dave@davec.name>
    -   **Source**: [github.com/davecgh/go-spew](https://github.com/davecgh/go-spew)

-   **go-difflib**: `github.com/pmezard/go-difflib`
    -   **Purpose**: Provides data comparison utilities (likely pulled in by a testing dependency for diffing text).
    -   **License**: BSD 3-Clause License
    -   **Copyright**: Copyright (c) 2013, Patrick Mezard
    -   **Source**: [github.com/pmezard/go-difflib](https://github.com/pmezard/go-difflib)

-   **testify**: `github.com/stretchr/testify`
    -   **Purpose**: A set of packages that provide common assertions and tools for Go tests.
    -   **License**: MIT License
    -   **Copyright**: Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors
    -   **Source**: [github.com/stretchr/testify](https://github.com/stretchr/testify)

-   **yaml.v3**: `gopkg.in/yaml.v3` (Source from README: `github.com/go-yaml/yaml/tree/v3`)
    -   **Purpose**: YAML support for Go (likely pulled in by a testing or utility dependency).
    -   **License**: MIT License and Apache License 2.0
    -   **Copyright**: Copyright (c) 2006-2010 Kirill Simonov (MIT portions), Copyright (c) 2011-2019 Canonical Ltd (Apache portions)
    -   **Source**: [github.com/go-yaml/yaml/tree/v3](https://github.com/go-yaml/yaml/tree/v3)

Please refer to the respective repositories for full license texts.
