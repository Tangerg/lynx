package s3vectors

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3vdoc "github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
)

func TestDecodeDocumentMetadataSeparatesOwnedContent(t *testing.T) {
	text, encoded, err := decodeDocumentMetadata(s3vdoc.NewLazyDocument(map[string]any{
		contentMetaKey: "hello",
		"tenant":       "acme",
	}))
	if err != nil {
		t.Fatalf("decodeDocumentMetadata: %v", err)
	}
	values, err := encoded.Values()
	if err != nil {
		t.Fatalf("metadata Values: %v", err)
	}
	if text != "hello" || len(values) != 1 || values["tenant"] != "acme" {
		t.Fatalf("text = %q, metadata = %#v", text, values)
	}
}

func TestDecodeDocumentMetadataRejectsMalformedContent(t *testing.T) {
	_, _, err := decodeDocumentMetadata(s3vdoc.NewLazyDocument(map[string]any{contentMetaKey: 42}))
	if err == nil || !strings.Contains(err.Error(), "must be a non-empty string") {
		t.Fatalf("decodeDocumentMetadata error = %v", err)
	}
}

func TestQueryVectorKeysRejectsMissingKey(t *testing.T) {
	_, err := queryVectorKeys([]types.QueryOutputVector{{Key: aws.String("doc-1")}, {}})
	if err == nil || !strings.Contains(err.Error(), "result[1] is missing key") {
		t.Fatalf("queryVectorKeys error = %v", err)
	}
}
