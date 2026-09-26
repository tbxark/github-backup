package request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func DefaultHttpClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

func URL(base string, query map[string]string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	for k, v := range query {
		params.Add(k, v)
	}
	baseURL.RawQuery = params.Encode()
	return baseURL.String(), nil
}

type Modifier func(client *http.Client, req *http.Request)

type StatusError struct {
	Code    int
	Status  string
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %s: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %s", e.Status)
}

func Request(ctx context.Context, method, url string, modifier ...Modifier) (*http.Response, error) {
	client := DefaultHttpClient()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	for _, m := range modifier {
		m(client, req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &StatusError{Code: resp.StatusCode, Status: resp.Status, Message: strings.TrimSpace(string(message))}
}

func GET[T any](ctx context.Context, url string, modifier ...Modifier) (*T, error) {
	client := DefaultHttpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for _, m := range modifier {
		m(client, req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var result T
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func POST[T any](ctx context.Context, url string, data any, modifier ...Modifier) (*T, error) {
	client := DefaultHttpClient()
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	for _, m := range modifier {
		m(client, req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var result T
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func WithAuthorization(token string, prefix string) Modifier {
	return func(client *http.Client, req *http.Request) {
		req.Header.Add("Authorization", prefix+" "+token)
	}
}
