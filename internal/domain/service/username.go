package service

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxUsernameRunes = 64

func normalizeAndValidateUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if username == "" {
		return "", fmt.Errorf("用户名不能为空")
	}
	if !utf8.ValidString(username) {
		return "", fmt.Errorf("用户名包含无效字符")
	}
	if utf8.RuneCountInString(username) > maxUsernameRunes {
		return "", fmt.Errorf("用户名不能超过 %d 个字符", maxUsernameRunes)
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("用户名不能包含控制字符")
		}
	}
	return username, nil
}
