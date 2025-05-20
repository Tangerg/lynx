package mime

import (
	"strings"
)

// xPrefixSubtypeToStandard maps MIME types with x-prefix to their standard equivalents
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

// NormalizeXSubtype converts MIME types with x-prefix in subtype to their standard form.
// It first checks a predefined mapping table for known conversions. If no mapping is found,
// it simply removes the "x-" prefix from the subtype.
//
// Examples:
// - "application/x-javascript" becomes "application/javascript"
// - "text/x-markdown" becomes "text/markdown"
//
// This function always returns a new MIME instance and does not modify the original.
func NormalizeXSubtype(m *MIME) *MIME {
	if !strings.HasPrefix(m.subType, "x-") {
		return m.Clone()
	}

	standardSubtype, ok := xPrefixSubtypeToStandard[m.subType]
	if !ok {
		standardSubtype = strings.TrimPrefix(m.subType, "x-")
	}

	standardMime, _ := NewBuilder().
		FromMime(m).
		WithSubType(standardSubtype).
		Build()
	return standardMime
}
