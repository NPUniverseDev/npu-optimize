package hfclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) SearchModels(search, numParams string, limit int) ([]ModelInfo, error) {
	u, _ := url.Parse(c.BaseURL + "/api/models")
	q := u.Query()

	q.Set("search", search)
	if numParams != "" {
		q.Set("num_parameters", numParams)
	}
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("full", "true")
	u.RawQuery = q.Encode()

	rawURL := u.String()
	cacheKey := c.cacheKey("search", rawURL)
	if cached, ok := c.getFromCache(cacheKey); ok {
		var resp SearchResponse
		if err := json.Unmarshal(cached, &resp); err == nil {
			return resp, nil
		}
	}

	data, err := c.doRequest(rawURL)
	if err != nil {
		return nil, fmt.Errorf("search models: %w", err)
	}

	c.storeInCache(cacheKey, data, searchCacheTTL)

	var resp SearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}

	return resp, nil
}

func (c *Client) GetTree(repo string) ([]TreeEntry, error) {
	rawURL := fmt.Sprintf("%s/api/models/%s/tree/main", c.BaseURL, repo)

	cacheKey := c.cacheKey("tree", rawURL)
	if cached, ok := c.getFromCache(cacheKey); ok {
		var entries TreeResponse
		if err := json.Unmarshal(cached, &entries); err == nil {
			return entries, nil
		}
	}

	data, err := c.doRequest(rawURL)
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	c.storeInCache(cacheKey, data, treeCacheTTL)

	var entries TreeResponse
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing tree response: %w", err)
	}

	return entries, nil
}

func (c *Client) GetPathsInfo(repo string, paths []string) ([]PathsInfoEntry, error) {
	rawURL := fmt.Sprintf("%s/api/models/%s/paths-info/main", c.BaseURL, repo)

	cacheKey := c.cacheKey("paths-info", rawURL+"|"+strings.Join(paths, ","))
	if cached, ok := c.getFromCache(cacheKey); ok {
		var entries []PathsInfoEntry
		if err := json.Unmarshal(cached, &entries); err == nil {
			return entries, nil
		}
	}

	body, err := json.Marshal(PathsInfoRequest{Paths: paths})
	if err != nil {
		return nil, fmt.Errorf("marshalling paths-info request: %w", err)
	}

	data, err := c.doWithRetry(func() (*http.Response, error) {
		req, err := http.NewRequest("POST", rawURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		req.Header.Set("Content-Type", "application/json")
		return c.HTTPClient.Do(req)
	})
	if err != nil {
		return nil, fmt.Errorf("get paths-info: %w", err)
	}

	c.storeInCache(cacheKey, data, treeCacheTTL)

	var entries []PathsInfoEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing paths-info response: %w", err)
	}

	return entries, nil
}

func (c *Client) GetGGUFHeader(repo, file string, maxSize int) ([]byte, error) {
	rawURL := fmt.Sprintf("%s/%s/resolve/main/%s", c.BaseURL, repo, file)

	cacheKey := c.cacheKey("gguf", fmt.Sprintf("%s|%d", rawURL, maxSize))
	if cached, ok := c.getFromCache(cacheKey); ok {
		return cached, nil
	}

	data, err := c.doWithRetry(func() (*http.Response, error) {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxSize-1))
		return c.HTTPClient.Do(req)
	})
	if err != nil {
		return nil, fmt.Errorf("get gguf header: %w", err)
	}

	c.storeInCache(cacheKey, data, ggufCacheTTL)
	return data, nil
}
