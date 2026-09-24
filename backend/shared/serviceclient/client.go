package serviceclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"movie-platform/shared/authmw"
)

// Client wraps http.Client and signs each outgoing request with a fresh,
// short-lived internal service token. Use this for all backend-to-backend
// calls (e.g. Processing -> Catalog) instead of a raw http.Client.
type Client struct {
	http        *http.Client
	secret      []byte
	serviceName string
}

func New(secret []byte, serviceName string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 10 * time.Second},
		secret:      secret,
		serviceName: serviceName,
	}
}

func (c *Client) do(method, url string, body any) (*http.Response, error) {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	token, err := authmw.GenerateServiceToken(c.secret, c.serviceName)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	return c.http.Do(req)
}

func (c *Client) Patch(url string, body any) (*http.Response, error) {
	return c.do(http.MethodPatch, url, body)
}

func (c *Client) Post(url string, body any) (*http.Response, error) {
	return c.do(http.MethodPost, url, body)
}

// CheckStatus closes the response body and returns an error for non-2xx
// status codes.
func CheckStatus(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}
