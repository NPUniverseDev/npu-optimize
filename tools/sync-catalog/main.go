package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type Catalog struct {
	Version   string   `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Sources   []Source `json:"sources"`
}

type Source struct {
	Name     string                  `json:"name"`
	Repo     string                  `json:"repo"`
	Runtimes map[string]RuntimeEntry `json:"runtimes"`
}

type RuntimeEntry struct {
	ID             string   `json:"-"`
	Platform       string   `json:"platform"`
	Arch           string   `json:"arch"`
	Backend        string   `json:"backend"`
	BackendVersion string   `json:"backend_version,omitempty"`
	Version        string   `json:"version"`
	DownloadURL    string   `json:"download_url"`
	SHA256         string   `json:"sha256"`
	SizeBytes      int64    `json:"size_bytes"`
	Format         string   `json:"format"`
	RequiresLib    []string `json:"requires_lib,omitempty"`
	SourceName     string   `json:"source_name"`
}

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "warning: GITHUB_TOKEN not set, API rate limit is 60/hr")
	}

	data, err := os.ReadFile("docs/runtime-catalog.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading catalog: %v\n", err)
		os.Exit(1)
	}

	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		fmt.Fprintf(os.Stderr, "error: parsing catalog: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	changed := false

	for si := range cat.Sources {
		src := &cat.Sources[si]
		if src.Repo == "" {
			fmt.Fprintf(os.Stderr, "skipping source %q: no repo\n", src.Name)
			continue
		}

		fmt.Printf("checking %s...\n", src.Repo)

		release, err := fetchLatestRelease(client, src.Repo, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			continue
		}

		fmt.Printf("  latest tag: %s\n", release.TagName)

		for id, entry := range src.Runtimes {
			if entry.SourceName != src.Name {
				entry.SourceName = src.Name
				changed = true
			}
			updated := updateEntry(src.Repo, id, &entry, release)
			src.Runtimes[id] = entry
			if updated {
				changed = true
			}
		}
	}

	if !changed {
		fmt.Println("no updates needed")
		return
	}

	cat.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	out, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshaling catalog: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile("docs/runtime-catalog.json", out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing catalog: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("catalog updated successfully")
}

func fetchLatestRelease(client *http.Client, repo, token string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &release, nil
}

func updateEntry(repo, id string, entry *RuntimeEntry, release *Release) bool {
	if entry.DownloadURL == "" {
		return false
	}

	oldTag := extractTag(entry.DownloadURL)
	if oldTag == "" {
		return false
	}
	if oldTag == release.TagName {
		return false
	}

	oldAssetName := extractAssetName(entry.DownloadURL)
	newAssetName := strings.Replace(oldAssetName, oldTag, release.TagName, 1)

	for _, asset := range release.Assets {
		if asset.Name != newAssetName {
			continue
		}
		entry.DownloadURL = fmt.Sprintf(
			"https://github.com/%s/releases/download/%s/%s",
			repo, release.TagName, newAssetName,
		)
		entry.SizeBytes = asset.Size
		entry.Version = release.TagName
		fmt.Printf("  updated %s: %s -> %s\n", id, oldTag, release.TagName)
		return true
	}

	fmt.Printf("  warning: no asset matching %q found in %s release\n", newAssetName, release.TagName)
	return false
}

func extractTag(rawURL string) string {
	parts := strings.Split(rawURL, "/")
	for i, part := range parts {
		if part == "download" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractAssetName(rawURL string) string {
	parts := strings.Split(rawURL, "/")
	return parts[len(parts)-1]
}
