package main

import (
	"fmt"
	"os"
)

// Read once before scheduling requests, avoiding the command-line size limit.
func readBody(body, path string) (string, error) {
	if path == "" {
		return body, nil
	}
	if body != "" {
		return "", fmt.Errorf("-body 与 -body-file 不能同时使用")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取请求体文件: %w", err)
	}
	return string(data), nil
}
