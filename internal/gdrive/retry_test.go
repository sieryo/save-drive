package gdrive

import (
	"context"
	"net/http"
	"testing"
)

func TestRetryTransientErrors(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		fd, c := newFakeDrive(t)
		id := fd.add("root", "app.zip", "", nil)
		fd.failStatus, fd.failCount = status, 2

		if _, err := c.Get(context.Background(), id); err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		if fd.requests != 3 {
			t.Fatalf("status %d: requests = %d, want 3", status, fd.requests)
		}
	}
}

func TestRetryGivesUp(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "app.zip", "", nil)
	fd.failStatus, fd.failCount = http.StatusServiceUnavailable, 100

	if _, err := c.Get(context.Background(), id); err == nil {
		t.Fatal("expected error")
	}
	if fd.requests != retryAttempts {
		t.Fatalf("requests = %d, want %d", fd.requests, retryAttempts)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.failStatus, fd.failCount = http.StatusBadRequest, 1

	if _, err := c.Get(context.Background(), "missing"); err == nil {
		t.Fatal("expected error")
	}
	if fd.requests != 1 {
		t.Fatalf("requests = %d, want 1", fd.requests)
	}
}
