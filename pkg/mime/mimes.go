package mime

import (
	"fmt"
	"mime"
	"path"
	"strings"
	"sync"
)

var extMimetypeStringMappings = map[string]string{
	// Microsoft Office (OpenXML)
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xltx": "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	".potx": "application/vnd.openxmlformats-officedocument.presentationml.template",
	".ppsx": "application/vnd.openxmlformats-officedocument.presentationml.slideshow",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".sldx": "application/vnd.openxmlformats-officedocument.presentationml.slide",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".dotx": "application/vnd.openxmlformats-officedocument.wordprocessingml.template",
	".xlam": "application/vnd.ms-excel.addin.macroEnabled.12",
	".xlsb": "application/vnd.ms-excel.sheet.binary.macroEnabled.12",

	// Old Microsoft Office
	".doc": "application/msword",
	".dot": "application/msword",
	".xls": "application/vnd.ms-excel",
	".xlt": "application/vnd.ms-excel",
	".xla": "application/vnd.ms-excel",
	".xlc": "application/vnd.ms-excel",
	".xlm": "application/vnd.ms-excel",
	".xlw": "application/vnd.ms-excel",
	".ppt": "application/vnd.ms-powerpoint",
	".pot": "application/vnd.ms-powerpoint",
	".pps": "application/vnd.ms-powerpoint",
	".mpp": "application/vnd.ms-project",

	// Microsoft Works & Other MS Formats
	".wcm": "application/vnd.ms-works",
	".wdb": "application/vnd.ms-works",
	".wks": "application/vnd.ms-works",
	".wps": "application/vnd.ms-works",
	".pub": "application/x-mspublisher",
	".mdb": "application/x-msaccess",
	".msg": "application/vnd.ms-outlook",
	".mny": "application/x-msmoney",

	// OpenDocument
	".odc": "application/vnd.oasis.opendocument.chart",
	".odb": "application/vnd.oasis.opendocument.database",
	".odf": "application/vnd.oasis.opendocument.formula",
	".odg": "application/vnd.oasis.opendocument.graphics",
	".otg": "application/vnd.oasis.opendocument.graphics-template",
	".odi": "application/vnd.oasis.opendocument.image",
	".odp": "application/vnd.oasis.opendocument.presentation",
	".otp": "application/vnd.oasis.opendocument.presentation-template",
	".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".ots": "application/vnd.oasis.opendocument.spreadsheet-template",
	".odt": "application/vnd.oasis.opendocument.text",
	".odm": "application/vnd.oasis.opendocument.text-master",
	".ott": "application/vnd.oasis.opendocument.text-template",
	".oth": "application/vnd.oasis.opendocument.text-web",

	// Sun/OpenOffice Legacy
	".sxw": "application/vnd.sun.xml.writer",
	".stw": "application/vnd.sun.xml.writer.template",
	".sxc": "application/vnd.sun.xml.calc",
	".stc": "application/vnd.sun.xml.calc.template",
	".sxd": "application/vnd.sun.xml.draw",
	".std": "application/vnd.sun.xml.draw.template",
	".sxi": "application/vnd.sun.xml.impress",
	".sti": "application/vnd.sun.xml.impress.template",
	".sxg": "application/vnd.sun.xml.writer.global",
	".sxm": "application/vnd.sun.xml.math",

	// Documents
	".pdf":  "application/pdf",
	".rtf":  "text/rtf",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".tsv":  "text/tab-separated-values",
	".json": "application/json",
	".xml":  "application/xml",
	".xsl":  "application/xml",
	".xslt": "application/xslt+xml",
	".dtd":  "application/xml-dtd",

	// Archives & Compressed
	".zip":     "application/zip",
	".rar":     "application/vnd.rar",
	".7z":      "application/x-7z-compressed",
	".tar":     "application/x-tar",
	".gz":      "application/gzip",
	".bz2":     "application/x-bzip2",
	".tgz":     "application/gzip",
	".sit":     "application/x-stuffit",
	".sea":     "application/x-stuffit",
	".lha":     "application/octet-stream",
	".lzh":     "application/octet-stream",
	".gtar":    "application/x-gtar",
	".ustar":   "application/x-ustar",
	".cpio":    "application/x-cpio",
	".bcpio":   "application/x-bcpio",
	".sv4cpio": "application/x-sv4cpio",
	".sv4crc":  "application/x-sv4crc",
	".shar":    "application/x-shar",
	".torrent": "application/x-bittorrent",
	".uu":      "application/x-uuencode",
	".uue":     "application/x-uuencode",
	".z":       "application/x-compress",
	".taz":     "application/x-tar",
	".x-gzip":  "application/x-gzip",

	// Images - Common
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".jpe":  "image/jpeg",
	".jfif": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
	".tif":  "image/tiff",
	".tiff": "image/tiff",

	// Images - Advanced Formats
	".jp2":  "image/jp2",
	".j2k":  "image/j2k",
	".jpz":  "image/jpeg",
	".pnz":  "image/png",
	".djv":  "image/vnd.djvu",
	".djvu": "image/vnd.djvu",
	".wbmp": "image/vnd.wap.wbmp",

	// Images - Legacy & Specialized
	".ras":  "image/x-cmu-raster",
	".pnm":  "image/x-portable-anymap",
	".pbm":  "image/x-portable-bitmap",
	".pgm":  "image/x-portable-graymap",
	".ppm":  "image/x-portable-pixmap",
	".rgb":  "image/x-rgb",
	".xbm":  "image/x-xbitmap",
	".xpm":  "image/x-xpixmap",
	".xwd":  "image/x-xwindowdump",
	".pct":  "image/pict",
	".pic":  "image/pict",
	".pict": "image/pict",
	".pnt":  "image/x-macpaint",
	".pntg": "image/x-macpaint",
	".mac":  "image/x-macpaint",
	".qti":  "image/x-quicktime",
	".qtif": "image/x-quicktime",
	".pcx":  "image/x-pcx",
	".pda":  "image/x-pda",
	".ief":  "image/ief",
	".cgm":  "image/cgm",
	".cmx":  "image/x-cmx",
	".cod":  "image/cis-cod",
	".eri":  "image/x-eri",
	".fpx":  "image/x-fpx",
	".dcx":  "image/x-dcx",
	".cal":  "image/x-cals",
	".mil":  "image/x-cals",
	".nbmp": "image/nbmp",
	".rf":   "image/vnd.rn-realflash",
	".rp":   "image/vnd.rn-realpix",
	".si6":  "image/si6",
	".si7":  "image/vnd.stiwap.sis",
	".si9":  "image/vnd.lgtwap.sis",
	".svh":  "image/svh",
	".svf":  "image/vnd",
	".toy":  "image/toy",
	".wi":   "image/wavelet",
	".wpng": "image/x-up-wpng",
	".fh4":  "image/x-freehand",
	".fh5":  "image/x-freehand",
	".fhc":  "image/x-freehand",
	".ifm":  "image/gif",
	".ifs":  "image/ifs",

	// Audio - Common
	".mp3":  "audio/mpeg",
	".mp2":  "audio/mpeg",
	".mpga": "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".aac":  "audio/aac",
	".flac": "audio/flac",
	".opus": "audio/opus",

	// Audio - MPEG-4
	".m4a": "audio/mp4",
	".m4b": "audio/mp4",
	".m4p": "audio/mp4",

	// Audio - Windows Media
	".wma": "audio/x-ms-wma",
	".wax": "audio/x-ms-wax",

	// Audio - MIDI
	".mid":  "audio/midi",
	".midi": "audio/midi",
	".kar":  "audio/midi",
	".rmi":  "audio/midi",

	// Audio - RealAudio
	".ra":   "audio/x-pn-realaudio",
	".ram":  "audio/x-pn-realaudio",
	".rm":   "application/vnd.rn-realmedia",
	".rmm":  "audio/x-pn-realaudio",
	".rmvb": "audio/x-pn-realaudio",
	".rmf":  "audio/x-rmf",

	// Audio - Other Formats
	".au":    "audio/basic",
	".snd":   "audio/basic",
	".aif":   "audio/x-aiff",
	".aifc":  "audio/x-aiff",
	".aiff":  "audio/x-aiff",
	".m3u":   "audio/x-mpegurl",
	".m3url": "audio/x-mpegurl",
	".qcp":   "audio/vnd.qcelp",

	// Audio - Specialized & Module Formats
	".als":  "audio/x-alpha5",
	".awb":  "audio/amr-wb",
	".es":   "audio/echospeech",
	".esl":  "audio/echospeech",
	".imy":  "audio/melody",
	".it":   "audio/x-mod",
	".itz":  "audio/x-mod",
	".ma1":  "audio/ma1",
	".ma2":  "audio/ma2",
	".ma3":  "audio/ma3",
	".ma5":  "audio/ma5",
	".m15":  "audio/x-mod",
	".mdz":  "audio/x-mod",
	".mio":  "audio/x-mio",
	".mod":  "audio/x-mod",
	".mpn":  "application/vnd.mophun.application",
	".nsnd": "audio/nsnd",
	".pac":  "audio/x-pac",
	".pae":  "audio/x-epac",
	".s3m":  "audio/x-mod",
	".s3z":  "audio/x-mod",
	".smd":  "audio/x-smd",
	".smz":  "audio/x-smd",
	".tsi":  "audio/tsplayer",
	".ult":  "audio/x-mod",
	".vib":  "audio/vib",
	".vox":  "audio/voxware",
	".vqe":  "audio/x-twinvq-plugin",
	".vqf":  "audio/x-twinvq",
	".vql":  "audio/x-twinvq",
	".xm":   "audio/x-mod",
	".xmz":  "audio/x-mod",

	// Video - Common
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mpg4": "video/mp4",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".qt":   "video/quicktime",
	".mpeg": "video/mpeg",
	".mpg":  "video/mpeg",
	".mpe":  "video/mpeg",
	".mpa":  "video/mpeg",
	".mpv2": "video/mpeg",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".3gp":  "video/3gpp",
	".mkv":  "video/x-matroska",

	// Video - Windows Media
	".wmv": "video/x-ms-wmv",
	".wm":  "video/x-ms-wm",
	".wmx": "video/x-ms-wmx",
	".wvx": "video/x-ms-wvx",
	".asf": "video/x-ms-asf",
	".asr": "video/x-ms-asf",
	".asx": "video/x-ms-asf",

	// Video - Other Formats
	".flv":   "video/x-flv",
	".dv":    "video/x-dv",
	".dif":   "video/x-dv",
	".fvi":   "video/isivideo",
	".lsf":   "video/x-la-asf",
	".lsx":   "video/x-la-asf",
	".m4u":   "video/vnd.mpegurl",
	".mng":   "video/x-mng",
	".movie": "video/x-sgi-movie",
	".mxu":   "video/vnd.mpegurl",
	".pvx":   "video/x-pv-pvx",
	".rv":    "video/vnd.rn-realvideo",
	".vdo":   "video/vdo",
	".viv":   "video/vivo",
	".vivo":  "video/vivo",
	".wv":    "video/wavelet",

	// Web
	".html":  "text/html",
	".htm":   "text/html",
	".shtml": "text/html",
	".shtm":  "text/html",
	".stm":   "text/html",
	".dhtml": "text/html",
	".hts":   "text/html",
	".htt":   "text/webviewhtml",
	".xhtml": "application/xhtml+xml",
	".xht":   "application/xhtml+xml",
	".xhtm":  "application/xhtml+xml",
	".css":   "text/css",
	".js":    "application/javascript",
	".mjs":   "application/javascript",
	".cjs":   "application/javascript",
	".wasm":  "application/wasm",

	// Web - Markup & Data
	".sgm":    "text/sgml",
	".sgml":   "text/sgml",
	".rdf":    "application/rdf+xml",
	".atom":   "application/atom+xml",
	".vxml":   "application/voicexml+xml",
	".xul":    "application/vnd.mozilla.xul+xml",
	".mathml": "application/mathml+xml",

	// Web - WAP
	".wml":       "text/vnd.wap.wml",
	".wmls":      "text/vnd.wap.wmlscript",
	".wmlscript": "text/vnd.wap.wmlscript",
	".wmlc":      "application/vnd.wap.wmlc",
	".wmlsc":     "application/vnd.wap.wmlscriptc",
	".wsc":       "application/vnd.wap.wmlscriptc",
	".wbxml":     "application/vnd.wap.wbxml",
	".hdm":       "text/x-hdml",
	".hdml":      "text/x-hdml",

	// Text Formats
	".rtx":   "text/richtext",
	".etx":   "text/x-setext",
	".asc":   "text/plain",
	".c":     "text/x-c",
	".cc":    "text/x-c",
	".cpp":   "text/x-c",
	".h":     "text/x-c",
	".java":  "text/x-java-source",
	".bas":   "text/plain",
	".conf":  "text/plain",
	".log":   "text/plain",
	".prop":  "text/plain",
	".rc":    "text/plain",
	".ics":   "text/calendar",
	".ifb":   "text/calendar",
	".vcf":   "text/x-vcard",
	".323":   "text/h323",
	".jad":   "text/vnd.sun.j2me.app-descriptor",
	".talk":  "text/x-speech",
	".uls":   "text/iuls",
	".vcard": "text/vcard",

	// Text - Specialized
	".mel": "text/x-vmel",
	".mrl": "text/x-mrml",
	".r3t": "text/vnd.rn-realtext3d",
	".rt":  "text/vnd.rn-realtext",
	".htc": "text/x-component",

	// Programming & Scripts
	".sh":  "application/x-sh",
	".csh": "application/x-csh",
	".pl":  "application/x-perl",
	".pm":  "application/x-perl",
	".py":  "text/x-python",
	".tcl": "application/x-tcl",
	".php": "application/x-httpd-php",
	".asp": "application/x-asap",
	".cgi": "magnus-internal/cgi",
	".sct": "text/scriptlet",

	// Binary & Executables
	".exe":   "application/x-msdownload",
	".dll":   "application/x-msdownload",
	".so":    "application/x-sharedlib",
	".bin":   "application/octet-stream",
	".class": "application/java-vm",
	".o":     "application/octet-stream",
	".obj":   "application/octet-stream",
	".com":   "application/octet-stream",
	".dmg":   "application/x-apple-diskimage",
	".deb":   "application/vnd.debian.binary-package",
	".rpm":   "application/x-rpm",
	".apk":   "application/vnd.android.package-archive",

	// Java
	".jar":  "application/java-archive",
	".jnlp": "application/x-java-jnlp-file",
	".jwc":  "application/jwc",

	// Adobe & PostScript
	".ai":  "application/postscript",
	".eps": "application/postscript",
	".ps":  "application/postscript",
	".swf": "application/x-shockwave-flash",

	// TeX & LaTeX
	".tex":     "application/x-tex",
	".latex":   "application/x-latex",
	".dvi":     "application/x-dvi",
	".texi":    "application/x-texinfo",
	".texinfo": "application/x-texinfo",

	// Fonts
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".eot":   "application/vnd.ms-fontobject",
	".pfr":   "application/font-tdpfr",

	// 3D Models & CAD
	".dwf":  "drawing/x-dwf",
	".dwg":  "application/x-autocad",
	".dxf":  "application/x-autocad",
	".vrml": "model/vrml",
	".wrl":  "model/vrml",
	".mesh": "model/mesh",
	".msh":  "model/mesh",
	".silo": "model/mesh",
	".iges": "model/iges",
	".igs":  "model/iges",
	".xof":  "x-world/x-vrml",
	".xar":  "x-world/x-vrml",
	".flr":  "x-world/x-vrml",
	".svr":  "x-world/x-svr",
	".vre":  "x-world/x-vream",
	".vrt":  "x-world/x-vrt",
	".vrw":  "x-world/x-vream",
	".wrz":  "x-world/x-vrml",
	".ivr":  "i-world/i-vrml",

	// Chemistry
	".pdb":  "chemical/x-pdb",
	".xyz":  "chemical/x-xyz",
	".csm":  "chemical/x-csml",
	".csml": "chemical/x-csml",
	".emb":  "chemical/x-embl-dl-nucleotide",
	".embl": "chemical/x-embl-dl-nucleotide",
	".gau":  "chemical/x-gaussian-input",
	".mol":  "chemical/x-mdl-molfile",
	".mop":  "chemical/x-mopac-input",

	// Scientific & Data
	".hdf": "application/x-hdf",
	".nc":  "application/x-netcdf",
	".cdf": "application/x-netcdf",

	// E-book
	".epub": "application/epub+zip",
	".ebk":  "application/x-expandedbook",
	".mobi": "application/x-mobipocket-ebook",

	// Apple Specific
	".hqx": "application/mac-binhex40",
	".cpt": "application/mac-compactpro",

	// Symbian
	".sis": "application/vnd.symbian.install",

	// Networking & Configuration
	".mif":   "application/vnd.mif",
	".proxy": "application/x-ns-proxy-autoconfig",

	// Certificates & Security
	".cer": "application/x-x509-ca-cert",
	".crt": "application/x-x509-ca-cert",
	".der": "application/x-x509-ca-cert",
	".p7b": "application/x-pkcs7-certificates",
	".p7c": "application/x-pkcs7-mime",
	".p7m": "application/x-pkcs7-mime",
	".p7r": "application/x-pkcs7-certreqresp",
	".p7s": "application/x-pkcs7-signature",
	".p10": "application/pkcs10",
	".p12": "application/x-pkcs12",
	".pfx": "application/x-pkcs12",
	".crl": "application/pkix-crl",
	".spc": "application/x-pkcs7-certificates",
	".sst": "application/vnd.ms-pkicertstore",
	".stl": "application/vnd.ms-pkistl",
	".cat": "application/vnd.ms-pkiseccat",
	".pko": "application/ynd.ms-pkipko",

	// Microsoft Specific
	".axs": "application/olescript",
	".clp": "application/x-msclip",
	".crd": "application/x-mscardfile",
	".hlp": "application/winhlp",
	".hta": "application/hta",
	".ins": "application/x-internet-signup",
	".isp": "application/x-internet-signup",
	".acx": "application/internet-property-stream",
	".oda": "application/oda",
	".pma": "application/x-perfmon",
	".pmc": "application/x-perfmon",
	".pml": "application/x-perfmon",
	".pmr": "application/x-perfmon",
	".pmw": "application/x-perfmon",
	".prf": "application/pics-rules",
	".scd": "application/x-msschedule",
	".trm": "application/x-msterminal",
	".wri": "application/x-mswrite",
	".wmf": "application/x-msmetafile",
	".wmd": "application/x-ms-wmd",
	".wmz": "application/x-ms-wmz",
	".m13": "application/x-msmediaview",
	".m14": "application/x-msmediaview",
	".mvb": "application/x-msmediaview",

	// Multimedia & Streaming
	".amc":  "application/x-mpeg",
	".mts":  "application/metastream",
	".mtx":  "application/metastream",
	".mtz":  "application/metastream",
	".mzv":  "application/metastream",
	".rtg":  "application/metastream",
	".smi":  "application/smil",
	".smil": "application/smil",
	".spl":  "application/x-futuresplash",
	".ssm":  "application/streamingmedia",
	".vmd":  "application/vocaltec-media-desc",
	".vmf":  "application/vocaltec-media-file",

	// Miscellaneous Application Types
	".aab":    "application/x-authoware-bin",
	".aam":    "application/x-authoware-map",
	".aas":    "application/x-authoware-seg",
	".ani":    "application/octet-stream",
	".asd":    "application/astound",
	".asn":    "application/astound",
	".avb":    "application/octet-stream",
	".bld":    "application/bld",
	".bld2":   "application/bld2",
	".bpk":    "application/octet-stream",
	".ccn":    "application/x-cnc",
	".cco":    "application/x-cocoa",
	".chat":   "application/x-chat",
	".chrt":   "application/x-kchart",
	".co":     "application/x-cult3d-object",
	".cur":    "application/octet-stream",
	".dcr":    "application/x-director",
	".dir":    "application/x-director",
	".dxr":    "application/x-director",
	".etc":    "application/x-earthtime",
	".evy":    "application/envoy",
	".ez":     "application/andrew-inset",
	".fif":    "application/fractals",
	".fm":     "application/x-maker",
	".gca":    "application/x-gca-compressed",
	".gps":    "application/x-gps",
	".gram":   "application/srgs",
	".grxml":  "application/srgs+xml",
	".iii":    "application/x-iphone",
	".ips":    "application/x-ipscript",
	".ipx":    "application/x-ipix",
	".jam":    "application/x-jam",
	".kil":    "application/x-killustrator",
	".kjx":    "application/x-kjx",
	".ksp":    "application/x-kspread",
	".lcc":    "application/fastman",
	".lcl":    "application/x-digitalloca",
	".lcr":    "application/x-digitalloca",
	".lgh":    "application/lgh",
	".man":    "application/x-troff-man",
	".map":    "magnus-internal/imagemap",
	".mbd":    "application/mbedlet",
	".mct":    "application/x-mascot",
	".me":     "application/x-troff-me",
	".mmf":    "application/x-skt-lbs",
	".moc":    "application/x-mocha",
	".mocha":  "application/x-mocha",
	".mof":    "application/x-yumekara",
	".mpc":    "application/vnd.mpohun.certificate",
	".mps":    "application/x-mapserver",
	".mrm":    "application/x-mrm",
	".ms":     "application/x-troff-ms",
	".nar":    "application/zip",
	".ndwn":   "application/ndwn",
	".nif":    "application/x-nif",
	".nmz":    "application/x-scream",
	".npx":    "application/x-netfpx",
	".nva":    "application/x-neva1",
	".nws":    "message/rfc822",
	".oom":    "application/x-AtlasMate-Plugin",
	".pan":    "application/x-pan",
	".pmd":    "application/x-pmd",
	".pqf":    "application/x-cprplayer",
	".pqi":    "application/cprplayer",
	".prc":    "application/x-prc",
	".ptlk":   "application/listenup",
	".rlf":    "application/x-richlink",
	".rnx":    "application/vnd.rn-realplayer",
	".roff":   "application/x-troff",
	".rwc":    "application/x-rogerwilco",
	".sca":    "application/x-supercard",
	".sdf":    "application/e-score",
	".sdp":    "application/sdp",
	".setpay": "application/set-payment-initiation",
	".setreg": "application/set-registration-initiation",
	".shw":    "application/presentations",
	".skd":    "application/x-koan",
	".skm":    "application/x-koan",
	".skp":    "application/x-koan",
	".skt":    "application/x-koan",
	".slc":    "application/x-salsa",
	".smp":    "application/studiom",
	".spr":    "application/x-sprite",
	".sprite": "application/x-sprite",
	".spt":    "application/x-spt",
	".src":    "application/x-wais-source",
	".stk":    "application/hyperstudio",
	".swfl":   "application/x-shockwave-flash",
	".t":      "application/x-troff",
	".tad":    "application/octet-stream",
	".tbp":    "application/x-timbuktu",
	".tbt":    "application/x-timbuktu",
	".thm":    "application/vnd.eri.thm",
	".tki":    "application/x-tkined",
	".tkined": "application/x-tkined",
	".toc":    "application/toc",
	".tr":     "application/x-troff",
	".tsp":    "application/dsptype",
	".ttz":    "application/t-time",
	".vcd":    "application/x-cdlink",
	".vmi":    "application/x-dreamcast-vms-info",
	".vms":    "application/x-dreamcast-vms",
	".vts":    "workbook/formulaone",
	".web":    "application/vnd.xara",
	".wis":    "application/x-InstallShield",
	".wxl":    "application/x-wxl",
	".xdm":    "application/x-xdma",
	".xdma":   "application/x-xdma",
	".xdw":    "application/vnd.fujixerox.docuworks",
	".xll":    "application/x-excel",
	".xpi":    "application/x-xpinstall",
	".xsit":   "text/xml",
	".yz1":    "application/x-yz1",
	".zac":    "application/x-zaurus-zac",

	// Game & Entertainment
	".pgn": "application/x-chess-pgn",
	".ice": "x-conference/x-cooltalk",

	// Specialized Media Formats
	".dcm":     "x-lml/x-evm",
	".evm":     "x-lml/x-evm",
	".gdb":     "x-lml/x-gdb",
	".lak":     "x-lml/x-lak",
	".lml":     "x-lml/x-lml",
	".lmlpack": "x-lml/x-lmlpack",
	".ndb":     "x-lml/x-ndb",
	".rte":     "x-lml/x-gps",
	".trk":     "x-lml/x-gps",
	".wpt":     "x-lml/x-gps",

	// Messages & Mail
	".mht":   "message/rfc822",
	".mhtml": "message/rfc822",
	".eml":   "message/rfc822",

	// Nokia & Mobile Specific
	".nokia-op-logo": "image/vnd.nok-oplogo-color",
}

