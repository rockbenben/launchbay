package httpapi

import (
	"encoding/base64"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/rockbenben/launchbay/internal/apierr"
	"github.com/rockbenben/launchbay/internal/model"
)

var (
	fallbackHostShape   = regexp.MustCompile(`^(?:[a-z0-9-]+\.)+[a-z][a-z0-9-]*$`)
	fallbackLocalSuffix = regexp.MustCompile(`\.(?:arpa|corp|example|home|i2p|internal|invalid|lan|local|localhost|onion|test)$`)
)

func (s *Server) handleIconFallback(w http.ResponseWriter, r *http.Request) {
	profile, _, ok := s.require(r, credSession, "")
	if !ok {
		s.writeAuthFailure(w, r)
		return
	}
	if s.profileDisabledRefuse(w, profile) {
		return
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "body", Reason: "请求必须是 application/json"}})
		return
	}
	var in struct {
		Host string `json:"host"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, decodeFieldErrors(err))
		return
	}
	host := strings.ToLower(strings.TrimSuffix(in.Host, "."))
	if len(host) > 253 || !fallbackHostShape.MatchString(host) || fallbackLocalSuffix.MatchString(host) ||
		model.LANShapeOf(host, s.deps.Store.LANHosts()).Mode == model.LinkLAN {
		writeFieldErrors(w, http.StatusUnprocessableEntity, []fieldError{{Kind: apierr.FieldInvalid, Field: "host", Reason: "备用图标需要公网域名"}})
		return
	}

	data, ext, err := s.deps.Fetch.FallbackImage(r.Context(), host)
	media := iconContentTypes[ext]
	if err != nil || len(data) == 0 || media == "" {
		writeJSON(w, http.StatusBadGateway, errorResponse{Code: 502, Kind: apierr.TargetUnreachable, Detail: "备用服务未能提供网站图标"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Image string `json:"image"`
	}{Image: "data:" + media + ";base64," + base64.StdEncoding.EncodeToString(data)})
}
