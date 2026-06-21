package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"credential-priority/internal/config"
)

// Envelope 是 CPA C ABI 使用的 JSON 信封。
type Envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *EnvelopeError  `json:"error,omitempty"`
}

// EnvelopeError 是失败 JSON 信封中的错误对象。
type EnvelopeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func decodeRegisterRequest(raw []byte) (RegisterRequest, error) {
	configYAML, err := decodeConfigYAML(raw)
	if err != nil {
		return RegisterRequest{}, err
	}
	return RegisterRequest{ConfigYAML: configYAML}, nil
}

func decodeReconfigureRequest(raw []byte) (ReconfigureRequest, error) {
	configYAML, err := decodeConfigYAML(raw)
	if err != nil {
		return ReconfigureRequest{}, err
	}
	return ReconfigureRequest{ConfigYAML: configYAML}, nil
}

func decodeConfigYAML(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	// 官方 Go SDK 将 []byte 形式的 config_yaml 编码为 base64 字符串；先按该形态解码，失败时再兼容旧的明文字符串形态。
	var bytesRequest struct {
		ConfigYAML []byte `json:"config_yaml"`
	}
	if err := json.Unmarshal(raw, &bytesRequest); err == nil {
		return string(bytesRequest.ConfigYAML), nil
	}
	var stringRequest struct {
		ConfigYAML string `json:"config_yaml"`
	}
	if err := json.Unmarshal(raw, &stringRequest); err != nil {
		return "", fmt.Errorf("%w: decode json: %v", ErrInvalidRequest, err)
	}
	return stringRequest.ConfigYAML, nil
}

func envelopeRegister(result RegisterResult, err error) []byte {
	if err != nil {
		return failure(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return failure(fmt.Errorf("encode result: %w", err))
	}
	return mustMarshal(Envelope{OK: true, Result: encoded})
}

func envelopeStatus(err error) []byte {
	if err != nil {
		return failure(err)
	}
	return mustMarshal(Envelope{OK: true, Result: json.RawMessage(`{"status":"ok"}`)})
}

func failure(err error) []byte {
	return mustMarshal(Envelope{OK: false, Error: envelopeError(err)})
}

func envelopeError(err error) *EnvelopeError {
	code, retryable := "internal_error", false
	switch {
	case errors.Is(err, ErrInvalidRequest):
		code = "invalid_request"
	case errors.Is(err, config.ErrInvalidConfig):
		code = "invalid_config"
	case errors.Is(err, ErrRunInProgress):
		code, retryable = "run_in_progress", true
	case errors.Is(err, ErrShutdown):
		code = "shutdown"
	}
	return &EnvelopeError{Code: code, Message: err.Error(), Retryable: retryable}
}

func mustMarshal(envelope Envelope) []byte {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"internal_error","message":"encode envelope failed","retryable":false}}`)
	}
	return encoded
}
