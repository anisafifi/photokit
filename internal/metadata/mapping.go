package metadata

import (
	"fmt"
	"math"
	"strconv"
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
	webStatementTags = []string{"XMP-xmpRights:WebStatement"}
	descriptionTags  = []string{"IFD0:ImageDescription", "IPTC:Caption-Abstract", "XMP-dc:Description"}
	titleTags        = []string{"IPTC:ObjectName", "XMP-dc:Title"}
	headlineTags     = []string{"IPTC:Headline", "XMP-photoshop:Headline"}
	writerTags       = []string{"IPTC:Writer-Editor", "XMP-photoshop:CaptionWriter"}
	creditTags       = []string{"IPTC:Credit", "XMP-photoshop:Credit"}
	sourceTags       = []string{"IPTC:Source", "XMP-photoshop:Source"}
	instructionsTags = []string{"IPTC:SpecialInstructions", "XMP-photoshop:Instructions"}
	keywordTags      = []string{"IPTC:Keywords", "XMP-dc:Subject"}
	ownerTags        = []string{"ExifIFD:OwnerName", "XMP-aux:OwnerName"}
	labelTags        = []string{"XMP-xmp:Label"}

	locationTags    = []string{"IPTC:Sub-location", "XMP-iptcCore:Location"}
	cityTags        = []string{"IPTC:City", "XMP-photoshop:City"}
	stateTags       = []string{"IPTC:Province-State", "XMP-photoshop:State"}
	countryTags     = []string{"IPTC:Country-PrimaryLocationName", "XMP-photoshop:Country"}
	countryCodeTags = []string{"IPTC:Country-PrimaryLocationCode", "XMP-iptcCore:CountryCode"}

	contactEmailTags  = []string{"XMP-iptcCore:CreatorWorkEmail"}
	contactPhoneTags  = []string{"XMP-iptcCore:CreatorWorkTelephone"}
	contactURLTags    = []string{"XMP-iptcCore:CreatorWorkURL"}
	contactAddrTags   = []string{"XMP-iptcCore:CreatorAddress"}
	contactCityTags   = []string{"XMP-iptcCore:CreatorCity"}
	contactRegionTags = []string{"XMP-iptcCore:CreatorRegion"}
	contactPostalTags = []string{"XMP-iptcCore:CreatorPostalCode"}
	contactCtryTags   = []string{"XMP-iptcCore:CreatorCountry"}
)

