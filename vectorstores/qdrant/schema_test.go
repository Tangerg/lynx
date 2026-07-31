package qdrant

import (
	"errors"
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"
)

func TestValidateCollectionSchema(t *testing.T) {
	t.Parallel()

	valid := collectionInfo(qdrantclient.Distance_Cosine, 3)
	if err := validateCollectionSchema(valid, 3, qdrantclient.Distance_Cosine); err != nil {
		t.Fatalf("validate compatible collection: %v", err)
	}

	tests := map[string]struct {
		info       *qdrantclient.CollectionInfo
		dimensions int
		distance   qdrantclient.Distance
	}{
		"missing config":    {info: &qdrantclient.CollectionInfo{}, dimensions: 3, distance: qdrantclient.Distance_Cosine},
		"invalid dimension": {info: valid, dimensions: 0, distance: qdrantclient.Distance_Cosine},
		"dimension mismatch": {
			info: collectionInfo(qdrantclient.Distance_Cosine, 4), dimensions: 3, distance: qdrantclient.Distance_Cosine,
		},
		"metric mismatch": {
			info: collectionInfo(qdrantclient.Distance_Dot, 3), dimensions: 3, distance: qdrantclient.Distance_Cosine,
		},
		"named vectors": {
			info: &qdrantclient.CollectionInfo{Config: &qdrantclient.CollectionConfig{
				Params: &qdrantclient.CollectionParams{VectorsConfig: &qdrantclient.VectorsConfig{
					Config: &qdrantclient.VectorsConfig_ParamsMap{ParamsMap: &qdrantclient.VectorParamsMap{
						Map: map[string]*qdrantclient.VectorParams{"dense": {Size: 3, Distance: qdrantclient.Distance_Cosine}},
					}},
				}},
			}},
			dimensions: 3,
			distance:   qdrantclient.Distance_Cosine,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateCollectionSchema(test.info, test.dimensions, test.distance); !errors.Is(err, ErrIncompatibleCollection) {
				t.Fatalf("validateCollectionSchema() error = %v, want ErrIncompatibleCollection", err)
			}
		})
	}
}

func collectionInfo(distance qdrantclient.Distance, dimensions uint64) *qdrantclient.CollectionInfo {
	return &qdrantclient.CollectionInfo{Config: &qdrantclient.CollectionConfig{
		Params: &qdrantclient.CollectionParams{VectorsConfig: qdrantclient.NewVectorsConfig(&qdrantclient.VectorParams{
			Size: dimensions, Distance: distance,
		})},
	}}
}
