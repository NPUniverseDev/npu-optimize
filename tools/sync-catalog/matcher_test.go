package main

import "testing"

func makeRelease(tag string, names []string) *Release {
	r := &Release{TagName: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: n, Size: 123, BrowserDownloadURL: "https://example.com/" + n})
	}
	return r
}

func TestPlatformMatches(t *testing.T) {
	tests := []struct {
		platform string
		name     string
		want     bool
	}{
		{"windows", "llama-b11177-bin-win-cuda-13.4-x64.zip", true},
		{"linux", "llama-b11177-bin-ubuntu-x64.tar.gz", true},
		{"linux", "llama-b11177-bin-linux-arm64-snapdragon.tar.gz", true},
		{"darwin", "llama-b11177-bin-macos-arm64.tar.gz", true},
		{"android", "llama-daily-2026-09-25-bin-android-arm64.tar.gz", true},
		{"windows", "llama-b11177-bin-ubuntu-x64.tar.gz", false},
	}
	for _, tc := range tests {
		if got := platformMatches(tc.platform, tc.name); got != tc.want {
			t.Errorf("platformMatches(%q,%q)=%v want %v", tc.platform, tc.name, got, tc.want)
		}
	}
}

func TestBackendMatches(t *testing.T) {
	tests := []struct {
		backend string
		name    string
		want    bool
	}{
		{"cuda", "llama-b11177-bin-win-cuda-13.4-x64.zip", true},
		{"rocm", "llama-b11177-bin-win-rocm-10.0-x64.zip", true},
		{"rocm", "llama-b10235-bin-win-hip-radeon-x64.zip", true}, // legacy hip
		{"openvino", "llama-b11177-bin-ubuntu-openvino-2026.4-x64.tar.gz", true},
		{"vulkan", "llama-b11177-bin-ubuntu-vulkan-x64.tar.gz", true},
		{"sycl", "llama-b11177-bin-ubuntu-sycl-fp32-x64.tar.gz", true},
		{"cpu", "llama-b11177-bin-ubuntu-x64.tar.gz", true},
		{"cpu", "llama-b11177-bin-ubuntu-vulkan-x64.tar.gz", false},
		{"metal", "llama-b11177-bin-macos-arm64.tar.gz", true},
	}
	for _, tc := range tests {
		if got := backendMatches(tc.backend, tc.name); got != tc.want {
			t.Errorf("backendMatches(%q,%q)=%v want %v", tc.backend, tc.name, got, tc.want)
		}
	}
}

func TestExtractBackendVersion(t *testing.T) {
	if got := extractBackendVersion("llama-b11177-bin-win-cuda-13.4-x64.zip", "cuda"); got != "13.4" {
		t.Errorf("extractBackendVersion cuda got %q want 13.4", got)
	}
	if got := extractBackendVersion("llama-b11177-bin-win-cuda-12.4-x64.zip", "cuda"); got != "12.4" {
		t.Errorf("extractBackendVersion cuda 12.4 got %q", got)
	}
	if got := extractBackendVersion("llama-b11177-bin-ubuntu-x64.tar.gz", "cpu"); got != "" {
		t.Errorf("extractBackendVersion cpu got %q want empty", got)
	}
}

func TestFindBestAsset_CUDA13_3To13_4(t *testing.T) {
	entry := RuntimeEntry{Platform: "windows", Arch: "x64", Backend: "cuda", BackendVersion: "13.3"}
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-win-cuda-12.4-x64.zip",
		"llama-b11177-bin-win-cuda-13.4-x64.zip",
		"llama-b11177-bin-ubuntu-x64.tar.gz",
	})
	best := findBestAsset(&entry, rel)
	if best == nil || best.Name != "llama-b11177-bin-win-cuda-13.4-x64.zip" {
		t.Fatalf("expected cuda 13.4, got %v", best)
	}
}

func TestFindBestAsset_ROCmHipMigration(t *testing.T) {
	entry := RuntimeEntry{Platform: "windows", Arch: "x64", Backend: "rocm"}
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-win-rocm-10.0-x64.zip",
		"llama-b11177-bin-win-cuda-13.4-x64.zip",
	})
	best := findBestAsset(&entry, rel)
	if best == nil || best.Name != "llama-b11177-bin-win-rocm-10.0-x64.zip" {
		t.Fatalf("expected rocm 10.0, got %v", best)
	}
}

