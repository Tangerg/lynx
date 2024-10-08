package document

type MetadataMode string

const (
	All       MetadataMode = "all"
	Embed     MetadataMode = "embed"
	Inference MetadataMode = "inference"
	None      MetadataMode = "none"
)

type ContentFormatter interface {
	Format(document *Document, mode MetadataMode) string
}
