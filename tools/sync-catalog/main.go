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
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
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
	var llamaRelease *Release

	for si := range cat.Sources {
		src := &cat.Sources[si]
		if src.Repo == "" {
			fmt.Fprintf(os.Stderr, "skipping source %q: no repo\n", src.Name)
			continue
		}

		// Migrate legacy Ericson246 -> NPUniverseDev alias (fork renamed).
		if src.Repo == "Ericson246/llama.cpp" {
			fmt.Printf("migrating source %q: %s -> NPUniverseDev/llama.cpp\n", src.Name, src.Repo)
			src.Repo = "NPUniverseDev/llama.cpp"
			src.Name = "NPUniverseDev/llama.cpp"
			changed = true
		}

		fmt.Printf("checking %s...\n", src.Repo)

		release, err := fetchLatestRelease(client, src.Repo, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			continue
		}

		if src.Repo == "ggml-org/llama.cpp" {
			llamaRelease = release
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
	} else {
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

	if llamaRelease != nil {
		emitBenchURL(llamaRelease)
	}
}

func emitBenchURL(release *Release) {
	// Prefer ubuntu-x64 CPU asset (smallest, deterministic) over vulkan/rocm variants.
	candidates := []Asset{}
	for _, a := range release.Assets {
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		if !strings.Contains(name, "linux") && !strings.Contains(name, "ubuntu") {
			continue
		}
		if !strings.Contains(name, "x64") && !strings.Contains(name, "amd64") {
			continue
		}
		if !strings.Contains(name, "llama") && !strings.Contains(name, "bin") {
			continue
		}
		candidates = append(candidates, a)
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "warning: no llama-bench linux/amd64 asset found in latest release")
		return
	}
	// Rank: prefer plain ubuntu-x64 (CPU) without gpu backend markers, then any ubuntu-x64.
	bestIdx := 0
	bestScore := -1000
	for i, a := range candidates {
		name := strings.ToLower(a.Name)
		score := 0
		if strings.Contains(name, "ubuntu-x64.tar.gz") && !strings.Contains(name, "vulkan") && !strings.Contains(name, "cuda") && !strings.Contains(name, "rocm") && !strings.Contains(name, "hip") && !strings.Contains(name, "openvino") && !strings.Contains(name, "sycl") && !strings.Contains(name, "snapdragon") {
			score = 100
		} else if strings.Contains(name, "ubuntu-x64") {
			score = 50
		} else if strings.Contains(name, "linux") {
			score = 10
		}
		if strings.Contains(name, "snapdragon") {
			score -= 20
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	fmt.Printf("LLAMA_BENCH_URL=%s\n", candidates[bestIdx].BrowserDownloadURL)
}

func fetchLatestRelease(client *http.Client, repo, token string) (*Release, error) {
	// Try /releases/latest first; for ggml-org/llama.cpp the latest is now v0.5.0
	// which has no bin assets (only nightly-tag.txt). Detect and fallback to latest
	// bXXXX/daily tag with actual binaries.
	release, err := fetchReleaseByURL(client, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), token)
	if err != nil {
		return nil, err
	}
	if hasBinAssets(release) {
		return release, nil
	}
	// Fallback: scan recent releases for the newest with bin assets.
	// For ggml-org we want bXXXX (rolling), for NPUniverseDev daily-YYYY-MM-DD.
	fmt.Fprintf(os.Stderr, "  [%s] latest %s has no bin assets (%d assets), scanning recent releases...\n", repo, release.TagName, len(release.Assets))
	releases, err := fetchReleasesList(client, repo, token, 30)
	if err != nil {
		return nil, err
	}
	for _, r := range releases {
		if hasBinAssets(&r) {
			fmt.Fprintf(os.Stderr, "  [%s] using fallback release %s (%d assets)\n", repo, r.TagName, len(r.Assets))
			return &r, nil
		}
	}
	// No bin assets found at all — return the original (caller will warn)
	return release, nil
}

func fetchReleaseByURL(client *http.Client, url, token string) (*Release, error) {
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

func fetchReleasesList(client *http.Client, repo, token string, perPage int) ([]Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d", repo, perPage)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func hasBinAssets(r *Release) bool {
	if len(r.Assets) < 3 {
		return false
	}
	count := 0
	for _, a := range r.Assets {
		lower := strings.ToLower(a.Name)
		if (strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")) && strings.Contains(lower, "llama") {
			count++
			if count >= 3 {
				return true
			}
		}
	}
	return false
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

	// Fast-path: exact name replacement (backwards compatible, deterministic).
	oldAssetName := extractAssetName(entry.DownloadURL)
	newAssetName := strings.Replace(oldAssetName, oldTag, release.TagName, 1)
	for _, asset := range release.Assets {
		if asset.Name == newAssetName {
			entry.DownloadURL = fmt.Sprintf(
				"https://github.com/%s/releases/download/%s/%s",
				repo, release.TagName, newAssetName,
			)
			entry.SizeBytes = asset.Size
			entry.Version = release.TagName
			if bv := extractBackendVersion(asset.Name, entry.Backend); bv != "" && entry.BackendVersion != bv {
				entry.BackendVersion = bv
			}
			fmt.Printf("  updated %s: %s -> %s (exact)\n", id, oldTag, release.TagName)
			return true
		}
	}

	// Stable fallback: structured matching by platform/arch/backend (handles renamed assets
	// like cuda 13.3->13.4, rocm hip->rocm 10.0, openvino 2026.2->2026.4, sycl split).
	best := findBestAsset(entry, release)
	if best == nil {
		fmt.Printf("  warning: no asset matching %q found in %s release (fuzzy: no candidate for %s/%s/%s)\n", newAssetName, release.TagName, entry.Platform, entry.Arch, entry.Backend)
		return false
	}
	entry.DownloadURL = fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/%s",
		repo, release.TagName, best.Name,
	)
	entry.SizeBytes = best.Size
	entry.Version = release.TagName
	if bv := extractBackendVersion(best.Name, entry.Backend); bv != "" && entry.BackendVersion != bv {
		// Only update backend_version if entry had one (cuda) or backend is cuda.
		if entry.BackendVersion != "" || entry.Backend == "cuda" {
			entry.BackendVersion = bv
		}
	}
	fmt.Printf("  updated %s: %s -> %s (fuzzy: %s)\n", id, oldTag, release.TagName, best.Name)
	return true
}

func findBestAsset(entry *RuntimeEntry, release *Release) *Asset {
	candidates := []*Asset{}
	for i := range release.Assets {
		a := &release.Assets[i]
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tgz") {
			continue
		}
		if !platformMatches(entry.Platform, name) {
			continue
		}
		if !archMatches(entry.Arch, name) {
			continue
		}
		if !backendMatches(entry.Backend, name) {
			continue
		}
		candidates = append(candidates, a)
	}
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	// Score among candidates: prefer backend_version major match, prefer non-snapdragon, prefer fp32 for sycl.
	best := candidates[0]
	bestScore := scoreAsset(entry, candidates[0])
	for _, c := range candidates[1:] {
		s := scoreAsset(entry, c)
		if s > bestScore || (s == bestScore && strings.ToLower(c.Name) < strings.ToLower(best.Name)) {
			best = c
			bestScore = s
		}
	}
	return best
}

