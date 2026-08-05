// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package client

import "testing"

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("https://platform.betteredge.cloud:4000/", "token")

	if got, want := c.BaseURL, "https://platform.betteredge.cloud:4000"; got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
	if c.Token != "token" {
		t.Errorf("Token = %q, want %q", c.Token, "token")
	}
}

func TestAPIErrorMessage(t *testing.T) {
	err := &APIError{Method: "GET", Endpoint: "/api/project/123", StatusCode: 404, Body: "not found"}

	got := err.Error()
	want := `GET /api/project/123: status 404: not found`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestJournalAccessors(t *testing.T) {
	j := Journal{"jobStatus": "COMPLETED", "resourceId": "abc-123"}

	if got := j.Status(); got != "COMPLETED" {
		t.Errorf("Status() = %q, want %q", got, "COMPLETED")
	}
	if got := j.ResourceID(); got != "abc-123" {
		t.Errorf("ResourceID() = %q, want %q", got, "abc-123")
	}

	empty := Journal{}
	if got := empty.Status(); got != "" {
		t.Errorf("Status() on empty journal = %q, want empty string", got)
	}
	if got := empty.ResourceID(); got != "" {
		t.Errorf("ResourceID() on empty journal = %q, want empty string", got)
	}
}

func TestJobFailedErrorMessage(t *testing.T) {
	err := &JobFailedError{JobID: "job-1", Journal: Journal{"jobStatus": "FAILED"}}
	if got := err.Error(); got == "" {
		t.Error("Error() returned empty string")
	}
}

func TestJobTimeoutErrorMessage(t *testing.T) {
	err := &JobTimeoutError{JobID: "job-1", LastStatus: "RUNNING"}
	want := "job job-1 timed out waiting for completion (last status: RUNNING)"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
