package seed

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

//go:embed artifacts/v1.json
var artifactV1Bytes []byte

type ServiceSeed struct {
	Code            string `json:"code"`
	DisplayName     string `json:"displayName"`
	DurationMinutes int16  `json:"durationMinutes"`
}

type ProfessionalSeed struct {
	Code           string   `json:"code"`
	DisplayName    string   `json:"displayName"`
	Qualifications []string `json:"qualifications"`
}

type Artifact struct {
	Version       string             `json:"version"`
	Services      []ServiceSeed      `json:"services"`
	Professionals []ProfessionalSeed `json:"professionals"`
}

// Checksum returns the SHA-256 hex digest of the artifact's raw embedded
// bytes — an operator can independently reproduce this with
// `sha256sum internal/service/seed/artifacts/vN.json`.
func Checksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LoadArtifactV1 parses the embedded v1 artifact and returns it along with
// the checksum computed over its raw bytes.
func LoadArtifactV1() (Artifact, string, error) {
	var a Artifact
	if err := json.Unmarshal(artifactV1Bytes, &a); err != nil {
		return Artifact{}, "", fmt.Errorf("parse embedded artifact v1: %w", err)
	}
	return a, Checksum(artifactV1Bytes), nil
}
