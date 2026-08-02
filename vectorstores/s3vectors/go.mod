module github.com/Tangerg/lynx/vectorstores/s3vectors

go 1.26.5

require (
	github.com/Tangerg/lynx/core v0.0.0-20260802201325-86ac84535c5e
	github.com/Tangerg/lynx/embeddingclient v0.0.0-20260731193916-0098789d89e9
	github.com/Tangerg/lynx/internal/vectorstorekit v0.0.0-20260802201617-1225d4100cab
	github.com/aws/aws-sdk-go-v2 v1.43.2
	github.com/aws/aws-sdk-go-v2/service/s3vectors v1.10.2
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.33 // indirect
	github.com/aws/smithy-go v1.27.5 // indirect
)
