package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func FetchTemplatesJSON(url string) ([]byte, error) {
	const filePrefix = "file://"
	if path, found := strings.CutPrefix(url, filePrefix); found {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read templates: %w", err)
		}
		return data, nil
	}

	data, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}
	return data, nil
}

func httpGet(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		url,
		nil,
	)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}
