package skills

import "fmt"

const (
	DefaultMaxRepositoryEntries = 512
	DefaultMaxFrontmatterBytes  = int64(64 * 1024)
	DefaultMaxSkillBytes        = int64(1024 * 1024)
	DefaultMaxResourceBytes     = int64(1024 * 1024)
	maxBoundedReadBytes         = int64(^uint64(0)>>1) - 1
)

// RepositoryConfig bounds repository discovery and skill-document reads.
// Zero fields select the package defaults.
type RepositoryConfig struct {
	MaxEntries          int
	MaxFrontmatterBytes int64
	MaxSkillBytes       int64
}

type repositoryLimits struct {
	maxEntries          int
	maxFrontmatterBytes int64
	maxSkillBytes       int64
}

func (config RepositoryConfig) resolve() (repositoryLimits, error) {
	if config.MaxEntries < 0 || config.MaxFrontmatterBytes < 0 || config.MaxSkillBytes < 0 {
		return repositoryLimits{}, fmt.Errorf("%w: repository limits must not be negative", ErrInvalidLimit)
	}
	if config.MaxFrontmatterBytes > maxBoundedReadBytes || config.MaxSkillBytes > maxBoundedReadBytes {
		return repositoryLimits{}, fmt.Errorf("%w: byte limits are too large", ErrInvalidLimit)
	}
	limits := repositoryLimits{
		maxEntries:          config.MaxEntries,
		maxFrontmatterBytes: config.MaxFrontmatterBytes,
		maxSkillBytes:       config.MaxSkillBytes,
	}
	if limits.maxEntries == 0 {
		limits.maxEntries = DefaultMaxRepositoryEntries
	}
	if limits.maxFrontmatterBytes == 0 {
		limits.maxFrontmatterBytes = DefaultMaxFrontmatterBytes
	}
	if limits.maxSkillBytes == 0 {
		limits.maxSkillBytes = DefaultMaxSkillBytes
	}
	if limits.maxFrontmatterBytes > limits.maxSkillBytes {
		return repositoryLimits{}, fmt.Errorf("%w: frontmatter limit exceeds skill limit", ErrInvalidLimit)
	}
	return limits, nil
}
