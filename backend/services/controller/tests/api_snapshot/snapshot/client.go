package snapshot

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// Client wraps http.Client with a default Authorization header so tests can
// call client.Get(path) without re-stating the JWT on every request.
type Client struct {
	httpClient *http.Client
	baseURL    string // empty means use absolute URLs as given
	authToken  string
}

// NewClient returns a Client authenticated with the given Bearer token.
// Pass an empty token for public endpoints (healthz, login).
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{},
		authToken:  token,
	}
}

// Do sends a request with the configured Authorization header.
// body may be nil, a string, or []byte. Returns raw response body bytes.
func (c *Client) Do(method, url string, body interface{}) (status int, respBody []byte, err error) {
	return c.DoWithContentType(method, url, body, "application/json")
}

// DoWithContentType is like Do but lets the caller override the Content-Type
// (e.g. for multipart/form-data uploads).
func (c *Client) DoWithContentType(method, url string, body interface{}, contentType string) (status int, respBody []byte, err error) {
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = strings.NewReader(v)
		case []byte:
			reader = bytes.NewReader(v)
		case io.Reader:
			reader = v
		default:
			reader = strings.NewReader("")
		}
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", c.authToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err = io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, err
}

// Get is shorthand for Do("GET", url, nil).
func (c *Client) Get(url string) (int, []byte, error) {
	return c.Do("GET", url, nil)
}

// PostJSON is shorthand for Do("POST", url, body).
func (c *Client) PostJSON(url, body string) (int, []byte, error) {
	return c.Do("POST", url, body)
}

// PutJSON is shorthand for Do("PUT", url, body).
func (c *Client) PutJSON(url, body string) (int, []byte, error) {
	return c.Do("PUT", url, body)
}

// Delete is shorthand for Do("DELETE", url, nil).
func (c *Client) Delete(url string) (int, []byte, error) {
	return c.Do("DELETE", url, nil)
}

// EqualStatus returns a helper that asserts the status code; usable with
// testify-style t.Helper patterns or directly.
func EqualStatus(t interface {
	Helper()
	Fatalf(string, ...interface{})
}, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("status: want %d, got %d", want, got)
	}
}
