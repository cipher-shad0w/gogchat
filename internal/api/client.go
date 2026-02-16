package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// BaseURL is the default Google Chat API endpoint.
const BaseURL = "https://chat.googleapis.com/v1"

// Client is the base HTTP client for the Google Chat API.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Verbose    bool
}

// NewClient creates a new API client with the default BaseURL.
func NewClient(httpClient *http.Client) *Client {
	return &Client{
		HTTPClient: httpClient,
		BaseURL:    BaseURL,
	}
}

// ErrorLink represents a help link in a Google API error detail.
type ErrorLink struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

// ErrorDetail represents a single entry in the Google API error "details" array.
type ErrorDetail struct {
	Type     string            `json:"@type"`
	Reason   string            `json:"reason,omitempty"`
	Domain   string            `json:"domain,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Links    []ErrorLink       `json:"links,omitempty"`
}

// APIError represents a non-2xx response from the Google Chat API.
// Google API error format: {"error": {"code": 403, "message": "...", "status": "PERMISSION_DENIED", "details": [...]}}
type APIError struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Status  string        `json:"status"`
	Details []ErrorDetail `json:"details,omitempty"`
	RawBody string        `json:"-"` // raw response body for verbose/debug output
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d (%s): %s", e.Code, e.Status, e.Message)
}

// HelpLinks returns all help URLs from the error details.
func (e *APIError) HelpLinks() []ErrorLink {
	var links []ErrorLink
	for _, d := range e.Details {
		links = append(links, d.Links...)
	}
	return links
}

// ErrorReason returns the reason from the first ErrorInfo detail, or empty.
func (e *APIError) ErrorReason() string {
	for _, d := range e.Details {
		if d.Reason != "" {
			return d.Reason
		}
	}
	return ""
}

// Get performs an HTTP GET request and returns the raw JSON response body.
func (c *Client) Get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, path, params, nil, "")
}

// Post performs an HTTP POST request with a JSON body and returns the raw JSON response.
func (c *Client) Post(ctx context.Context, path string, params url.Values, body interface{}) (json.RawMessage, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, params, bytes.NewReader(jsonBody), "application/json")
}

// Patch performs an HTTP PATCH request with a JSON body and returns the raw JSON response.
func (c *Client) Patch(ctx context.Context, path string, params url.Values, body interface{}) (json.RawMessage, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}
	return c.do(ctx, http.MethodPatch, path, params, bytes.NewReader(jsonBody), "application/json")
}

// Put performs an HTTP PUT request with a JSON body and returns the raw JSON response.
func (c *Client) Put(ctx context.Context, path string, params url.Values, body interface{}) (json.RawMessage, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}
	return c.do(ctx, http.MethodPut, path, params, bytes.NewReader(jsonBody), "application/json")
}

// Delete performs an HTTP DELETE request and returns the raw JSON response body.
func (c *Client) Delete(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	return c.do(ctx, http.MethodDelete, path, params, nil, "")
}

// Upload performs an HTTP POST request with arbitrary content (e.g. multipart upload)
// and returns the raw JSON response.
func (c *Client) Upload(ctx context.Context, path string, params url.Values, body io.Reader, contentType string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, path, params, body, contentType)
}

// Download performs an HTTP GET and returns the response body as a ReadCloser,
// the Content-Type header, and any error.
func (c *Client) Download(ctx context.Context, path string) (io.ReadCloser, string, error) {
	reqURL := c.buildURL(path, nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating request: %w", err)
	}

	if c.Verbose {
		log.Printf(">> %s %s\n", req.Method, req.URL.String())
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("executing request: %w", err)
	}

	if c.Verbose {
		log.Printf("<< %d %s\n", resp.StatusCode, resp.Status)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		apiErr := parseAPIError(resp)
		if apiErr != nil {
			return nil, "", apiErr
		}
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	return resp.Body, contentType, nil
}

// do is the internal helper that executes an HTTP request, checks the status code,
// and returns the response body as raw JSON or an error.
func (c *Client) do(ctx context.Context, method, path string, params url.Values, body io.Reader, contentType string) (json.RawMessage, error) {
	reqURL := c.buildURL(path, params)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if c.Verbose {
		log.Printf(">> %s %s\n", req.Method, req.URL.String())
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if c.Verbose {
		log.Printf("<< %d %s\n", resp.StatusCode, resp.Status)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if c.Verbose {
			log.Printf("<< Response body:\n%s\n", string(respBody))
		}
		apiErr := parseAPIErrorFromBody(resp.StatusCode, respBody)
		if apiErr != nil {
			return nil, apiErr
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return json.RawMessage(respBody), nil
}

// buildURL constructs the full request URL from the base URL, path, and query parameters.
func (c *Client) buildURL(path string, params url.Values) string {
	u := c.BaseURL + "/" + strings.TrimLeft(path, "/")
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

// parseAPIError reads the response body and attempts to parse a Google API error.
func parseAPIError(resp *http.Response) *APIError {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	return parseAPIErrorFromBody(resp.StatusCode, body)
}

// parseAPIErrorFromBody attempts to parse a Google API error from raw bytes.
func parseAPIErrorFromBody(statusCode int, body []byte) *APIError {
	var envelope struct {
		Error struct {
			Code    int           `json:"code"`
			Message string        `json:"message"`
			Status  string        `json:"status"`
			Details []ErrorDetail `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Code != 0 {
		return &APIError{
			Code:    envelope.Error.Code,
			Message: envelope.Error.Message,
			Status:  envelope.Error.Status,
			Details: envelope.Error.Details,
			RawBody: string(body),
		}
	}
	// If we couldn't parse the Google error format, build a generic one.
	if statusCode >= 400 {
		return &APIError{
			Code:    statusCode,
			Message: strings.TrimSpace(string(body)),
			Status:  http.StatusText(statusCode),
			RawBody: string(body),
		}
	}
	return nil
}

// NormalizeName ensures name starts with the given prefix.
// E.g. NormalizeName("AAAA", "spaces/") → "spaces/AAAA"
// E.g. NormalizeName("spaces/AAAA", "spaces/") → "spaces/AAAA"
func NormalizeName(name, prefix string) string {
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

// AddQueryParam adds a query parameter only if the value is non-empty.
func AddQueryParam(params url.Values, key, value string) {
	if value != "" {
		params.Set(key, value)
	}
}

// AddQueryParamBool adds a boolean query parameter only if the value is true.
func AddQueryParamBool(params url.Values, key string, value bool) {
	if value {
		params.Set(key, strconv.FormatBool(value))
	}
}

// AddQueryParamInt adds an integer query parameter only if the value is greater than 0.
func AddQueryParamInt(params url.Values, key string, value int) {
	if value > 0 {
		params.Set(key, strconv.Itoa(value))
	}
}
