module github.com/Tangerg/lynx/vectorstores/cockroachdb

go 1.26.5

require (
	github.com/Tangerg/lynx/core v0.0.0-20260802201325-86ac84535c5e
	github.com/Tangerg/lynx/embeddingclient v0.0.0-20260731193916-0098789d89e9
	github.com/Tangerg/lynx/internal/vectorstorekit v0.0.0-20260802201617-1225d4100cab
	github.com/Tangerg/lynx/internal/vectorstorepg v0.0.0-20260802201738-83da29e02f49
	github.com/jackc/pgx/v5 v5.10.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/pgvector/pgvector-go v0.4.1 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
