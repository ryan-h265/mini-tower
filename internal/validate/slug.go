package validate

import (
	"errors"
	"regexp"
)

var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

var reservedSlugs = map[string]bool{
	"api":      true,
	"admin":    true,
	"system":   true,
	"internal": true,
	"default":  true,
}

var (
	ErrSlugEmpty    = errors.New("slug cannot be empty")
	ErrSlugTooShort = errors.New("slug must be at least 3 characters")
	ErrSlugTooLong  = errors.New("slug must be at most 32 characters")
	ErrSlugFormat   = errors.New("slug must start with a letter and contain only lowercase letters, numbers, and hyphens")
	ErrSlugReserved = errors.New("slug is reserved")
)

// ValidateSlug validates a team or app slug.
func ValidateSlug(s string) error {
	if s == "" {
		return ErrSlugEmpty
	}

	if len(s) < 3 {
		return ErrSlugTooShort
	}

	if len(s) > 32 {
		return ErrSlugTooLong
	}

	if !slugRegex.MatchString(s) {
		return ErrSlugFormat
	}

	if reservedSlugs[s] {
		return ErrSlugReserved
	}

	return nil
}
