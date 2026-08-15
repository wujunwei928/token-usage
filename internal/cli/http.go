package cli

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	pricingFetchTimeoutSeconds = 10
	pricingFetchMaxBytes       = 64 * 1024 * 1024
)

var pricingHTTPClient = &http.Client{
	Timeout: pricingFetchTimeoutSeconds * time.Second,
}

// FetchJSON fetches a JSON document for the pricing refresh, mirroring the
// reference http.rs: 10s global timeout, HTTP 200 required, 64MB body cap.
// It is installed into core via core.SetJSONFetcher from main so that
// net/http is not a dependency of the core package.
func FetchJSON(url string) (string, error) {
	resp, err := pricingHTTPClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, pricingFetchMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > pricingFetchMaxBytes {
		return "", fmt.Errorf("response exceeds %d bytes", pricingFetchMaxBytes)
	}
	return string(body), nil
}
