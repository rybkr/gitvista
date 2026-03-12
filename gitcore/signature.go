package gitcore

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	signatureRe = regexp.MustCompile("[<>]")
)

// Signature represents the author or committer of a Git commit.
// See: https://git-scm.com/docs/signature-format
type Signature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	When  time.Time `json:"when"`
}

// NewSignature parses a Git signature line: "Name <email> unix-timestamp timezone".
func NewSignature(line string) (Signature, error) {
	parts := signatureRe.Split(line, -1)
	if len(parts) != 3 {
		return Signature{}, fmt.Errorf("invalid signature line: %q", line)
	}

	name := strings.TrimSpace(parts[0])
	email := strings.TrimSpace(parts[1])

	timePart := strings.TrimSpace(parts[2])
	timeFields := strings.Fields(timePart)
	if timePart == "" || len(timeFields) == 0 {
		return Signature{}, fmt.Errorf("invalid signature line: missing timestamp: %q", line)
	}

	var unixTime int64
	if _, err := fmt.Sscanf(timeFields[0], "%d", &unixTime); err != nil {
		return Signature{}, fmt.Errorf("invalid signature line: invalid timestamp: %q", line)
	}

	var loc *time.Location
	if len(timeFields) >= 2 {
		loc = parseTimezone(timeFields[1])
	}
	if loc == nil {
		loc = time.UTC
	}

	return Signature{
		Name:  name,
		Email: email,
		When:  time.Unix(unixTime, 0).In(loc),
	}, nil
}

func parseTimezone(tz string) *time.Location {
	if len(tz) != 5 {
		return nil
	}
	sign := 1
	if tz[0] == '-' {
		sign = -1
	} else if tz[0] != '+' {
		return nil
	}
	hours, err := strconv.Atoi(tz[1:3])
	if err != nil {
		return nil
	}
	mins, err := strconv.Atoi(tz[3:5])
	if err != nil {
		return nil
	}
	offset := sign * (hours*3600 + mins*60)
	return time.FixedZone(tz, offset)
}
