package engine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// TODO: add more encodings
func GetEncodingFromName(encodingName string) (encoding.Encoding, error) {
	switch strings.ToLower(strings.ReplaceAll(encodingName, "-", "")) {
	case "shiftjis", "shift_jis", "sjis":
		return japanese.ShiftJIS, nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", encodingName)
	}
}

func ConvertToUTF8(content []byte, encodingName string) ([]byte, error) {
	if encodingName == "" || strings.ToUpper(encodingName) == "UTF-8" || strings.ToUpper(encodingName) == "UTF8" {
		return content, nil
	}
	enc, err := GetEncodingFromName(encodingName)
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding for %s: %w", encodingName, err)
	}
	decoder := enc.NewDecoder()
	utf8Content, err := io.ReadAll(transform.NewReader(bytes.NewReader(content), decoder))
	if err != nil {
		return nil, fmt.Errorf("failed to convert from %s to UTF-8: %w", encodingName, err)
	}

	return utf8Content, nil
}

func OpenFileWithEncoding(filePath string, encodingName string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if encodingName != "" {
		utf8Content, err := ConvertToUTF8(content, encodingName)
		if err != nil {
			return content, err
		}
		return utf8Content, nil
	}

	return content, nil
}
