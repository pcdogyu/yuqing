package external

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestClassifyAndRetryPolicy(t *testing.T) {
	if got := Classify(context.DeadlineExceeded); got != ErrTimeout {
		t.Fatalf("expected timeout classification, got %q", got)
	}
	if !ShouldRetryResponse(nil, context.DeadlineExceeded) {
		t.Fatal("expected timeout errors to be retryable")
	}
	if !ShouldRetryResponse(&resty.Response{RawResponse: &http.Response{StatusCode: http.StatusTooManyRequests}}, nil) {
		t.Fatal("expected 429 to be retryable")
	}
	if !ShouldRetryResponse(&resty.Response{RawResponse: &http.Response{StatusCode: http.StatusBadGateway}}, nil) {
		t.Fatal("expected 5xx to be retryable")
	}
	if ShouldRetryResponse(&resty.Response{RawResponse: &http.Response{StatusCode: http.StatusBadRequest}}, nil) {
		t.Fatal("expected 400 to be non-retryable")
	}
}
