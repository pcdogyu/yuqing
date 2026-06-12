package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

const (
	ErrTimeout       = "external_timeout"
	ErrNon200        = "external_non_200"
	ErrInvalidJSON   = "external_invalid_json"
	ErrEmptyData     = "external_empty_data"
	ErrDuplicateData = "external_duplicate_data"
	ErrDisabled      = "external_disabled"
)

type ClassifiedError struct {
	Code    string
	Message string
}

func (e ClassifiedError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func New(code, message string) error {
	return ClassifiedError{Code: code, Message: strings.TrimSpace(message)}
}

func Classify(err error) string {
	if err == nil {
		return ""
	}
	var classified ClassifiedError
	if errors.As(err, &classified) {
		return classified.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrTimeout
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return ErrInvalidJSON
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return ErrInvalidJSON
	}
	return ""
}

func HTTPStatus(status string, code int) error {
	if code == 0 {
		return New(ErrNon200, status)
	}
	return New(ErrNon200, fmt.Sprintf("%s (%d)", status, code))
}

func IsSuccess(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func ShouldRetryResponse(resp *resty.Response, err error) bool {
	if err != nil {
		return Classify(err) == ErrTimeout
	}
	if resp == nil {
		return false
	}
	status := resp.StatusCode()
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}
