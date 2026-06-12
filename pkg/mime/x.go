package mime

import (
	"strings"
	"sync"
)

// xPrefixSubtypeToStandard maps legacy "x-" subtypes onto their RFC
// 6648 standard counterparts.
var xPrefixSubtypeToStandard = map[string]string{
	"x-javascript":          "javascript",
	"x-ecmascript":          "ecmascript",
	"x-www-form-urlencoded": "x-www-form-urlencoded",
	"x-latex":               "latex",
	"x-sh":                  "sh",
	"x-perl":                "perl",
	"x-httpd-php":           "php",
	"x-httpd-cgi":           "cgi",
	"x-dvi":                 "dvi",
	"x-gzip":                "gzip",
	"x-compressed":          "compressed",
	"x-zip-compressed":      "zip",
	"x-stuffit":             "stuffit",
	"x-rar-compressed":      "vnd.rar",
	"x-7z-compressed":       "x-7z-compressed",
	"x-shockwave-flash":     "vnd.adobe.flash-movie",
	"x-director":            "vnd.adobe.director",
	"x-msdos-program":       "vnd.microsoft.portable-executable",
	"x-wais-source":         "wais-source",
	"x-bittorrent":          "x-bittorrent",
	"x-csh":                 "csh",
	"x-python":              "python",
	"x-ruby":                "ruby",
	"x-json":                "json",
	"x-bytecode.python":     "python-bytecode",
	"x-yaml":                "yaml",
	"x-ole-storage":         "vnd.ms-ole-storage",
	"x-tcl":                 "tcl",
	"x-pkcs7-signature":     "pkcs7-signature",
	"x-pkcs7-mime":          "pkcs7-mime",
	"x-x509-ca-cert":        "x-x509-ca-cert",
	"x-mpeg":                "mpeg",
	"x-mp3":                 "mpeg",
	"x-wav":                 "wav",
	"x-midi":                "midi",
	"x-aiff":                "aiff",
	"x-ms-wma":              "x-ms-wma",
	"x-realaudio":           "vnd.rn-realaudio",
	"x-pn-realaudio":        "vnd.rn-realaudio",
	"x-ogg":                 "ogg",
	"x-flac":                "flac",
	"x-ac3":                 "ac3",
	"x-m4a":                 "mp4",
	"x-m4r":                 "mp4",
	"x-mod":                 "x-mod",
	"x-aac":                 "aac",
	"x-png":                 "png",
	"x-icon":                "vnd.microsoft.icon",
	"x-ms-bmp":              "bmp",
	"x-portable-pixmap":     "x-portable-pixmap",
	"x-portable-bitmap":     "x-portable-bitmap",
	"x-portable-graymap":    "x-portable-graymap",
	"x-rgb":                 "x-rgb",
	"x-xbitmap":             "x-xbitmap",
	"x-xpixmap":             "x-xpixmap",
	"x-tiff":                "tiff",
	"x-xcf":                 "x-xcf",
	"x-photoshop":           "vnd.adobe.photoshop",
	"x-cmu-raster":          "x-cmu-raster",
	"x-pict":                "x-pict",
	"x-webp":                "webp",
	"x-windows-bmp":         "bmp",
	"x-tga":                 "x-tga",
	"x-markdown":            "markdown",
	"x-java-source":         "x-java-source",
	"x-c":                   "x-c",
	"x-c++":                 "x-c++",
	"x-pascal":              "x-pascal",
	"x-diff":                "x-diff",
	"x-tex":                 "x-tex",
	"x-log":                 "x-log",
	"x-fortran":             "x-fortran",
	"x-asm":                 "x-asm",
	"x-script":              "x-script",
	"x-vcard":               "vcard",
	"x-vcalendar":           "calendar",
	"x-setext":              "x-setext",
	"x-csv":                 "csv",
	"x-sgml":                "sgml",
	"x-rst":                 "x-rst",
	"x-asciidoc":            "x-asciidoc",
	"x-component":           "html-component",
	"x-scss":                "x-scss",
	"x-less":                "x-less",
	"x-msvideo":             "x-msvideo",
	"x-ms-wmv":              "x-ms-wmv",
	"x-flv":                 "x-flv",
	"x-matroska":            "x-matroska",
	"x-ms-asf":              "vnd.ms-asf",
	"x-m4v":                 "mp4",
	"x-motion-jpeg":         "x-motion-jpeg",
	"x-dv":                  "x-dv",
	"x-sgi-movie":           "x-sgi-movie",
	"x-quicktime":           "quicktime",
}

// xPrefixMutex guards xPrefixSubtypeToStandard.
var xPrefixMutex sync.RWMutex

// RegisterXSubtype registers an "x-" subtype mapping consulted by
// [NormalizeXSubtype]. Safe for concurrent use.
func RegisterXSubtype(xSubtype, standardSubtype string) {
	xPrefixMutex.Lock()
	defer xPrefixMutex.Unlock()
	xPrefixSubtypeToStandard[xSubtype] = standardSubtype
}

// RegisterXSubtypes is the batch form of [RegisterXSubtype].
func RegisterXSubtypes(mappings map[string]string) {
	xPrefixMutex.Lock()
	defer xPrefixMutex.Unlock()
	for xSubtype, standardSubtype := range mappings {
		xPrefixSubtypeToStandard[xSubtype] = standardSubtype
	}
}

// NormalizeXSubtype returns a copy of sourceMime with its "x-" subtype
// rewritten to the modern equivalent. If no specific mapping is
// registered, the "x-" prefix is dropped. Subtypes without an "x-"
// prefix are returned as a clone unchanged.
func NormalizeXSubtype(sourceMime *MIME) *MIME {
	// Return a clone if the subtype doesn't have x-prefix
	if !strings.HasPrefix(sourceMime.subType, "x-") {
		return sourceMime.Clone()
	}

	xPrefixMutex.RLock()
	// Check if there's a specific mapping for this x-prefix subtype
	normalizedSubtype, hasMapping := xPrefixSubtypeToStandard[sourceMime.subType]
	xPrefixMutex.RUnlock()

	if !hasMapping {
		// If no mapping found, simply remove the "x-" prefix
		normalizedSubtype = strings.TrimPrefix(sourceMime.subType, "x-")
	}

	normalizedMime, _ := NewBuilder().
		FromMime(sourceMime).
		WithSubType(normalizedSubtype).
		Build()

	return normalizedMime
}
