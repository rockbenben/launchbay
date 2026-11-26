package model

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
)

const UploadedIconPrefix = "data:image/png;base64,"

const (
	maxUploadedIconBytes = 20 << 10
	maxUploadedIconSide  = 64
)

// ValidUploadedIcon checks the PNG stored on the bookmark. Bound the decoded
// dimensions before decompressing and require an unbroken, canonical base64 value.
func ValidUploadedIcon(icon string) bool {
	encoded, ok := strings.CutPrefix(icon, UploadedIconPrefix)
	if !ok || len(encoded) > base64.StdEncoding.EncodedLen(maxUploadedIconBytes) {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > maxUploadedIconBytes || base64.StdEncoding.EncodeToString(data) != encoded {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width > maxUploadedIconSide || cfg.Height > maxUploadedIconSide {
		return false
	}
	_, err = png.Decode(bytes.NewReader(data))
	return err == nil
}
