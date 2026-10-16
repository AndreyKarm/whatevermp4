package storage

import (
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	seRe        = regexp.MustCompile(`(?i)s(\d{1,2})[ ._-]?e(\d{1,3})`)
	xRe         = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
)

func SafeName(s string) string {
	s = unsafeChars.ReplaceAllString(filepath.Base(s), "_")
	if s == "" || s == "." {
		return "file"
	}
	return s
}

func ParseSE(name string) (season, ep *int) {
	m := seRe.FindStringSubmatch(name)
	if m == nil {
		m = xRe.FindStringSubmatch(name)
	}
	if m == nil {
		return nil, nil
	}
	s, _ := strconv.Atoi(m[1])
	e, _ := strconv.Atoi(m[2])
	return &s, &e
}
