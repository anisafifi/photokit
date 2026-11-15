# PhotoKit

**Photography automation for the command line.** PhotoKit inspects and edits photo metadata, stamps a transparent PNG signature on your images, and runs whole export workflows from reusable YAML presets, in parallel, without ever touching your originals.

```bash
photokit init --creator "Jane Doe"
photokit process ./photos --recursive --preset default
```

```
processed photos/IMG_001.jpg -> export/IMG_001.jpg [watermark+metadata]
processed photos/IMG_002.jpg -> export/IMG_002.jpg [watermark+metadata]

2 file(s): 2 processed, 0 skipped, 0 failed in 900ms
```

## Contents

- [Features](#features)
- [Installation](#installation)
- [Quick start](#quick-start)
- [How PhotoKit works](#how-photokit-works)
- [Command reference](#command-reference)
- [Presets](#presets)
- [Signatures (watermarks)](#signatures-watermarks)
- [Metadata](#metadata)
- [Supported formats](#supported-formats)
- [Configuration](#configuration)
- [Scripting and automation](#scripting-and-automation)
- [Recipes](#recipes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Releasing](#releasing)
- [Limitations and roadmap](#limitations-and-roadmap)

## Features

| | |
|---|---|
| **Non-destructive** | Sources are only ever read. Results are written as copies, through temporary files and atomic renames, so an interrupted run never leaves a half-written photo. |
| **Metadata** | Read and write creator, copyright, captions, keywords, rating, location, GPS, capture date and contact details across **EXIF, IPTC and XMP** at once, plus **any of ExifTool's ~7,000 writable tags** (`--tag`), tag removal and GPS/all stripping, powered by ExifTool. |
| **Signature watermark** | Composite a transparent PNG on JPEG and PNG images: 9 anchor positions, opacity, relative scale, margin. Alpha is preserved and EXIF orientation is respected. |
| **Metadata survives** | Camera, lens, capture time, GPS and copyright are restored onto watermarked images. Thumbnails (which would leak the un-watermarked picture) are not. |
| **Presets** | Describe a workflow once in YAML. Validated before anything runs, unknown fields rejected, CLI flags can override any value. |
| **Batch** | Directories, recursion, include/exclude globs, a bounded worker pool, dry-run, per-file failure isolation and a summary report (table or JSON). |
| **Cross-platform** | Single static binary for Linux, macOS and Windows (amd64 and arm64). |

## Installation

PhotoKit has **one external dependency: [ExifTool](https://exiftool.org) 12 or newer**. It does all the metadata reading and writing. Install it first, then PhotoKit.

### 1. Install ExifTool

| Platform | Command |
|---|---|
| macOS | `brew install exiftool` |
| Debian / Ubuntu | `sudo apt install libimage-exiftool-perl` |
| Fedora | `sudo dnf install perl-Image-ExifTool` |
| Arch | `sudo pacman -S perl-image-exiftool` |
| Windows | `winget install OliverBetz.ExifTool`, or `choco install exiftool`, or `scoop install exiftool` |

Or download it from <https://exiftool.org>. If it lives somewhere unusual, point PhotoKit at it with `--exiftool PATH`, the `PHOTOKIT_EXIFTOOL_PATH` environment variable, or `exiftool.path` in the [config file](#configuration).

### 2. Install PhotoKit

**Download a release (recommended).** Grab the archive for your platform from the project's **Releases** page, verify it against `checksums.txt`, and put the `photokit` binary somewhere on your `PATH`.

| Platform | Archive |
|---|---|
| Linux | `photokit_<version>_linux_amd64.tar.gz` / `..._linux_arm64.tar.gz` |
| macOS | `photokit_<version>_darwin_amd64.tar.gz` (Intel) / `..._darwin_arm64.tar.gz` (Apple silicon) |
| Windows | `photokit_<version>_windows_amd64.zip` / `..._windows_arm64.zip` |

```bash
# Linux / macOS example
tar -xzf photokit_1.0.0_linux_amd64.tar.gz
sudo mv photokit /usr/local/bin/
sha256sum -c --ignore-missing checksums.txt   # optional integrity check
```

On macOS, a downloaded binary may be quarantined by Gatekeeper. Clear it with `xattr -d com.apple.quarantine photokit`.

**Build from source** (Go 1.22 or newer):

```bash
git clone <repository-url> photokit && cd photokit
make build            # produces ./photokit
make install          # or install into $GOBIN
```

### 3. Verify

```bash
photokit version
```

```
photokit 1.0.0 (commit 3f2a1bc, built 2026-11-14T08:00:00Z)
platform linux/amd64, go1.22.5
exiftool 12.76 (/usr/bin/exiftool)
```

If it prints `exiftool: NOT AVAILABLE`, PhotoKit tells you how to install it for your platform.

### Shell completion

```bash
photokit completion bash > /etc/bash_completion.d/photokit      # bash
photokit completion zsh  > "${fpath[1]}/_photokit"              # zsh
photokit completion fish > ~/.config/fish/completions/photokit.fish
photokit completion powershell | Out-String | Invoke-Expression  # PowerShell
```

## Quick start

```bash
# 1. Scaffold a config, a preset and a placeholder signature in the current directory
photokit init --creator "Jane Doe" --copyright "© 2026 Jane Doe"

# 2. Put your own transparent PNG at assets/signature.png (the generated one is just text)

# 3. Look at what you have
photokit info ./photos --recursive

# 4. Preview where the signature lands on one photo
photokit watermark preview ./photos/IMG_001.jpg --image assets/signature.png -o preview.jpg

# 5. Rehearse the whole run. Nothing is written.
photokit process ./photos --recursive --preset default --dry-run

# 6. Do it. Copies appear in ./export, your originals stay untouched
photokit process ./photos --recursive --preset default
```

## How PhotoKit works

**Non-destructive by default.** Every command that changes pictures writes *copies* into an output directory (default `./export`), mirroring the folder structure of your input:

```
photos/                      export/
├── IMG_001.jpg      ──►     ├── IMG_001.jpg
├── IMG_002.jpg              ├── IMG_002.jpg
└── trips/                   └── trips/
    └── IMG_003.jpg              └── IMG_003.jpg
```

| Operation | Source file | Output |
|---|---|---|
| `info`, `metadata show` | read | none |
| `metadata set` | read | modified copy |
| `watermark apply` | read | rendered copy |
| `process` (preset) | read | processed copy |
| `metadata set --in-place` | **modified** | none (only when you ask) |

Safety rules PhotoKit enforces:

- **Always processes from the original.** Running the same command twice never stacks a second watermark: existing outputs are skipped (or, with `--overwrite`, regenerated from the source).
- **Never reads its own exports.** If your output directory lives inside the input directory, it is excluded from discovery. An output directory equal to the input directory is refused.
- **Never overwrites without `--overwrite`**, and never overwrites the source file.
- **Atomic writes.** Work happens in hidden `.photokit-*` temp files next to the destination and is renamed into place on success. A crash or Ctrl-C leaves no partial output.
- **Duplicate output detection.** If two inputs would produce the same output path (for example `a/IMG.jpg` and `b/IMG.jpg` passed separately), the second is reported as failed instead of silently clobbering the first.
- **Symbolic links are never followed** while scanning directories, and hidden files and folders are ignored. (A file you name explicitly on the command line is always considered.)
- **One bad file never stops the batch.** Failures are recorded per file and summarised; the exit code is non-zero if any file failed.

## Command reference

```
photokit
├── init                    Create a starter project
├── version                 Print version and check for ExifTool
├── info                    Show camera details and metadata (--all for every tag)
├── metadata
│   ├── show                Show only copyright / descriptive metadata
│   ├── set                 Write metadata to copies of your photos
│   └── tags                List every tag ExifTool can write
├── watermark
│   ├── apply               Apply a signature to copies of your photos
│   └── preview             Render one image to check placement
├── process                 Run a preset
├── preset
│   ├── list                List presets
│   ├── show                Print a preset with paths resolved
│   └── validate            Check presets for errors
└── completion              Generate shell completions
```

Run `photokit <command> --help` for the full, authoritative flag list.

### Global flags

| Flag | Description |
|---|---|
| `--config FILE` | Config file (default: `./photokit.yaml`, then the user config directory) |
| `--exiftool PATH` | Path to the ExifTool executable (default: search `PATH`) |
| `-v`, `--verbose` | Detailed processing logs on stderr |
| `--version` | Print the version |

### Input and batch flags

These appear only on commands where they make sense.

| Flag | Commands | Description |
|---|---|---|
| `-r`, `--recursive` | all that take inputs | Include nested directories |
| `--include GLOB` | same | Only process files matching the pattern. Repeatable or comma-separated |
| `--exclude GLOB` | same | Skip files matching the pattern |
| `-o`, `--output DIR` | commands that write copies | Output directory (default `./export`) |
| `--suffix TEXT` | `metadata set`, `watermark apply`, `process` | Appended to output names before the extension, e.g. `-signed` gives `IMG_001-signed.jpg` |
| `--overwrite` | commands that write copies | Replace existing outputs |
| `--dry-run` | commands that write copies | Print the plan, write nothing |
| `--jobs N` | all batch commands | Files processed in parallel (default 4, max 64) |
| `--format table\|json` | all batch and inspect commands | Output format |

Globs use `path.Match` syntax and are matched against both the file name and the path relative to the input directory: `--include '*.jpg'`, `--exclude 'drafts/*'`, `--exclude 'IMG_00?.jpg'`. Quote them so your shell doesn't expand them.

### `photokit init`

Creates `photokit.yaml`, `presets/default.yaml` and `assets/signature.png` (a text placeholder). Existing files are never overwritten unless you pass `--force`.

| Flag | Description |
|---|---|
| `--dir DIR` | Directory to initialise (default `.`) |
| `--creator NAME` | Photographer name written into the preset |
| `--copyright TEXT` | Copyright notice (default `© <year> <creator>`) |
| `--force` | Overwrite existing files |

### `photokit info`

Show format, dimensions, camera, lens, capture date and the managed metadata. One file gets a detailed view; several get one row each.

```bash
photokit info photo.jpg
photokit info ./photos --recursive --format json
```

| Flag | Description |
|---|---|
| `--all` | Show **every** tag ExifTool finds, grouped (EXIF, IPTC, XMP, MakerNotes, ICC, File, ...) |
| `--tag PATTERN` | Show only matching tags. Repeatable. `Artist` matches that tag in any group, `XMP-dc:Creator` matches one group, wildcards work (`"XMP-dc:*"`, `"*GPS*"`). Matching is case-insensitive |

```bash
photokit info photo.jpg --all
photokit info photo.jpg --tag Artist --tag "XMP-dc:*" --tag "*GPS*"
photokit info ./photos -r --tag Lens* --format json     # JSON gains a "tags" object keyed Group:Name
```

The `tags` object only appears when you ask for it with `--all` or `--tag`.

Unreadable or corrupt files are listed with an `error:` and make the exit code non-zero. A file whose content doesn't match its extension (for example a text file named `.jpg`) counts as corrupt.

### `photokit metadata show`

Like `info`, but prints only the descriptive and copyright fields. Handy for checking an export.

```bash
photokit metadata show ./export --recursive
photokit metadata show photo.jpg --tag "*Rights*"      # same --all / --tag flags as info
```

Extended fields (headline, rating, location, GPS, contact ...) are shown only when they are set.

### `photokit metadata set`

Writes metadata to copies of your photos (or, with `--in-place`, to the originals).

Named fields are written **consistently to EXIF, IPTC and XMP**. Anything else ExifTool supports is reachable with `--tag`. Only the fields you pass are changed, and at least one is required.

| Flag | Description |
|---|---|
| `--creator` | Creator / photographer |
| `--copyright` | Copyright notice, e.g. `"© 2026 Your Name"` |
| `--rights` | Usage terms, e.g. `"All rights reserved"` |
| `--web-statement` | URL of the license / rights statement |
| `--title`, `--headline` | Title and headline |
| `--description` | Caption / description |
| `--keywords` | Keyword. Repeat the flag or comma-separate |
| `--append-keywords` | Add to existing keywords instead of replacing them |
| `--credit`, `--source`, `--instructions` | IPTC credit line, source, special instructions |
| `--writer` | Caption writer |
| `--owner` | Image owner |
| `--rating N` | Star rating `0`-`5` (`0` is written, not ignored) |
| `--label TEXT` | Colour label, e.g. `Green` |
| `--location`, `--city`, `--state`, `--country` | Where the photo was taken |
| `--country-code CC` | ISO country code, e.g. `BD` (stored upper-case) |
| `--latitude`, `--longitude` | Decimal degrees. South and west are **negative**. Must be given together. `0` is valid |
| `--altitude M` | Metres, negative below sea level. Needs latitude and longitude |
| `--date-taken` | `"2026-05-01"`, `"2026-05-01 10:30:00"`, `"2026-05-01T10:30:00+06:00"` or `2026:05:01 10:30:00`. A zone also sets the EXIF offset tags |
| `--contact-email`, `--contact-phone`, `--contact-url` | Photographer contact details (IPTC Core) |
| `--contact-address`, `--contact-city`, `--contact-region`, `--contact-postal-code`, `--contact-country` | Photographer contact address |
| `--tag NAME=VALUE` | **Any** writable ExifTool tag. Repeatable. Repeating the same name builds a list. An empty value (`--tag NAME=`) deletes the tag |
| `--remove NAME` | Delete a tag (wildcards allowed, e.g. `"XMP-dc:*"`). Repeatable |
| `--strip gps\|all` | `gps` removes all location data. `all` removes every tag except colour profile and orientation, then writes whatever else you passed |
| `--in-place` | Modify the originals. Cannot be combined with `--output` or `--suffix` |

**Order of operations:** strip, then remove, then named fields, then `--tag`. So `--tag` wins over a named field, and `--strip all --copyright X` leaves just the copyright.

**Validated before anything is written.** Tag names are checked against the list ExifTool can actually write, so a typo fails fast with a suggestion (`Artsit` -> `did you mean Artist?`) instead of being silently ignored. Latitude/longitude ranges, ratings and dates are checked too, and `--dry-run` validates exactly like a real run. Find tag names with [`photokit metadata tags`](#photokit-metadata-tags).

```bash
photokit metadata set ./photos --recursive \
  --creator "Your Name" \
  --copyright "© 2026 Your Name" \
  --rights "All rights reserved" \
  --keywords photography --keywords portrait

# Edit the originals in place (explicit opt-in)
photokit metadata set ./photos --in-place --creator "Your Name"

# Add keywords without losing the existing ones
photokit metadata set ./photos --append-keywords --keywords wedding,2026 -o ./tagged

# Location, GPS and capture date
photokit metadata set ./trip -r --city Dhaka --country Bangladesh --country-code BD \
  --latitude 23.8103 --longitude 90.4125 --altitude 12 --date-taken "2026-05-01 10:30:00+06:00"

# Rating, label and contact details
photokit metadata set ./picks --rating 5 --label Green \
  --contact-email me@example.com --contact-url https://example.com

# Any ExifTool tag, lists, deletions
photokit metadata set ./photos --tag "Comment=Shot on film" \
  --tag "XMP-dc:Subject=street" --tag "XMP-dc:Subject=night"
photokit metadata set ./photos --remove Comment --remove "XMP-dc:*"

# Make copies safe to publish: drop GPS (everything else stays)
photokit metadata set ./photos --strip gps -o ./safe-to-publish
```

### `photokit metadata tags`

Lists every tag ExifTool can write (about 7,000), with its group, so you know what to pass to `--tag`, `--remove` or a preset's `tags:` / `remove:`.

```bash
photokit metadata tags copyright            # substring filter (case-insensitive)
photokit metadata tags --group XMP-dc       # one group: EXIF, IPTC, XMP, XMP-dc, GPS, ...
photokit metadata tags --group IPTC --format json
```

### `photokit watermark apply`

Draws a signature on copies of JPEG and PNG photos.

| Flag | Default | Description |
|---|---|---|
| `--image FILE` | required | Transparent PNG signature |
| `--position` | `bottom-right` | One of the [nine anchors](#signatures-watermarks) |
| `--opacity` | `0.8` | `0`–`1` (exclusive of 0) |
| `--scale` | `0.12` | Signature width as a fraction of the image width |
| `--margin` | `32` | Pixels from the image edges |
| `--quality` | `92` | JPEG quality, 1–100 |

```bash
photokit watermark apply ./photos --recursive \
  --image ./assets/signature.png \
  --position bottom-right --opacity 0.8 --scale 0.12 --margin 32 \
  --output ./export
```

### `photokit watermark preview`

Renders **one** image with the signature so you can check placement quickly. It never modifies the input and writes no metadata.

| Flag | Default | Description |
|---|---|---|
| `-o`, `--output FILE` | `./preview.jpg` / `.png` | Preview to write (`.jpg` or `.png`) |
| `--max-size N` | `1600` | Longest edge in pixels, `0` keeps full size |
| `--overwrite` | | Replace an existing preview |

plus `--image`, `--position`, `--opacity`, `--scale`, `--margin` as above.

### `photokit process`

Runs a [preset](#presets): metadata, signature and export in one command.

```bash
photokit process ./photos --preset ./presets/default.yaml --output ./export
photokit process ./photos --preset default --dry-run
```

| Flag | Description |
|---|---|
| `-p`, `--preset` | **Required.** A file path, or a bare name looked up in `./presets` and your configured preset directories |
| `--no-watermark` | Skip the signature even if the preset enables it |
| `--export-format` | `original`, `jpeg` or `png` (overrides `export.format`) |
| `--quality` | JPEG quality (overrides `export.quality`) |
| `--suffix` | Output name suffix (overrides `export.suffix`) |
| `-o`, `--output` | Output directory (overrides `export.directory`) |
| metadata flags | Every `metadata set` flag (`--creator`, `--rating`, `--latitude`, `--contact-email`, `--tag`, `--remove`, `--strip`, ...) overrides the preset. See [override rules](#overriding-a-preset-from-the-command-line) |
| watermark flags | `--image`, `--position`, `--opacity`, `--scale`, `--margin` override individual preset settings |

See [override rules](#overriding-a-preset-from-the-command-line).

### `photokit preset`

```bash
photokit preset list                      # presets in ./presets and configured dirs
photokit preset list ~/my-presets         # presets in a specific directory
photokit preset show default              # the preset with relative paths resolved
photokit preset validate default other.yaml
```

`validate` reports **every** problem in a preset, not just the first, and checks that the signature file exists. It exits non-zero if any preset is invalid, so it is safe to use in CI.

## Presets

A preset is a YAML file describing a repeatable workflow.

```yaml
version: 1
name: default
description: Standard photography export

metadata:
  creator: "Your Name"
  copyright: "© 2026 Your Name"
  rights: "All rights reserved"
  keywords:
    - photography
  # Everything below is optional. See presets/full.yaml for the complete list.
  # rating: 5
  # city: Dhaka
  # contact: { email: me@example.com }
  # tags: { Comment: "Shot on film" }
  # strip: gps

watermark:
  enabled: true
  image: ../assets/signature.png   # relative to THIS file
  position: bottom-right
  opacity: 0.8
  scale: 0.12
  margin: 32

export:
  preserve_original: true
  format: original
  quality: 92
  directory: ./export
  # suffix: -signed
```

### Schema

| Key | Type | Default | Notes |
|---|---|---|---|
| `version` | int | required | Must be `1` |
| `name`, `description` | string | | Shown by `preset list` |
| `metadata.creator`, `copyright`, `rights`, `title`, `description`, `credit`, `source`, `instructions`, `headline`, `writer`, `owner`, `web_statement`, `label` | string | | Same fields as `metadata set`. Omitted fields are left untouched |
| `metadata.keywords` | list | | Replaces existing keywords. Empty entries are rejected |
| `metadata.rating` | int | | `0`-`5` |
| `metadata.location`, `city`, `state`, `country` | string | | Where the photo was taken |
| `metadata.country_code` | string | | 2-3 letter ISO code |
| `metadata.latitude`, `longitude` | float | | Decimal degrees, south/west negative. Must be given together |
| `metadata.altitude` | float | | Metres. Needs latitude and longitude |
| `metadata.date_taken` | string | | Same formats as `--date-taken`. Quote it in YAML |
| `metadata.contact` | map | | `email`, `phone`, `url`, `address`, `city`, `region`, `postal_code`, `country` |
| `metadata.tags` | map | | Any writable ExifTool tag: `Name: value`. A list value writes a list. `null` or `""` deletes the tag |
| `metadata.remove` | list | | Tags to delete (wildcards allowed) |
| `metadata.strip` | enum | `none` | `none`, `gps` or `all` |
| `watermark.enabled` | bool | `true` when the section exists | `false` disables the signature without deleting the section |
| `watermark.image` | path | required if enabled | Transparent PNG. Relative paths resolve against the **preset file's directory** |
| `watermark.position` | enum | `bottom-right` | One of the nine anchors |
| `watermark.opacity` | float | `0.8` | `0 < x ≤ 1` |
| `watermark.scale` | float | `0.12` | `0 < x ≤ 1`, fraction of image width |
| `watermark.margin` | int | `32` | Pixels, `≥ 0` |
| `export.preserve_original` | bool | `true` | **Must not be `false`.** Presets can never modify sources |
| `export.format` | enum | `original` | `original`, `jpeg`, `png` |
| `export.quality` | int | `92` | JPEG quality 1–100 |
| `export.directory` | path | `./export` | Relative to where you run `photokit` |
| `export.suffix` | string | | Appended to file names |

### Rules

- **Validated before processing.** An invalid preset stops the run before any file is touched. Tag names in `tags:` and `remove:` are checked against ExifTool, both by `preset validate` and by `process`.
- **Unknown fields are rejected**, so a typo like `opacty: 0.5` is an error rather than a silently ignored setting.
- **The schema is versioned** (`version: 1`) so it can evolve safely.
- **Relative asset paths resolve against the preset file**, so a preset works no matter where you run it from.
- **Presets never overwrite sources.**

### Overriding a preset from the command line

Flags always win over the preset:

| You pass | Effect |
|---|---|
| `--creator "X"` (any named metadata flag) | Replaces that field's preset value. Other preset fields stay |
| `--contact-phone "X"` | Replaces just that contact field. Other contact fields stay |
| `--tag NAME=VALUE` | Merges per tag: replaces that tag, keeps the preset's other tags |
| `--remove NAME` | Adds to the preset's `remove` list |
| `--strip gps` | Replaces the preset's `strip` |
| `--opacity 0.5` (any watermark flag) | Changes only that setting; the rest still comes from the preset |
| `--image other.png` on a preset *without* a watermark | Enables a watermark using default placement |
| `--no-watermark` | Skips the signature. Cannot be combined with watermark flags |
| `-o DIR` | Replaces `export.directory` |
| `--export-format`, `--quality`, `--suffix` | Replace the matching `export.*` value |

### Finding presets

`--preset` accepts a path (`./presets/wedding.yaml`) or a bare name (`wedding`). Names are searched as `<name>.yaml` / `<name>.yml` in:

1. `./presets`
2. directories listed under `preset_dirs` in your config
3. `<user config dir>/photokit/presets`

## Signatures (watermarks)

The signature is a **transparent PNG**, for example your handwritten signature or logo as white or black ink on a transparent background. Its alpha channel is preserved; `--opacity` scales it on top.

### Position

```
top-left      top-center      top-right
center-left   center         center-right
bottom-left   bottom-center   bottom-right
```

### Scale and margin

- **Scale** is relative to the image **width**: `0.12` means the signature is 12% as wide as the photo, with its aspect ratio preserved. The same preset gives a proportionally identical result on a 2 000 px and a 6 000 px image. A signature is never allowed to grow beyond the image.
- **Margin** is the gap in pixels between the signature and the nearest edge(s). It shrinks automatically on images too small to fit it.

### Orientation and metadata

- **EXIF orientation is applied first.** A portrait photo stored sideways with `Orientation=6` is rotated upright before placement, so `bottom-right` means bottom-right *as you view it*. The output's orientation tag is reset to normal so it is not rotated twice.
- **Metadata is restored from the original** after re-encoding: camera, lens, timestamps, GPS, copyright and so on. Embedded thumbnails and previews are deliberately **not** copied (they would show the un-watermarked image), and pixel dimensions come from the new file.
- JPEGs are re-encoded at `--quality` (default 92). Re-encoding is inherent to burning in a signature, but the original is untouched.

### Tips

- Use **`watermark preview`** first. It is quick and shows exactly where the signature lands.
- A white signature with a faint dark shadow stays legible on both bright and dark photos.
- For stronger protection keep `--opacity` high and `--scale` larger; for a discreet signature, lower both.

## Metadata

PhotoKit's metadata model is independent of any one standard. When it writes a value, it writes it **explicitly and identically to every standard that can hold it**, so EXIF, IPTC and XMP never contradict each other.

| Field | EXIF | IPTC | XMP |
|---|---|---|---|
| `creator` | `Artist` | `By-line` | `dc:Creator` |
| `copyright` | `Copyright` | `CopyrightNotice` | `dc:Rights` |
| `rights` | | | `xmpRights:UsageTerms` |
| `web_statement` | | | `xmpRights:WebStatement` |
| `description` | `ImageDescription` | `Caption-Abstract` | `dc:Description` |
| `title` | | `ObjectName` | `dc:Title` |
| `headline` | | `Headline` | `photoshop:Headline` |
| `keywords` | | `Keywords` | `dc:Subject` |
| `credit`, `source`, `instructions` | | `Credit`, `Source`, `SpecialInstructions` | `photoshop:*` |
| `writer` | | `Writer-Editor` | `photoshop:CaptionWriter` |
| `owner` | `OwnerName` | | `aux:OwnerName` |
| `rating` | `Rating`, `RatingPercent` | | `xmp:Rating` |
| `label` | | | `xmp:Label` |
| `location`, `city`, `state`, `country`, `country_code` | | `Sub-location`, `City`, `Province-State`, `Country-PrimaryLocationName`, `Country-PrimaryLocationCode` | `iptcCore:Location`, `photoshop:City`, `photoshop:State`, `photoshop:Country`, `iptcCore:CountryCode` |
| `latitude`, `longitude`, `altitude` | `GPSLatitude`/`Ref`, `GPSLongitude`/`Ref`, `GPSAltitude`/`Ref` | | `exif:GPS*` |
| `date_taken` | `DateTimeOriginal`, `CreateDate`, `OffsetTime*` | `DateCreated`, `TimeCreated` | `exif:`, `xmp:CreateDate`, `photoshop:DateCreated` |
| `contact.*` | | | `iptcCore:CreatorWorkEmail`, `CreatorWorkTelephone`, `CreatorWorkURL`, `CreatorAddress`, `CreatorCity`, `CreatorRegion`, `CreatorPostalCode`, `CreatorCountry` |

Notes:

- Setting `copyright` or `rights` also sets `xmpRights:Marked=True` so software knows the work is copyrighted.
- IPTC text is written as UTF-8, so `©`, accents and non-Latin scripts survive.
- When reading, the first non-empty value across standards wins (PNG files' native text chunks are also understood).
- Keywords replace the existing set by default; use `--append-keywords` to add instead.
- Multi-line values are flattened to a single line.
- GPS is written the way EXIF requires (absolute value plus `N/S/E/W` reference, and altitude reference `0` or `1`) and as signed decimals in XMP, so southern/western/below-sea-level values come back with the right sign.

### Anything else: raw ExifTool tags

PhotoKit has no named flag for most of ExifTool's ~7,000 writable tags, and doesn't need one. Use `--tag` (or a preset's `tags:`) with ExifTool's own names:

| Form | Meaning |
|---|---|
| `Comment=hi` | Tag `Comment`, in whichever group ExifTool picks |
| `XMP-dc:Format=image/jpeg` | A specific group (`Group:Tag`). `XMP-dc`, `IPTC`, `IFD0`, `ExifIFD`, `GPS` ... |
| `EXIF:IFD0:Make=...` | Family-0 and family-1 groups can be combined |
| `GPSAltitudeRef#=1` | A trailing `#` writes the raw numeric value instead of the printable one |
| `Subject=a` `Subject=b` (repeat) | A list tag. Repeating the name builds the list |
| `Comment=` | An empty value deletes the tag |

Discover names with `photokit metadata tags FILTER`, and inspect what a file contains with `photokit info FILE --all`. Names are verified against ExifTool's writable list before any file is touched. Anything that isn't a plain tag name (such as ExifTool command options like `csv`) is rejected.

### Privacy: `--strip`

| Mode | Removes |
|---|---|
| `gps` | All GPS tags in EXIF and XMP. Everything else is kept |
| `all` | Every tag. Colour profile and orientation are kept so the image still displays correctly. Combine with other flags to write fresh metadata afterwards |

## Supported formats

|  | Read / edit metadata | Visual watermark | Export / convert target |
|---|:---:|:---:|:---:|
| JPEG (`.jpg`, `.jpeg`) | ✅ | ✅ | ✅ |
| PNG (`.png`) | ✅ | ✅ | ✅ |
| TIFF, WebP, HEIC / HEIF | ✅ | ❌ | ❌ |
| RAW (CR2, CR3, NEF, ARW, DNG, ORF, RAF, RW2, PEF, …) | ✅ | ❌ | ❌ |

A RAW file is **never** treated as a rendered image. When a run includes a visual step (a signature or a format conversion), files that can't be rendered are *skipped entirely with a clear reason*. They are not failed, and nothing is written for them, so you never end up with an unsigned copy by accident. One run can therefore safely mix JPEG and RAW files:

```
skipped   photos/IMG_004.CR3 -> unsupported format: image rendering (watermark) is not supported for raw files
```

Metadata-only operations (`metadata set`, or `process --no-watermark` with a JPEG/original export) work on every format above. Files with other extensions are ignored during directory scans, and reported as skipped when named explicitly.

## Configuration

Configuration is optional. PhotoKit looks for `photokit.yaml` (also `.yml`, `.json`, `.toml`) in the current directory, then in the user config directory (`~/.config/photokit` on Linux, `~/Library/Application Support/photokit` on macOS, `%AppData%\photokit` on Windows). Pass `--config FILE` to use a specific file (it must exist).

```yaml
jobs: 4                    # files processed in parallel (1-64)
output: ./export           # default output directory
preset_dirs:               # extra places to look up presets by name
  - ~/photo-presets
exiftool:
  path: /usr/local/bin/exiftool
```

Environment variables override the file:

| Variable | Setting |
|---|---|
| `PHOTOKIT_JOBS` | `jobs` |
| `PHOTOKIT_OUTPUT` | `output` |
| `PHOTOKIT_EXIFTOOL_PATH` | `exiftool.path` |

**Precedence** (highest first): command-line flag → environment variable → config file → built-in default. For the ExifTool location: `--exiftool` → `PHOTOKIT_EXIFTOOL_PATH` → `exiftool.path` → search `PATH`.

## Scripting and automation

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Success. For `process` / `watermark apply` / `metadata set`, no file failed (skipped files are not failures) |
| `1` | Any error: invalid input, invalid preset, ExifTool missing, a file failed, or the run was interrupted |

Errors go to **stderr** prefixed with `Error:`; results go to **stdout**.

### JSON output

Add `--format json` to get machine-readable output on stdout:

```bash
photokit process ./photos -r --preset default --format json
```

```json
{
  "total": 120,
  "processed": 115,
  "skipped": 3,
  "failed": 2,
  "duration": "12.4s",
  "results": [
    {
      "source": "photos/IMG_001.jpg",
      "destination": "export/IMG_001.jpg",
      "status": "processed",
      "operations": ["watermark", "metadata"]
    },
    {
      "source": "photos/IMG_002.jpg",
      "status": "skipped",
      "reason": "output already exists (use --overwrite to replace it)"
    }
  ]
}
```

`status` is `processed`, `skipped`, `failed` (with an `error`), or `planned` (dry run, which also adds `"dry_run": true` and a `planned` count). `info` and `metadata show` emit arrays of per-file objects.

```bash
# list files that failed
photokit process ./photos -r -p default --format json | jq -r '.results[] | select(.status=="failed") | "\(.source): \(.error)"'

# copyright holders across a library
photokit metadata show ./export -r --format json | jq -r '.[].metadata.creator' | sort | uniq -c
```

### Interruption

Ctrl-C (or `SIGTERM`) stops new files from starting, lets in-flight files finish or abort cleanly, removes temporary files, prints the summary of what completed, and exits non-zero. Re-run the same command to continue: finished outputs are skipped.

### Logging

Add `-v` / `--verbose` for structured per-file debug logs on stderr.

### Cron / CI example

```bash
#!/usr/bin/env sh
set -eu
photokit preset validate presets/default.yaml
photokit process /mnt/inbox --recursive --preset default --output /mnt/export --format json > report.json
```

## Recipes

**Copyright everything, no watermark, keep original names**

```bash
photokit metadata set ./photos -r --creator "Jane Doe" --copyright "© 2026 Jane Doe" -o ./copyrighted
```

**Web-ready JPEGs from PNG or JPEG masters**

```bash
photokit process ./masters -r -p default --export-format jpeg --quality 85 --suffix -web -o ./web
```

**Only certain files**

```bash
photokit process ./shoot -r -p default --include '*.jpg' --exclude 'rejects/*'
```

**Different signature position for one run**

```bash
photokit process ./photos -p default --position top-center --opacity 0.6
```

**Process a mixed folder of RAW + JPEG**

```bash
# JPEGs get the signature; RAW files are listed as skipped (they can't be rendered)
photokit process ./shoot -r -p default

# Copyright the RAW files too: metadata only, no signature
photokit process ./shoot -r -p default --no-watermark --include '*.CR3' --include '*.NEF'
```

**One preset per client**

```bash
photokit process ./clientA -p ./presets/clientA.yaml -o ./deliver/clientA
photokit process ./clientB -p ./presets/clientB.yaml -o ./deliver/clientB
```

**Check what a previous run produced**

```bash
photokit info ./export --recursive
```

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `exiftool not found` | Install ExifTool 12+ ([instructions](#1-install-exiftool)), or pass `--exiftool PATH` / set `PHOTOKIT_EXIFTOOL_PATH` |
| `unsupported exiftool version` | Upgrade ExifTool to 12 or newer |
| Everything is `skipped: output already exists` | Outputs from an earlier run exist. Use `--overwrite`, a different `--output`, or `--suffix` |
| `output directory ... is the input directory` | Choose an output directory different from the input |
| `duplicate output path` | Two inputs map to the same output name. Process them separately into different directories, or process their common parent with `--recursive` so folder structure is mirrored |
| `skipped ... not supported for raw files` | Expected: RAW files can't be rendered. Use `--no-watermark` (or `metadata set`) to write metadata to them |
| `watermark.image: cannot read ...` | The signature path is wrong. Paths in a preset are relative to the preset file, not your working directory. Run `photokit preset validate` |
| `"X" is not a tag ExifTool can write` | A typo or a read-only tag. The message suggests close matches. Find valid names with `photokit metadata tags X` |
| `latitude and longitude must be set together` | Give both `--latitude` and `--longitude` (and `--altitude` only with both) |
| `unknown field` / `field ... not found` in a preset | A typo in a preset key. Unknown fields are rejected on purpose. Compare with [the schema](#schema) |
| `X is not a valid JPEG file (content looks like TXT)` | The file is corrupt or mislabelled |
| Signature is placed on the "wrong" side | Placement uses the *displayed* orientation. Check the photo's EXIF orientation with `photokit info`, and use `watermark preview` |
| Watermarked JPEGs are a different size on disk | They are re-encoded. Adjust with `--quality` |
| A transparent PNG signature looks like a black box | The PNG has no alpha channel. Export it again with a transparent background |
| Slow on huge batches | Raise `--jobs`. Decoding large images is CPU-bound, so about one job per core is a good starting point |

## Development

Requirements: Go 1.22+, `make`, and ExifTool (integration tests skip themselves if ExifTool is missing).

```bash
make test          # run the tests
make test-race     # with the race detector (what CI runs)
make vet           # go vet
make fmt           # gofmt
make cover         # coverage summary
make dist          # cross-compile all six release targets into ./dist
make help          # list all targets
```

### Architecture

A modular monolith: one executable, independently testable packages under `internal/`.

```
cmd/photokit/        main: signal handling, exit code
internal/
  cli/               Cobra commands. Parse flags, print results, no processing logic
  app/               Use cases (Processor, MetadataService, WatermarkService). No Cobra
  domain/            Shared types: Photo, Format, Metadata, WatermarkConfig, Report, errors
  metadata/          Engine interface + ExifTool adapter + explicit tag mapping
  watermark/         Placement maths, alpha compositing, renderer (no metadata knowledge)
  pipeline/          Ordered operations on one photo, temp files, atomic publish
  batch/             File discovery and the bounded worker pool
  preset/            YAML schema, loader (strict), validator
  filesystem/        Atomic writes, canonical paths, copy helpers
  imageio/           Decode (EXIF-oriented) / encode
  output/            Table and JSON rendering
  config/            Viper configuration
  testutil/          Fixture generators and a fake metadata engine
presets/             Example preset
assets/              Placeholder signature
```

Boundaries that are enforced by the package graph:

- CLI commands contain no processing logic; app services never import Cobra.
- Direct commands and presets build the **same** pipeline, so they cannot diverge.
- Metadata code never imports image code, and the reverse.
- ExifTool is only reached through the `metadata.Engine` interface, always with an argument list or argument file, **never through a shell**.

### Tests

Tests use fixture images generated into temporary directories, never real photographs. They cover placement and scaling maths, alpha/opacity compositing, discovery rules (including symlinks and the export directory), worker-pool bounds and cancellation, preset parsing and validation (including malformed and unknown fields), the ExifTool adapter against a real ExifTool (round trips, keyword semantics, UTF-8, PNG), end-to-end processing (sources byte-identical afterwards, metadata surviving watermarking, orientation, overwrite rules, duplicate outputs, interrupted batches leaving no temp files), and the CLI itself (flags, overrides, exit codes).

## Releasing

Releases are automated with GitHub Actions and [GoReleaser](https://goreleaser.com).

**Continuous integration** (`.github/workflows/ci.yml`) runs on every push to `main` and every pull request: `gofmt`, `go vet`, a `go mod tidy` cleanliness check, the full test suite with the race detector on **Linux, macOS and Windows** (with a real ExifTool installed on each), and a cross-compile of every release target.

**To publish a release:**

```bash
git tag v1.0.0
git push origin v1.0.0
```

The `Release` workflow (`.github/workflows/release.yml`) then:

1. runs the tests,
2. builds static binaries for **Linux, macOS and Windows on amd64 and arm64**,
3. packages them (`.tar.gz`, or `.zip` on Windows) together with the README, example preset and signature,
4. generates `checksums.txt` (SHA-256) and a changelog from commit messages,
5. publishes a GitHub Release. Tags like `v1.0.0-rc.1` are marked as pre-releases.

The version, commit and build date are stamped into the binary and shown by `photokit version`.

Try the packaging locally without publishing: `goreleaser release --snapshot --clean`.

> **First-time setup:** run `go mod tidy` once and commit the generated `go.sum`. CI fails fast if `go.mod`/`go.sum` are not tidy.

## Limitations and roadmap

PhotoKit is deliberately small: no database, no plugin system, no custom image engine, and ExifTool is a required external dependency.

Current limits:

- Visual watermarking is JPEG and PNG only; the signature must be a PNG.
- Rotation of the signature is not available yet.
- JPEG output is re-encoded when a signature or conversion is applied.

Possible future work: watch folders, named profiles (multiple photographer identities), resizing and multiple export sizes, WebP/AVIF export, contact sheets, duplicate detection, GPS management, ICC profile handling, preset chaining, and bundling ExifTool.