var (
	extMutex              sync.RWMutex
	extToMimeTypeMappings = make(map[string]*MIME)
)

func init() {
	for ext, mimeTypeString := range extMimetypeStringMappings {
		parsed, err := Parse(mimeTypeString)
		if err != nil {
			panic(err)
		}
		extToMimeTypeMappings[ext] = parsed
	}
}

// StringTypeByExtension returns the MIME type string associated with a file extension.
// It uses both the Go standard library's mime package and an internal mapping to
// determine the appropriate MIME type. If the extension is not recognized,
// it falls back to "application/octet-stream" which is the standard default for
// binary content of unknown type.
//
// This function is thread-safe and uses read locks for concurrent access.
//
// Parameters:
//   - filePath: A file path or filename from which to extract the extension
//
// Examples:
//   - StringTypeByExtension("document.pdf") returns "application/pdf"
//   - StringTypeByExtension("image.png") returns "image/png"
//   - StringTypeByExtension("file.unknown") returns "application/octet-stream"
//
// Returns a string representation of the MIME type associated with the file extension.
func StringTypeByExtension(filePath string) string {
	fileExtension := strings.ToLower(path.Ext(filePath))

	// First try internal mapping with read lock
	extMutex.RLock()
	mimeTypeString := extMimetypeStringMappings[fileExtension]
	extMutex.RUnlock()

	if mimeTypeString != "" {
		return mimeTypeString
	}

	// Fall back to  the standard library's mime package
	mimeTypeString = mime.TypeByExtension(fileExtension)
	if mimeTypeString != "" {
		return mimeTypeString
	}

	mimeTypeString = "application/octet-stream"

	return mimeTypeString
}