// Read priority (first non-empty wins), using ExifTool "-G1" group names.
var (
	readCreator      = []string{"IFD0:Artist", "XMP-dc:Creator", "IPTC:By-line", "PNG:Artist", "PNG:Author"}
	readCopyright    = []string{"IFD0:Copyright", "XMP-dc:Rights", "IPTC:CopyrightNotice", "PNG:Copyright"}
	readRights       = []string{"XMP-xmpRights:UsageTerms"}
	readWebStatement = []string{"XMP-xmpRights:WebStatement"}
	readDescription  = []string{"XMP-dc:Description", "IFD0:ImageDescription", "IPTC:Caption-Abstract", "PNG:Description"}
	readTitle        = []string{"XMP-dc:Title", "IPTC:ObjectName", "PNG:Title"}
	readHeadline     = []string{"XMP-photoshop:Headline", "IPTC:Headline"}
	readWriter       = []string{"XMP-photoshop:CaptionWriter", "IPTC:Writer-Editor"}
	readCredit       = []string{"XMP-photoshop:Credit", "IPTC:Credit"}
	readSource       = []string{"XMP-photoshop:Source", "IPTC:Source"}
	readInstructions = []string{"XMP-photoshop:Instructions", "IPTC:SpecialInstructions"}
	readKeywords     = []string{"XMP-dc:Subject", "IPTC:Keywords"}
	readOwner        = []string{"ExifIFD:OwnerName", "XMP-aux:OwnerName"}
	readLabel        = []string{"XMP-xmp:Label"}
	readRating       = []string{"XMP-xmp:Rating", "IFD0:Rating"}

	readLocation    = []string{"XMP-iptcCore:Location", "IPTC:Sub-location"}
	readCity        = []string{"XMP-photoshop:City", "IPTC:City"}
	readState       = []string{"XMP-photoshop:State", "IPTC:Province-State"}
	readCountry     = []string{"XMP-photoshop:Country", "IPTC:Country-PrimaryLocationName"}
	readCountryCode = []string{"XMP-iptcCore:CountryCode", "IPTC:Country-PrimaryLocationCode"}
	readDateTaken   = []string{"ExifIFD:DateTimeOriginal", "XMP-exif:DateTimeOriginal", "ExifIFD:CreateDate", "XMP-xmp:CreateDate", "IFD0:ModifyDate"}

	readLatitude  = []string{"Composite:GPSLatitude", "XMP-exif:GPSLatitude"}
	readLongitude = []string{"Composite:GPSLongitude", "XMP-exif:GPSLongitude"}
	readAltitude  = []string{"Composite:GPSAltitude", "XMP-exif:GPSAltitude"}

	readContactEmail  = []string{"XMP-iptcCore:CreatorWorkEmail"}
	readContactPhone  = []string{"XMP-iptcCore:CreatorWorkTelephone"}
	readContactURL    = []string{"XMP-iptcCore:CreatorWorkURL"}
	readContactAddr   = []string{"XMP-iptcCore:CreatorAddress"}
	readContactCity   = []string{"XMP-iptcCore:CreatorCity"}
	readContactRegion = []string{"XMP-iptcCore:CreatorRegion"}
	readContactPostal = []string{"XMP-iptcCore:CreatorPostalCode"}
	readContactCtry   = []string{"XMP-iptcCore:CreatorCountry"}
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

func fnum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// buildWriteArgs converts metadata into ExifTool arguments, in the order
// ExifTool must see them: strip, remove, named fields, then raw tags (which
// therefore win over named fields). They are handed to ExifTool through an
// argument file, never through a shell. The result is nil when there is
// nothing to do.
func buildWriteArgs(md domain.Metadata, opts WriteOptions) []string {
	var args []string

	// 1. Strip.
	switch strings.ToLower(md.Strip) {
	case domain.StripAll:
		// Delete every tag, then put back what the picture needs to look
		// right: the colour profile and the orientation.
		args = append(args, "-all=", "-tagsfromfile", "@", "-ICC_Profile", "-Orientation#")
	case domain.StripGPS:
		args = append(args, "-GPS:all=", "-XMP-exif:GPS*=")
	}

	// 2. Remove.
	seenRemove := map[string]bool{}
	for _, r := range md.Remove {
		if k := strings.ToLower(r); !seenRemove[k] {
			seenRemove[k] = true
			args = append(args, "-"+r+"=")
		}
	}

	// 3. Named fields.
	var named []string
	set := func(tags []string, value string) {
		value = clean(value)
		if value == "" {
			return
		}
		for _, t := range tags {
			named = append(named, "-"+t+"="+value)
		}
	}
	set(creatorTags, md.Creator)
	set(copyrightTags, md.Copyright)
	set(rightsTags, md.Rights)
	set(webStatementTags, md.WebStatement)
	set(descriptionTags, md.Description)
	set(titleTags, md.Title)
	set(headlineTags, md.Headline)
	set(writerTags, md.Writer)
	set(creditTags, md.Credit)
	set(sourceTags, md.Source)
	set(instructionsTags, md.Instructions)
	set(ownerTags, md.Owner)
	set(labelTags, md.Label)
	set(locationTags, md.Location)
	set(cityTags, md.City)
	set(stateTags, md.State)
	set(countryTags, md.Country)
	set(countryCodeTags, strings.ToUpper(md.CountryCode))
	set(contactEmailTags, md.Contact.Email)
	set(contactPhoneTags, md.Contact.Phone)
	set(contactURLTags, md.Contact.URL)
	set(contactAddrTags, md.Contact.Address)
	set(contactCityTags, md.Contact.City)
	set(contactRegionTags, md.Contact.Region)
	set(contactPostalTags, md.Contact.PostalCode)
	set(contactCtryTags, md.Contact.Country)

	if clean(md.Copyright) != "" || clean(md.Rights) != "" {
		named = append(named, "-XMP-xmpRights:Marked=True")
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
				named = append(named, "-"+t+op+k)
			}
		}
	}

	named = append(named, gpsArgs(md)...)

	if md.Rating != nil {
		r := *md.Rating
		named = append(named,
			"-IFD0:Rating="+strconv.Itoa(r),
			"-IFD0:RatingPercent="+strconv.Itoa(ratingPercent(r)),
			"-XMP-xmp:Rating="+strconv.Itoa(r))
	}

	if md.DateTaken != "" {
		if d, err := domain.ParseDateTime(md.DateTaken); err == nil {
			named = append(named, dateArgs(d)...)
		}
	}

	// 4. Raw tags.
	var raw []string
	for _, name := range md.SortedTagNames() {
		vals := md.Tags[name]
		if len(vals) == 0 {
			vals = domain.TagValue{""}
		}
		for _, v := range vals {
			raw = append(raw, "-"+name+"="+clean(v))
		}
	}

	if len(named)+len(raw) > 0 {
		// Make IPTC text unambiguous (UTF-8) so "©" and accents survive.
		args = append(args, "-IPTC:CodedCharacterSet=UTF8")
	}
	args = append(args, named...)
	args = append(args, raw...)
	return args
}

