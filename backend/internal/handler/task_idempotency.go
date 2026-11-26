package handler

import (
	"errors"
	"strings"
)

func resolveIdempotencyKey(headerKey, bodyKey string) (string, error) {
	headerKey = strings.TrimSpace(headerKey)
	bodyKey = strings.TrimSpace(bodyKey)
	if headerKey == "" {
		return bodyKey, nil
	}
	if bodyKey != "" && bodyKey != headerKey {
		return "", errors.New("Idempotency-Key 与请求体中的 idempotencyKey 不一致")
	}
	return headerKey, nil
}
