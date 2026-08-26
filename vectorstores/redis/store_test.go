package redis_test

import (
	"testing"

	"github.com/Tangerg/lynx/vectorstores/redis"
)

func TestMetadataFieldOwnsItsSchemaContract(t *testing.T) {
	valid := redis.MetadataField{Name: "profile.author", Type: redis.FieldTag}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid field: %v", err)
	}

	for _, field := range []redis.MetadataField{
		{Name: "author"},
		{Name: " author", Type: redis.FieldText},
	} {
		if err := field.Validate(); err == nil {
			t.Fatalf("invalid field accepted: %+v", field)
		}
	}
}

func TestMetadataFieldTypeHasStableTextIdentity(t *testing.T) {
	tests := []struct {
		fieldType redis.MetadataFieldType
		want      string
	}{
		{fieldType: redis.FieldTag, want: "TAG"},
		{fieldType: redis.FieldText, want: "TEXT"},
		{fieldType: redis.FieldNumeric, want: "NUMERIC"},
	}
	for _, test := range tests {
		if !test.fieldType.Valid() || test.fieldType.String() != test.want {
			t.Fatalf("field type = %q, valid = %t; want %q", test.fieldType, test.fieldType.Valid(), test.want)
		}
	}
	if redis.MetadataFieldType("").Valid() {
		t.Fatal("zero MetadataFieldType is valid")
	}
}