func ratingPercent(r int) int {
	switch r {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return 25
	case 3:
		return 50
	case 4:
		return 75
	}
	return 99
}

func hemisphere(v float64, pos, neg string) string {
	if v < 0 {
		return neg
	}
	return pos
}

// gpsArgs writes absolute values with explicit hemisphere references to EXIF
// (ExifTool does not derive the reference from a sign) and signed decimals to
// XMP. The altitude reference is numeric, so it needs the "#" form.
func gpsArgs(md domain.Metadata) []string {
	if md.Latitude == nil || md.Longitude == nil {
		return nil
	}
	lat, lon := *md.Latitude, *md.Longitude
	args := []string{
		"-EXIF:GPSLatitude=" + fnum(math.Abs(lat)),
		"-EXIF:GPSLatitudeRef=" + hemisphere(lat, "N", "S"),
		"-EXIF:GPSLongitude=" + fnum(math.Abs(lon)),
		"-EXIF:GPSLongitudeRef=" + hemisphere(lon, "E", "W"),
		"-XMP-exif:GPSLatitude=" + fnum(lat),
		"-XMP-exif:GPSLongitude=" + fnum(lon),
	}
	if md.Altitude != nil {
		alt := *md.Altitude
		ref := "0"
		if alt < 0 {
			ref = "1"
		}
		args = append(args,
			"-EXIF:GPSAltitude="+fnum(math.Abs(alt)),
			"-EXIF:GPSAltitudeRef#="+ref,
			"-XMP-exif:GPSAltitude="+fnum(math.Abs(alt)),
			"-XMP-exif:GPSAltitudeRef#="+ref)
	}
	return args
}

// dateArgs writes the capture date consistently to EXIF, IPTC and XMP. EXIF
// has no time zone field in the date itself, so a zone goes to OffsetTime*.
func dateArgs(d domain.DateTime) []string {
	args := []string{
		"-ExifIFD:DateTimeOriginal=" + d.Local(),
		"-ExifIFD:CreateDate=" + d.Local(),
		"-XMP-exif:DateTimeOriginal=" + d.WithZone(),
		"-XMP-xmp:CreateDate=" + d.WithZone(),
		"-XMP-photoshop:DateCreated=" + d.WithZone(),
		"-IPTC:DateCreated=" + d.Date,
		"-IPTC:TimeCreated=" + d.Time + d.Zone,
	}
	if d.Zone != "" {
		args = append(args, "-ExifIFD:OffsetTimeOriginal="+d.Zone, "-ExifIFD:OffsetTimeDigitized="+d.Zone)
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

// parseCoordinate understands ExifTool's printed forms of a GPS coordinate:
// `23 deg 48' 37.08" N`, `33.9 S` and plain signed decimals.
func parseCoordinate(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	sign := 1.0
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'S', 'W':
			sign = -1
			s = strings.TrimSpace(s[:n-1])
		case 'N', 'E':
			s = strings.TrimSpace(s[:n-1])
		}
	}
	var deg, min, sec float64
	if n, _ := fmt.Sscanf(s, "%f deg %f' %f\"", &deg, &min, &sec); n == 3 {
		return sign * (math.Abs(deg) + min/60 + sec/3600) * signOf(deg), true
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return sign * v, true
	}
	return 0, false
}

func signOf(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// parseAltitude understands "12.5 m", "30 m Below Sea Level" and plain numbers.
func parseAltitude(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	below := strings.Contains(strings.ToLower(s), "below")
	fields := strings.Fields(s)
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	if below {
		v = -math.Abs(v)
	}
	return v, true
}
