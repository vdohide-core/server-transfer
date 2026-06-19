package utils

import (
	"math/rand"
	"time"
)

const alphanumChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func init() {
	rand.Seed(time.Now().UnixNano())
}

func RandomAlphaNum(length int) string {
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	for i := range b {
		b[i] = alphanumChars[rand.Intn(len(alphanumChars))]
	}
	return string(b)
}

func RandomStringSpecial(length int) string {
	if length <= 0 {
		return ""
	}
	if length < 3 {
		return RandomAlphaNum(length)
	}
	base := RandomAlphaNum(length)
	runes := []rune(base)
	dashPos := rand.Intn(length-2) + 1
	underscorePos := rand.Intn(length-2) + 1
	for dashPos == underscorePos {
		underscorePos = rand.Intn(length-2) + 1
	}
	result := make([]rune, 0, length+2)
	for i, r := range runes {
		if i == dashPos {
			result = append(result, '-')
		}
		if i == underscorePos {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return string(result)
}
