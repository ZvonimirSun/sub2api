package service

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

const (
	DefaultCodexTurnStateTTLSeconds = 3600
	MaxCodexTurnStateBytes          = 8192
)

var (
	ErrEmptyTurnState      = errors.New("empty turn-state")
	ErrTurnStateTooLong    = errors.New("turn-state exceeds max allowed length")
	ErrInvalidTurnStateB64 = errors.New("invalid base64 encoding in turn-state")
	ErrNotFernetTurnState  = errors.New("turn-state does not match Fernet specification")
	ErrIllegalHeaderChars  = errors.New("turn-state contains characters illegal in HTTP header")
)

// CodexTurnStateInspection 包含对 Turn-State 原文解析后的元数据。
type CodexTurnStateInspection struct {
	Valid            bool      `json:"valid"`
	Length           int       `json:"length"`
	RawState         string    `json:"raw_state,omitempty"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	RemainingSeconds int       `json:"remaining_seconds"`
	AgeSeconds       int       `json:"age_seconds"`
	IsExpired        bool      `json:"is_expired"`
	Error            string    `json:"error,omitempty"`
}

// InspectCodexTurnState 解析并校验 Codex Turn-State（遵循 Fernet 规范前缀）。
func InspectCodexTurnState(state string) (*CodexTurnStateInspection, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return &CodexTurnStateInspection{Valid: false, Length: 0, Error: ErrEmptyTurnState.Error()}, ErrEmptyTurnState
	}
	if len(state) > MaxCodexTurnStateBytes {
		return &CodexTurnStateInspection{Valid: false, Length: len(state), RawState: state, Error: ErrTurnStateTooLong.Error()}, ErrTurnStateTooLong
	}
	if !httpguts.ValidHeaderFieldValue(state) {
		return &CodexTurnStateInspection{Valid: false, Length: len(state), RawState: state, Error: ErrIllegalHeaderChars.Error()}, ErrIllegalHeaderChars
	}

	// 补全 URL-Safe Base64 Padding
	padded := state
	if m := len(state) % 4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}

	raw, err := base64.URLEncoding.DecodeString(padded)
	if err != nil {
		// 容错：有些实现可能使用标准 Base64 编码
		raw, err = base64.StdEncoding.DecodeString(padded)
		if err != nil {
			return &CodexTurnStateInspection{Valid: false, Length: len(state), Error: ErrInvalidTurnStateB64.Error()}, ErrInvalidTurnStateB64
		}
	}

	// Fernet 格式校验：首字节必须是 0x80，后续 8 字节为大端时间戳
	if len(raw) < 9 || raw[0] != 0x80 {
		return &CodexTurnStateInspection{Valid: false, Length: len(state), Error: ErrNotFernetTurnState.Error()}, ErrNotFernetTurnState
	}

	sec := binary.BigEndian.Uint64(raw[1:9])
	issuedAt := time.Unix(int64(sec), 0).UTC()
	expiresAt := issuedAt.Add(time.Duration(DefaultCodexTurnStateTTLSeconds) * time.Second)
	now := time.Now().UTC()

	remaining := int(expiresAt.Sub(now).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	age := int(now.Sub(issuedAt).Seconds())
	if age < 0 {
		age = 0
	}

	return &CodexTurnStateInspection{
		Valid:            true,
		Length:           len(state),
		RawState:         state,
		IssuedAt:         issuedAt,
		ExpiresAt:        expiresAt,
		RemainingSeconds: remaining,
		AgeSeconds:       age,
		IsExpired:        now.After(expiresAt),
	}, nil
}
