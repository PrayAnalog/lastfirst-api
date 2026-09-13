package ytinput

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalid     = errors.New("not a YouTube playlist URL or ID")
	ErrUnsupported = errors.New("mixes, Watch Later, Liked videos and temporary lists are not supported")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func Parse(input string) (string, error) {
	id := strings.TrimSpace(input)
	if strings.ContainsAny(id, "/?") {
		u, err := url.Parse(id)
		if err != nil {
			return "", ErrInvalid
		}
		id = u.Query().Get("list")
	}
	if !idPattern.MatchString(id) {
		return "", ErrInvalid
	}
	if id == "WL" || id == "LL" || strings.HasPrefix(id, "RD") || strings.HasPrefix(id, "TL") {
		return "", ErrUnsupported
	}
	return id, nil
}
