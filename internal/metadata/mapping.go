package metadata

import (
	"strings"

	"photokit/internal/domain"
)

// Explicit mapping from PhotoKit fields to EXIF / IPTC / XMP tags. A managed
// value is written to every standard that can hold it, with identical
// content, so the standards never contradict each other.
var (
	creatorTags      = []string{"IFD0:Artist", "IPTC:By-line", "XMP-dc:Creator"}
	copyrightTags    = []string{"IFD0:Copyright", "IPTC:CopyrightNotice", "XMP-dc:Rights"}
	rightsTags       = []string{"XMP-xmpRights:UsageTerms"}
	descriptionTags  = []string{"IFD0:ImageDescription", "IPTC:Caption-Abstract", "XMP-dc:Description"}
	titleTags        = []string{"IPTC:ObjectName", "XMP-dc:Title"}
	creditTags       = []string{"IPTC:Credit", "XMP-photoshop:Credit"}
	sourceTags       = []string{"IPTC:Source", "XMP-photoshop:Source"}
	instructionsTags = []string{"IPTC:SpecialInstructions", "XMP-photoshop:Instructions"}
	keywordTags      = []string{"IPTC:Keywords", "XMP-dc:Subject"}
)

// Read priority (first non-empty wins), using ExifTool "-G1" group names.
var (
	readCreator      = []string{"IFD0:Artist", "XMP-dc:Creator", "IPTC:By-line", "PNG:Artist", "PNG:Author"}
	readCopyright    = []string{"IFD0:Copyright", "XMP-dc:Rights", "IPTC:CopyrightNotice", "PNG:Copyright"}
	readRights       = []string{"XMP-xmpRights:UsageTerms"}
	readDescription  = []string{"XMP-dc:Description", "IFD0:ImageDescription", "IPTC:Caption-Abstract", "PNG:Description"}
	readTitle        = []string{"XMP-dc:Title", "IPTC:ObjectName", "PNG:Title"}
	readCredit       = []string{"XMP-photoshop:Credit", "IPTC:Credit"}
	readSource       = []string{"XMP-photoshop:Source", "IPTC:Source"}
	readInstructions = []string{"XMP-photoshop:Instructions", "IPTC:SpecialInstructions"}
	readKeywords     = []string{"XMP-dc:Subject", "IPTC:Keywords"}
)

// Tags that are deliberately NOT restored onto a re-rendered image.
var copyExclusions = []string{
	"Orientation",     // pixels are already rotated; reset to 1 afterwards
	"ThumbnailImage",  // would leak the un-watermarked picture
	"PreviewImage",    // same
	"ThumbnailTIFF",   // same
	"ExifImageWidth",  // dimensions come from the new file
	"ExifImageHeight", //
	"ImageWidth",      //
	"ImageHeight",     //
}

func clean(v string) string {
	v = strings.ReplaceAll(v, "\r\n", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	return strings.TrimSpace(v)
}

// buildWriteArgs converts metadata into ExifTool arguments. They are handed
// to ExifTool through an argument file, never through a shell.
func buildWriteArgs(md domain.Metadata, opts WriteOptions) []string {
	var args []string
	set := func(tags []string, value string) {
		value = clean(value)
		if value == "" {
			return
		}
		for _, t := range tags {
			args = append(args, "-"+t+"="+value)
		}
	}
	set(creatorTags, md.Creator)
	set(copyrightTags, md.Copyright)
	set(rightsTags, md.Rights)
	set(descriptionTags, md.Description)
	set(titleTags, md.Title)
	set(creditTags, md.Credit)
	set(sourceTags, md.Source)
	set(instructionsTags, md.Instructions)

	if clean(md.Copyright) != "" || clean(md.Rights) != "" {
		args = append(args, "-XMP-xmpRights:Marked=True")
	}

	var kws []string
	for _, k := range md.Keywords {
		if k = clean(k); k != "" {
			kws = append(kws, k)
		}
	}
	if len(kws) > 0 {
		// ExifTool quirk: a clearing assignment in the same command as "+="
		// is ignored. Repeated "=" assignments replace the whole list, and
		// "+=" appends, so each mode uses exactly one operator.
		op := "="
		if opts.AppendKeywords {
			op = "+="
		}
		for _, k := range kws {
			for _, t := range keywordTags {
				args = append(args, "-"+t+op+k)
			}
		}
	}
	if len(args) > 0 {
		// Make IPTC text unambiguous (UTF-8) so "©" and accents survive.
		args = append([]string{"-IPTC:CodedCharacterSet=UTF8"}, args...)
	}
	return args
}

// buildCopyArgs builds the arguments restoring metadata from source onto a
// freshly rendered file.
func buildCopyArgs(source string) []string {
	args := []string{"-TagsFromFile", source, "-all:all"}
	for _, t := range copyExclusions {
		args = append(args, "--"+t)
	}
	args = append(args, "-Orientation#=1")
	return args
}
