// Package idgen provides the application-generated, CSPRNG-based UUID
// generator required by backend/README.md's Canonical Types ("UUID 由
// application 透過可注入、使用 CSPRNG 的 IDGenerator 產生").
package idgen

import (
	"fmt"

	"github.com/google/uuid"
)

type Generator struct{}

func New() Generator { return Generator{} }

// NewID returns an RFC 9562 lowercase-hyphenated UUID v4 string.
// uuid.NewRandom reads from crypto/rand internally.
func (Generator) NewID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	return id.String(), nil
}