// TypeByExtension returns a MIME object for the given file path or filename.
// It extracts the extension from the file path and looks it up in an internal
// mapping of extensions to MIME types. This provides a way to determine the likely
// MIME type of a file based on its extension without examining the file's contents.
//
// This function is thread-safe and uses read locks for concurrent access.
// The returned MIME object is a clone to ensure immutability.
//
// Parameters:
//   - filePath: The file path or filename from which to extract the extension
//
// Examples:
//   - TypeByExtension("document.html") returns a MIME object for "text/html"
//   - TypeByExtension("images/photo.jpg") returns a MIME object for "image/jpeg"
//   - TypeByExtension("/path/to/data.json") returns a MIME object for "application/json"
//
// Returns a MIME type object and a boolean indicating if the extension was recognized.
// If the extension is not recognized, returns nil and false.
func TypeByExtension(filePath string) (*MIME, bool) {
	fileExtension := strings.ToLower(path.Ext(filePath))

	extMutex.RLock()
	mappedMime, extensionFound := extToMimeTypeMappings[fileExtension]
	extMutex.RUnlock()

	if extensionFound {
		return mappedMime.Clone(), true
	}

	return nil, false
}

// RegisterExtension registers a new file extension with its corresponding MIME type.
// It updates both the string mapping and parsed MIME type mapping.
// If the extension already exists, it will be overwritten.
//
// This function is thread-safe and uses write locks to ensure data consistency.
//
// Parameters:
//   - ext: The file extension (should include the leading dot, e.g., ".json")
//   - mimeType: The MIME type string (e.g., "application/json")
//
// Returns:
//   - error: Returns an error if the MIME type string is invalid
//
// Example:
//
//	err := RegisterExtension(".custom", "application/x-custom")
//	if err != nil {
//	    log.Printf("Failed to register extension: %v", err)
//	}
func RegisterExtension(ext, mimeType string) error {
	parsedMime, err := Parse(mimeType)
	if err != nil {
		return fmt.Errorf("invalid MIME type %q: %w", mimeType, err)
	}

	extMutex.Lock()
	extMimetypeStringMappings[ext] = mimeType
	extToMimeTypeMappings[ext] = parsedMime
	extMutex.Unlock()

	return nil
}

