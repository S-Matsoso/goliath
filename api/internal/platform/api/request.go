package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func DecodeJSON(c *gin.Context, destination any, maxBytes int64) error {
	request := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(request)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains more than one JSON value")
		}
		return err
	}
	return nil
}

func PageSize(c *gin.Context) (int, error) {
	raw := c.Query("limit")
	if raw == "" {
		return 100, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 500 {
		return 0, fmt.Errorf("limit must be between 1 and 500")
	}
	return limit, nil
}

func Cursor(c *gin.Context) (uint64, error) {
	raw := c.Query("cursor")
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cursor must be a positive integer")
	}
	return cursor, nil
}

func BinaryCursor(c *gin.Context, expectedBytes int) ([]byte, error) {
	raw := c.Query("cursor")
	if raw == "" {
		return nil, nil
	}
	cursor, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(cursor) != expectedBytes {
		return nil, fmt.Errorf("cursor is invalid")
	}
	return cursor, nil
}

func EncodeBinaryCursor(cursor []byte) string {
	return base64.RawURLEncoding.EncodeToString(cursor)
}

func ValidPurpose(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 120
}
