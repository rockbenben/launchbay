package store

import (
	"errors"
	"strings"
	"testing"
)

func TestStoredJSONRejectsTrailingContentAndInvalidUTF8(t *testing.T) {
	for _, fixture := range []struct{ name, raw string }{
		{"bookmarks", dadBookmarks}, {"profile", dadProfile},
	} {
		for _, suffix := range []string{"{}", "null", "garbage"} {
			t.Run(fixture.name+suffix, func(t *testing.T) {
				raw := []byte(fixture.raw + suffix)
				var err error
				if fixture.name == "bookmarks" {
					_, err = DecodeBookmarks("bookmarks.json", raw)
				} else {
					_, err = decodeProfile("profile.json", raw)
				}
				if !errors.Is(err, ErrCorrupt) {
					t.Errorf("must reject trailing content as corrupt: %v", err)
				}
			})
		}
		bad := strings.Replace(fixture.raw, "爸爸", string([]byte{0xff}), 1)
		var err error
		if fixture.name == "bookmarks" {
			_, err = DecodeBookmarks("bookmarks.json", []byte(bad))
		} else {
			_, err = decodeProfile("profile.json", []byte(bad))
		}
		if !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: invalid UTF-8 must be corrupt: %v", fixture.name, err)
		}
	}
}