func TestFindBestAsset_OpenVINO(t *testing.T) {
	entry := RuntimeEntry{Platform: "linux", Arch: "x64", Backend: "openvino"}
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-ubuntu-openvino-2026.4-x64.tar.gz",
		"llama-b11177-bin-ubuntu-x64.tar.gz",
	})
	best := findBestAsset(&entry, rel)
	if best == nil || best.Name != "llama-b11177-bin-ubuntu-openvino-2026.4-x64.tar.gz" {
		t.Fatalf("expected openvino 2026.4, got %v", best)
	}
}

func TestFindBestAsset_SyclFp32Preference(t *testing.T) {
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-win-sycl-x64.zip", // legacy name not in b11177 but test
		"llama-b11177-bin-ubuntu-sycl-fp16-x64.tar.gz",
		"llama-b11177-bin-ubuntu-sycl-fp32-x64.tar.gz",
		"llama-b11177-bin-win-sycl-fp16-x64.zip",
		"llama-b11177-bin-win-sycl-fp32-x64.zip",
	})
	// For windows sycl, the two win-sycl fp variants; should prefer fp32.
	entry2 := RuntimeEntry{Platform: "windows", Arch: "x64", Backend: "sycl"}
	best := findBestAsset(&entry2, rel)
	if best == nil || best.Name != "llama-b11177-bin-win-sycl-fp32-x64.zip" {
		t.Fatalf("expected sycl fp32, got %v", best)
	}
}

func TestFindBestAsset_CPUPlain(t *testing.T) {
	entry := RuntimeEntry{Platform: "linux", Arch: "x64", Backend: "cpu"}
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-ubuntu-x64.tar.gz",
		"llama-b11177-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b11177-bin-ubuntu-cuda-12.8-x64.tar.gz",
	})
	best := findBestAsset(&entry, rel)
	if best == nil || best.Name != "llama-b11177-bin-ubuntu-x64.tar.gz" {
		t.Fatalf("expected ubuntu-x64 cpu, got %v", best)
	}
}

func TestUpdateEntry_ExactAndFuzzy(t *testing.T) {
	// Exact path should still work.
	entry := RuntimeEntry{
		Platform:       "windows",
		Arch:           "x64",
		Backend:        "cuda",
		BackendVersion: "12.4",
		DownloadURL:    "https://github.com/ggml-org/llama.cpp/releases/download/b10453/llama-b10453-bin-win-cuda-12.4-x64.zip",
		Version:        "b10453",
	}
	rel := makeRelease("b11177", []string{
		"llama-b11177-bin-win-cuda-12.4-x64.zip",
		"llama-b11177-bin-win-cuda-13.4-x64.zip",
	})
	if !updateEntry("ggml-org/llama.cpp", "windows-cuda-12.4-x64", &entry, rel) {
		t.Fatal("expected updateEntry to succeed via exact")
	}
	if entry.Version != "b11177" || entry.DownloadURL == "" {
		t.Fatalf("entry not updated %v", entry)
	}

	// Fuzzy path: 13.3 -> 13.4
	entry2 := RuntimeEntry{
		Platform:       "windows",
		Arch:           "x64",
		Backend:        "cuda",
		BackendVersion: "13.3",
		DownloadURL:    "https://github.com/ggml-org/llama.cpp/releases/download/b10453/llama-b10453-bin-win-cuda-13.3-x64.zip",
		Version:        "b10453",
	}
	if !updateEntry("ggml-org/llama.cpp", "windows-cuda-13.3-x64", &entry2, rel) {
		t.Fatal("expected fuzzy updateEntry to succeed")
	}
	if entry2.BackendVersion != "13.4" {
		t.Fatalf("expected BackendVersion 13.4, got %q", entry2.BackendVersion)
	}
	if entry2.DownloadURL != "https://github.com/ggml-org/llama.cpp/releases/download/b11177/llama-b11177-bin-win-cuda-13.4-x64.zip" {
		t.Fatalf("unexpected URL %q", entry2.DownloadURL)
	}
}
