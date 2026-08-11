package terminal

import "github.com/Tangerg/oolong/core/layout"

const (
	minimumFormDialogHeight = 8
	formDialogFrameRows     = 2
)

func formDialogHeight(contentRows, fieldCount, maximum int) int {
	withValidation := layout.Sum(contentRows, fieldCount, formDialogFrameRows)
	return min(maximum, max(minimumFormDialogHeight, withValidation))
}
