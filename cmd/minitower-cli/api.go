package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("request failed: status=%d message=%s", e.Status, e.Message)
	}
	return fmt.Sprintf("request failed: status=%d code=%s message=%s", e.Status, e.Code, e.Message)
}

func newAPIClient(serverURL, token string) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(serverURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *apiClient) endpoint(apiPath string) string {
	if strings.HasPrefix(apiPath, "http://") || strings.HasPrefix(apiPath, "https://") {
		return apiPath
	}
	return c.baseURL + apiPath
}

func (c *apiClient) newRequest(ctx context.Context, method, apiPath string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(apiPath), body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

func (c *apiClient) decodeResponse(resp *http.Response, out any) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		return &apiError{Status: resp.StatusCode, Code: env.Error.Code, Message: env.Error.Message}
	}

	return &apiError{Status: resp.StatusCode, Message: msg}
}

func (c *apiClient) doJSON(ctx context.Context, method, apiPath string, reqBody, out any) error {
	var body io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := c.newRequest(ctx, method, apiPath, body)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.decodeResponse(resp, out)
}

func (c *apiClient) doMultipartFile(ctx context.Context, apiPath, fieldName, fileName string, data []byte, out any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(fieldName, fileName)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := c.newRequest(ctx, http.MethodPost, apiPath, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.decodeResponse(resp, out)
}

func withQuery(apiPath string, query map[string]string) (string, error) {
	u, err := url.Parse(apiPath)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range query {
		if strings.TrimSpace(v) == "" {
			continue
		}
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
