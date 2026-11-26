package commands

import (
	"strconv"
	"strings"
)

// compareVersions 比较两段点分版本号；忽略 v 前缀和预发布后缀，缺失的分段按 0 处理。
func compareVersions(left, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	for index := 0; index < len(leftParts) || index < len(rightParts); index++ {
		var leftValue, rightValue int
		if index < len(leftParts) {
			leftValue = leftParts[index]
		}
		if index < len(rightParts) {
			rightValue = rightParts[index]
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

func versionParts(value string) []int {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if index := strings.IndexAny(value, "-+"); index >= 0 {
		value = value[:index]
	}
	if value == "" {
		return nil
	}
	fields := strings.Split(value, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil {
			return parts
		}
		parts = append(parts, number)
	}
	return parts
}
