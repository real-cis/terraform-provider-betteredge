// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

type APIError struct {
	Method     string
	Endpoint   string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Endpoint, e.StatusCode, e.Body)
}

func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	tflog.Debug(ctx, "BetterEdge API request", map[string]any{"method": method, "path": path})

	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body for %s %s: %w", method, path, err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("building request for %s %s: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Token", c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		tflog.Debug(ctx, "BetterEdge API request failed", map[string]any{"method": method, "path": path, "error": err.Error()})
		return fmt.Errorf("calling %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response from %s %s: %w", method, path, err)
	}

	tflog.Debug(ctx, "BetterEdge API response", map[string]any{"method": method, "path": path, "status_code": resp.StatusCode})

	if resp.StatusCode >= 400 {
		return &APIError{Method: method, Endpoint: path, StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response from %s %s: %w", method, path, err)
		}
	}

	return nil
}

func (c *Client) Exists(ctx context.Context, lookupPath string) (bool, error) {
	err := c.Do(ctx, http.MethodGet, lookupPath, nil, nil)
	if err == nil {
		return true, nil
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, err
}

type Journal map[string]any

func (j Journal) Status() string {
	status, _ := j["jobStatus"].(string)
	return status
}

func (j Journal) ResourceID() string {
	id, _ := j["resourceId"].(string)
	return id
}

type JobFailedError struct {
	JobID   string
	Journal Journal
}

func (e *JobFailedError) Error() string {
	return fmt.Sprintf("job %s failed: %v", e.JobID, e.Journal)
}

type JobTimeoutError struct {
	JobID      string
	LastStatus string
}

func (e *JobTimeoutError) Error() string {
	return fmt.Sprintf("job %s timed out waiting for completion (last status: %s)", e.JobID, e.LastStatus)
}

const (
	defaultPollInterval = 10 * time.Second
	defaultPollAttempts = 60
)

type pollConfig struct {
	successCheck func(ctx context.Context) (bool, error)
}

type PollOption func(*pollConfig)

func WithSuccessCheck(check func(ctx context.Context) (bool, error)) PollOption {
	return func(c *pollConfig) { c.successCheck = check }
}

func (c *Client) PollJob(ctx context.Context, jobID string, opts ...PollOption) (Journal, error) {
	cfg := &pollConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var journal Journal

	for attempt := 0; attempt < defaultPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}

		journal = Journal{}
		if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/journal/job/%s", jobID), nil, &journal); err != nil {
			return nil, fmt.Errorf("polling job %s: %w", jobID, err)
		}

		switch journal.Status() {
		case "COMPLETED":
			return journal, nil
		case "FAILED":
			return journal, &JobFailedError{JobID: jobID, Journal: journal}
		}

		if cfg.successCheck != nil {
			ok, err := cfg.successCheck(ctx)
			if err != nil {
				return nil, err
			}
			if ok {
				return journal, nil
			}
		}
	}

	return nil, &JobTimeoutError{JobID: jobID, LastStatus: journal.Status()}
}