func scoreAsset(entry *RuntimeEntry, a *Asset) int {
	name := strings.ToLower(a.Name)
	score := 0
	if entry.BackendVersion != "" {
		if strings.Contains(name, strings.ToLower(entry.BackendVersion)) {
			score += 20
		} else if entry.Backend == "cuda" {
			major := strings.Split(entry.BackendVersion, ".")[0]
			if strings.Contains(name, "cuda-"+major) {
				score += 10
			}
		} else {
			major := strings.Split(entry.BackendVersion, ".")[0]
			if major != "" && strings.Contains(name, major) {
				score += 5
			}
		}
	}
	// Prefer fp32 over fp16 for sycl split assets.
	if entry.Backend == "sycl" {
		if strings.Contains(name, "fp32") {
			score += 3
		} else if strings.Contains(name, "fp16") {
			score += 1
		}
	}
	// Prefer llama- prefix over cudart- for consistency with catalog history.
	if strings.HasPrefix(name, "llama-") {
		score += 5
	} else if strings.HasPrefix(name, "cudart-") {
		score -= 5
	}
	if strings.Contains(name, "snapdragon") {
		score -= 20
	}
	// For openvino/rocm where version changed, the single candidate will win anyway.
	return score
}

func platformMatches(platform, name string) bool {
	switch platform {
	case "windows":
		return strings.Contains(name, "win")
	case "linux":
		return strings.Contains(name, "ubuntu") || strings.Contains(name, "linux")
	case "darwin":
		return strings.Contains(name, "macos") || strings.Contains(name, "darwin")
	case "android":
		return strings.Contains(name, "android")
	default:
		return strings.Contains(name, platform)
	}
}

func archMatches(arch, name string) bool {
	switch arch {
	case "x64":
		return strings.Contains(name, "x64") || strings.Contains(name, "amd64")
	case "arm64":
		return strings.Contains(name, "arm64") || strings.Contains(name, "aarch64")
	default:
		return strings.Contains(name, arch)
	}
}

func backendMatches(backend, name string) bool {
	switch strings.ToLower(backend) {
	case "cuda":
		return strings.Contains(name, "cuda")
	case "rocm":
		return strings.Contains(name, "rocm") || strings.Contains(name, "hip")
	case "openvino":
		return strings.Contains(name, "openvino")
	case "vulkan":
		return strings.Contains(name, "vulkan")
	case "sycl":
		return strings.Contains(name, "sycl")
	case "cpu":
		if strings.Contains(name, "cpu") {
			return true
		}
		// CPU assets are plain os-arch without gpu markers.
		if strings.Contains(name, "cuda") || strings.Contains(name, "rocm") || strings.Contains(name, "hip") || strings.Contains(name, "vulkan") || strings.Contains(name, "openvino") || strings.Contains(name, "sycl") || strings.Contains(name, "metal") {
			return false
		}
		return true
	case "metal":
		if strings.Contains(name, "metal") {
			return true
		}
		// darwin metal builds are plain macos archives
		if strings.Contains(name, "macos") && !strings.Contains(name, "cuda") && !strings.Contains(name, "vulkan") && !strings.Contains(name, "rocm") && !strings.Contains(name, "hip") {
			return true
		}
		return false
	default:
		return strings.Contains(name, strings.ToLower(backend))
	}
}

func extractBackendVersion(assetName, backend string) string {
	lower := strings.ToLower(assetName)
	if strings.ToLower(backend) == "cuda" {
		// Find cuda-X.Y
		idx := strings.Index(lower, "cuda-")
		if idx >= 0 {
			rest := lower[idx+5:]
			end := 0
			for end < len(rest) && (rest[end] >= '0' && rest[end] <= '9' || rest[end] == '.') {
				end++
			}
			if end > 0 {
				v := rest[:end]
				// Trim trailing dot
				v = strings.TrimSuffix(v, ".")
				if strings.Count(v, ".") >= 1 {
					return v
				}
			}
		}
	}
	return ""
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
