package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	maxContactQRImageBytes     = 300 * 1024
	maxContactQRImageURLLength = 2048
	maxContactQRDataURLLength  = 450 * 1024 // generous envelope for 300 KiB base64 payload + header
)

// NormalizeSiteDomain validates and normalizes a canonical site host (optional port,
// no scheme, path, query, fragment, or userinfo). Empty input returns ("", nil).
func NormalizeSiteDomain(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("site domain must not include http:// or https://")
	}
	value = strings.TrimRight(value, "/")
	u, err := url.Parse("//" + value)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("site domain is invalid")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", fmt.Errorf("site domain must not contain a path, query, fragment, or user info")
	}
	return strings.ToLower(u.Host), nil
}

func normalizeSiteDomain(value string) (string, error) {
	return NormalizeSiteDomain(value)
}

// GetSiteDomain returns the configured canonical site domain. An absent setting
// keeps the legacy behavior and disables host enforcement.
func (s *SettingService) GetSiteDomain(ctx context.Context) (string, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySiteDomain)
	if errors.Is(err, ErrSettingNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get site domain: %w", err)
	}
	value, err = NormalizeSiteDomain(value)
	if err != nil {
		return "", fmt.Errorf("invalid stored site domain: %w", err)
	}
	return value, nil
}

// ValidateContactQRImageURL validates and trims contact_qr_image_url.
// Empty values are allowed. Non-empty values must be an http(s) URL or a
// data:image/*;base64 payload within the 300 KiB decoded size limit.
func ValidateContactQRImageURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "data:image/") {
		if len(value) > maxContactQRDataURLLength {
			return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR image exceeds 300 KB limit")
		}
		commaIdx := strings.IndexByte(value, ',')
		if commaIdx < 0 || !strings.Contains(lower[:commaIdx], ";base64") {
			return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR data URL must be base64-encoded image data")
		}
		payload := value[commaIdx+1:]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(payload)
			if err != nil {
				return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR image base64 data is invalid")
			}
		}
		if len(decoded) == 0 || len(decoded) > maxContactQRImageBytes {
			return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR image exceeds 300 KB limit")
		}
		return value, nil
	}
	if len(value) > maxContactQRImageURLLength {
		return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR image URL is too long")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", infraerrors.BadRequest("INVALID_CONTACT_QR_IMAGE", "contact QR image must be an http(s) URL or base64 data:image URL")
	}
	return value, nil
}