// RegisterExtensions registers multiple file extensions with their corresponding MIME types.
// It's a batch version of RegisterExtension for convenience.
//
// This function is thread-safe. It acquires a single write lock for the entire batch
// operation to ensure atomicity and better performance than multiple individual calls.
//
// Parameters:
//   - mappings: A map of file extensions to MIME type strings
//
// Returns:
//   - error: Returns the first error encountered, if any. If an error occurs,
//     no mappings from this batch will be registered (all-or-nothing behavior).
//
// Example:
//
//	mappings := map[string]string{
//	    ".custom1": "application/x-custom1",
//	    ".custom2": "application/x-custom2",
//	}
//	err := RegisterExtensions(mappings)
func RegisterExtensions(mappings map[string]string) error {
	// Pre-parse all MIME types before acquiring the lock
	parsedMimes := make(map[string]*MIME, len(mappings))
	for ext, mimeType := range mappings {
		parsedMime, err := Parse(mimeType)
		if err != nil {
			return fmt.Errorf("invalid MIME type %q for extension %q: %w", mimeType, ext, err)
		}
		parsedMimes[ext] = parsedMime
	}

	extMutex.Lock()
	for ext, mimeType := range mappings {
		extMimetypeStringMappings[ext] = mimeType
		extToMimeTypeMappings[ext] = parsedMimes[ext]
	}
	extMutex.Unlock()

	return nil
}
