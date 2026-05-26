package base62

import "strings"

const Charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Encode 将 uint64 数字转换为 Base62 字符串
func Encode(num uint64) string {
	if num == 0 {
		return string(Charset[0])
	}

	var result strings.Builder
	for num > 0 {
		remainder := num % 62
		result.WriteByte(Charset[remainder])
		num /= 62
	}

	// 反转字符串（因为从低位算起的）
	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}
