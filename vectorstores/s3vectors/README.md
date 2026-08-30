# s3vectors

Package s3vectors exposes AWS S3 Vectors through the Core vector-store
capability interfaces. S3 Vectors is a purpose-built, fully managed vector
storage tier that lives next to regular S3 buckets — vectors live in a *vector
bucket* under a typed *vector index*. Requirements: an AWS account with S3
Vectors enabled (currently available in a subset of regions), a vector bucket +
index provisioned out of band (ARM / Terraform / CloudFormation / SDK control
plane), and an aws-sdk-go-v2 s3vectors client. The store does NOT create
indexes — index dimensionality / metric / metadata schema are declared at index
creation. Distance metrics. S3 Vectors indexes are registered with one of
`cosine` or `euclidean` at creation. StoreConfig.DistanceMetric must match the
index's registered metric — the store uses it to map QueryVectors' raw distance
into a higher-is-better score in [0, 1]. Filter visitor produces S3 Vectors'
Mongo-flavored JSON filter document — `{"author": {"$eq": "Alice"}}`, `{"year":
{"$gte": 2020}}`, `{"$and": [...]}`, `{"$not": {...}}`. Metadata keys are
addressed flat (no nested-path support). Batching. PutVectors caps at 500
vectors per request; the document batcher should produce shards smaller than
that. The store passes each shard through as one PutVectors call. Delete. S3
Vectors has no filter-based DeleteVectors — the store enumerates ids via paged
QueryVectors (1000 per page using a zero probe vector since the distance is
discarded) and then issues a DeleteVectors batch. See
https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-vectors.html.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/s3vectors
```

## Constructors

Every constructor validates its config and returns a value implementing
the capability interfaces in `core/vectorstore`:

- `NewStore`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is
`core/vectorstore/storetest` — this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
