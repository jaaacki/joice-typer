package logging

import (
	"os"
	"strings"
)

func ReadLogTail(path string, maxLines int) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if len(content) == 0 {
		return "", false, nil
	}

	if maxLines <= 0 {
		return "", true, nil
	}

	trailingNewline := content[len(content)-1] == '\n'
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[len(lines)-maxLines:]
	}

	result := strings.Join(lines, "\n")
	if trailingNewline {
		result += "\n"
	}
	return result, truncated, nil
}

func ReadFullLog(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}
