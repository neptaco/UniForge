package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestGetMajorMinorFromVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2022.3.60f1", "2022.3"},
		{"2021.3.5f1", "2021.3"},
		{"6000.0.32f1", "6000.0"},
		{"2023.1.0a1", "2023.1"},
		{"2022.3", "2022.3"},
		{"invalid", "invalid"}, // returns as-is if no dots
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := GetMajorMinorFromVersion(tt.version)
			if result != tt.expected {
				t.Errorf("GetMajorMinorFromVersion(%q) = %q, want %q", tt.version, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int // 1 if a > b, -1 if a < b, 0 if equal
	}{
		{"2022.3.60f1", "2022.3.59f1", 1},
		{"2022.3.59f1", "2022.3.60f1", -1},
		{"2022.3.60f1", "2022.3.60f1", 0},
		{"2023.1.0f1", "2022.3.60f1", 1},
		{"6000.0.1f1", "2022.3.60f1", 1},
		{"2022.3.60f1", "2021.3.60f1", 1},
		// Alpha/Beta/Final ordering
		{"6000.4.0f1", "6000.4.0b6", 1}, // final > beta
		{"6000.4.0b6", "6000.4.0a5", 1}, // beta > alpha
		{"6000.4.0b6", "6000.4.0b5", 1}, // b6 > b5
		{"6000.4.0b5", "6000.4.0b4", 1}, // b5 > b4
		{"6000.4.0a5", "6000.4.0a4", 1}, // a5 > a4
		{"6000.4.0a4", "6000.4.0a2", 1}, // a4 > a2
		{"6000.4.0b1", "6000.4.0a5", 1}, // beta > alpha even if number is lower
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := compareVersions(tt.a, tt.b)
			if (tt.expected > 0 && result <= 0) ||
				(tt.expected < 0 && result >= 0) ||
				(tt.expected == 0 && result != 0) {
				t.Errorf("compareVersions(%q, %q) = %d, want sign of %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestFilterReleasesByVersion(t *testing.T) {
	releases := []UnityRelease{
		{Version: "2022.3.60f1"},
		{Version: "2022.3.59f1"},
		{Version: "2022.3.5f1"},
		{Version: "2021.3.60f1"},
		{Version: "6000.0.32f1"},
	}

	tests := []struct {
		filter   string
		expected []string
	}{
		{"2022.3", []string{"2022.3.60f1", "2022.3.59f1", "2022.3.5f1"}},
		{"2022.3.6", []string{"2022.3.60f1"}},
		{"2022.3.5", []string{"2022.3.59f1", "2022.3.5f1"}},
		{"6000", []string{"6000.0.32f1"}},
		{"9999", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			result := FilterReleasesByVersion(releases, tt.filter)
			if len(result) != len(tt.expected) {
				t.Errorf("FilterReleasesByVersion filter=%q got %d results, want %d", tt.filter, len(result), len(tt.expected))
				return
			}
			for i, r := range result {
				if r.Version != tt.expected[i] {
					t.Errorf("FilterReleasesByVersion filter=%q result[%d] = %q, want %q", tt.filter, i, r.Version, tt.expected[i])
				}
			}
		})
	}
}

func TestMergeReleases(t *testing.T) {
	apiReleases := []UnityRelease{
		{
			Version:     "2022.3.60f1",
			Changeset:   "abc123",
			ReleaseDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Recommended: true,
		},
		{
			Version:   "2022.3.59f1",
			Changeset: "def456",
		},
	}

	localReleases := []UnityRelease{
		{
			Version: "2022.3.60f1",
			Modules: []ModuleInfo{{ID: "android", Name: "Android"}},
		},
		{
			Version: "2021.3.5f1",
			Modules: []ModuleInfo{{ID: "ios", Name: "iOS"}},
		},
	}

	result := mergeReleases(apiReleases, localReleases)

	// Should have 3 releases: 2 from API + 1 only in local
	if len(result) != 3 {
		t.Errorf("mergeReleases got %d releases, want 3", len(result))
	}

	// First release should have API metadata
	if result[0].Version != "2022.3.60f1" {
		t.Errorf("result[0].Version = %q, want %q", result[0].Version, "2022.3.60f1")
	}
	if result[0].Changeset != "abc123" {
		t.Errorf("result[0].Changeset = %q, want %q", result[0].Changeset, "abc123")
	}
	if !result[0].Recommended {
		t.Error("result[0].Recommended should be true")
	}
}

func TestDeduplicateReleases(t *testing.T) {
	releases := []UnityRelease{
		{Version: "2022.3.60f1", Changeset: ""},
		{Version: "2022.3.60f1", Changeset: "abc123"},
		{Version: "2022.3.59f1"},
		{Version: "2022.3.59f1", Modules: []ModuleInfo{{ID: "android"}}},
	}

	result := deduplicateReleases(releases)

	if len(result) != 2 {
		t.Errorf("deduplicateReleases got %d releases, want 2", len(result))
	}

	// Should keep the one with changeset
	for _, r := range result {
		if r.Version == "2022.3.60f1" && r.Changeset != "abc123" {
			t.Error("Should keep release with changeset")
		}
		if r.Version == "2022.3.59f1" && len(r.Modules) != 1 {
			t.Error("Should keep release with more modules")
		}
	}
}

func TestBuildBatchReleasesQuery(t *testing.T) {
	client := &Client{}

	versions := []string{"2022.3", "2021.3", "6000.0"}
	query := client.buildBatchReleasesQuery(versions)

	// Check query contains aliases
	if !contains(query, "v2022_3:") {
		t.Error("Query should contain alias v2022_3:")
	}
	if !contains(query, "v2021_3:") {
		t.Error("Query should contain alias v2021_3:")
	}
	if !contains(query, "v6000_0:") {
		t.Error("Query should contain alias v6000_0:")
	}

	// Check query contains version parameters
	if !contains(query, `version: "2022.3"`) {
		t.Error("Query should contain version: \"2022.3\"")
	}

	// Check query is valid structure
	if !contains(query, "query GetAllReleases {") {
		t.Error("Query should start with query GetAllReleases")
	}
	if !contains(query, "getUnityReleases") {
		t.Error("Query should contain getUnityReleases")
	}
}

func TestParseBatchReleasesResponse(t *testing.T) {
	client := &Client{}

	// Mock response with multiple version aliases
	responseJSON := `{
		"data": {
			"v2022_3": {
				"edges": [
					{
						"node": {
							"version": "2022.3.60f1",
							"shortRevision": "abc123",
							"stream": "LTS",
							"releaseDate": "2024-01-15T00:00:00Z",
							"recommended": true,
							"releaseNotes": {"url": "https://example.com/notes"},
							"downloads": [
								{
									"platform": "MAC_OS",
									"architecture": "ARM64",
									"downloadSize": {"value": 2147483648, "unit": "BYTE"},
									"installedSize": {"value": 4294967296, "unit": "BYTE"},
									"modules": [
										{
											"id": "android",
											"name": "Android Build Support",
											"category": "PLATFORM",
											"hidden": false,
											"downloadSize": {"value": 1073741824, "unit": "BYTE"},
											"installedSize": {"value": 2147483648, "unit": "BYTE"}
										}
									]
								}
							]
						}
					}
				]
			},
			"v2021_3": {
				"edges": [
					{
						"node": {
							"version": "2021.3.5f1",
							"shortRevision": "def456",
							"stream": "LTS",
							"releaseDate": "2023-06-01T00:00:00Z",
							"recommended": false,
							"releaseNotes": {"url": ""},
							"downloads": []
						}
					}
				]
			}
		}
	}`

	releases, err := client.parseBatchReleasesResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("parseBatchReleasesResponse failed: %v", err)
	}

	if len(releases) != 2 {
		t.Errorf("Expected 2 releases, got %d", len(releases))
	}

	// Find the 2022.3.60f1 release
	var found2022 *UnityRelease
	for i := range releases {
		if releases[i].Version == "2022.3.60f1" {
			found2022 = &releases[i]
			break
		}
	}

	if found2022 == nil {
		t.Fatal("Release 2022.3.60f1 not found")
	}

	if found2022.Changeset != "abc123" {
		t.Errorf("Changeset = %q, want %q", found2022.Changeset, "abc123")
	}
	if found2022.Stream != "LTS" {
		t.Errorf("Stream = %q, want %q", found2022.Stream, "LTS")
	}
	if !found2022.LTS {
		t.Error("LTS should be true")
	}
	if !found2022.Recommended {
		t.Error("Recommended should be true")
	}
	if found2022.ReleaseNotesURL != "https://example.com/notes" {
		t.Errorf("ReleaseNotesURL = %q, want %q", found2022.ReleaseNotesURL, "https://example.com/notes")
	}
}

func TestParseBatchReleasesResponse_InvalidJSON(t *testing.T) {
	client := &Client{}

	_, err := client.parseBatchReleasesResponse([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseBatchReleasesResponse_EmptyData(t *testing.T) {
	client := &Client{}

	responseJSON := `{"data": {}}`
	releases, err := client.parseBatchReleasesResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("parseBatchReleasesResponse failed: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("Expected 0 releases for empty data, got %d", len(releases))
	}
}

func TestModuleInfo_IsVisible(t *testing.T) {
	tests := []struct {
		name     string
		module   ModuleInfo
		expected bool
	}{
		{
			name:     "Platform not hidden",
			module:   ModuleInfo{Category: "PLATFORM", Hidden: false},
			expected: true,
		},
		{
			name:     "Platform hidden",
			module:   ModuleInfo{Category: "PLATFORM", Hidden: true},
			expected: false,
		},
		{
			name:     "DevTool not hidden",
			module:   ModuleInfo{Category: "DEV_TOOL", Hidden: false},
			expected: false,
		},
		{
			name:     "Documentation",
			module:   ModuleInfo{Category: "DOCUMENTATION", Hidden: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.module.IsVisible()
			if result != tt.expected {
				t.Errorf("IsVisible() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertNodeToRelease(t *testing.T) {
	client := &Client{}

	node := graphQLReleaseNode{
		Version:       "2022.3.60f1",
		ShortRevision: "abc123",
		Stream:        "LTS",
		ReleaseDate:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Recommended:   true,
		ReleaseNotes: struct {
			URL string `json:"url"`
		}{URL: "https://example.com"},
		Downloads: []graphQLDownload{
			{
				Platform:      "MAC_OS",
				Architecture:  "ARM64",
				DownloadSize:  graphQLDigitalValue{Value: 2147483648, Unit: "BYTE"},
				InstalledSize: graphQLDigitalValue{Value: 4294967296, Unit: "BYTE"},
				Modules: []graphQLModule{
					{
						ID:            "android",
						Name:          "Android Build Support",
						Category:      "PLATFORM",
						Hidden:        false,
						DownloadSize:  graphQLDigitalValue{Value: 1073741824, Unit: "BYTE"},
						InstalledSize: graphQLDigitalValue{Value: 0, Unit: "BYTE"},
					},
				},
			},
		},
	}

	release := client.convertNodeToRelease(node, "MAC_OS", "ARM64")

	if release.Version != "2022.3.60f1" {
		t.Errorf("Version = %q, want %q", release.Version, "2022.3.60f1")
	}
	if release.Changeset != "abc123" {
		t.Errorf("Changeset = %q, want %q", release.Changeset, "abc123")
	}
	if !release.LTS {
		t.Error("LTS should be true")
	}
	if !release.Recommended {
		t.Error("Recommended should be true")
	}
	if release.DownloadSize != 2147483648 {
		t.Errorf("DownloadSize = %d, want %d", release.DownloadSize, 2147483648)
	}
	if len(release.Modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(release.Modules))
	}
	if release.Modules[0].ID != "android" {
		t.Errorf("Module ID = %q, want %q", release.Modules[0].ID, "android")
	}
}

func TestConvertNodeToRelease_DifferentPlatform(t *testing.T) {
	client := &Client{}

	node := graphQLReleaseNode{
		Version: "2022.3.60f1",
		Downloads: []graphQLDownload{
			{
				Platform:      "WINDOWS",
				Architecture:  "X86_64",
				DownloadSize:  graphQLDigitalValue{Value: 1000000, Unit: "BYTE"},
				InstalledSize: graphQLDigitalValue{Value: 0, Unit: "BYTE"},
			},
		},
	}

	// Request for MAC_OS but only WINDOWS available
	release := client.convertNodeToRelease(node, "MAC_OS", "ARM64")

	if release.DownloadSize != 0 {
		t.Errorf("DownloadSize should be 0 when platform doesn't match, got %d", release.DownloadSize)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestFetchReleasesFromGraphQL_Integration tests actual API call
// Skip in CI, run manually with: go test -run Integration -v
func TestFetchReleasesFromGraphQL_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &Client{}
	versions := []string{"2022.3"}

	// Debug: Test raw API call first
	query := client.buildBatchReleasesQuery(versions)
	t.Logf("Query:\n%s", query)

	// Make raw request to check response
	reqBody := map[string]any{
		"operationName": "GetAllReleases",
		"variables":     map[string]any{},
		"query":         query,
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "https://services.unity.com/graphql", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Response status: %d", resp.StatusCode)
	t.Logf("Response body preview: %s", string(body[:min(1000, len(body))]))

	// Now test the actual function
	releases, err := client.FetchReleasesFromGraphQL(versions)
	if err != nil {
		t.Fatalf("FetchReleasesFromGraphQL failed: %v", err)
	}

	t.Logf("Got %d releases", len(releases))

	if len(releases) == 0 {
		t.Error("Expected at least one release, got 0")
	}

	// Verify first release has expected fields
	if len(releases) > 0 {
		r := releases[0]
		t.Logf("First release: %s (changeset=%s, stream=%s, date=%s, rec=%v, size=%d)",
			r.Version, r.Changeset, r.Stream, r.ReleaseDate.Format("2006-01-02"), r.Recommended, r.DownloadSize)

		if r.Version == "" {
			t.Error("Version should not be empty")
		}
		if r.Changeset == "" {
			t.Error("Changeset should not be empty")
		}
		if r.Stream == "" {
			t.Error("Stream should not be empty")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestReleaseCacheRoundTrip(t *testing.T) {
	original := UnityRelease{
		Version:         "2022.3.60f1",
		Changeset:       "abc123",
		LTS:             true,
		Stream:          "LTS",
		ReleaseDate:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Recommended:     true,
		ReleaseNotesURL: "https://example.com/notes",
		DownloadSize:    2147483648,
		InstalledSize:   4294967296,
		Modules: []ModuleInfo{
			{
				ID:            "android",
				Name:          "Android Build Support",
				Category:      "PLATFORM",
				Hidden:        false,
				DownloadSize:  1073741824,
				InstalledSize: 2147483648,
			},
		},
	}

	// Convert to cache entry
	entry := releaseCacheEntry{
		Version:         original.Version,
		Changeset:       original.Changeset,
		LTS:             original.LTS,
		Stream:          original.Stream,
		ReleaseDate:     original.ReleaseDate,
		Recommended:     original.Recommended,
		ReleaseNotesURL: original.ReleaseNotesURL,
		DownloadSize:    original.DownloadSize,
		InstalledSize:   original.InstalledSize,
	}
	for _, mod := range original.Modules {
		entry.Modules = append(entry.Modules, moduleCacheEntry{
			ID:            mod.ID,
			Name:          mod.Name,
			Category:      mod.Category,
			Hidden:        mod.Hidden,
			DownloadSize:  mod.DownloadSize,
			InstalledSize: mod.InstalledSize,
		})
	}

	// Serialize and deserialize
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed releaseCacheEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify fields
	if parsed.Version != original.Version {
		t.Errorf("Version = %q, want %q", parsed.Version, original.Version)
	}
	if parsed.Changeset != original.Changeset {
		t.Errorf("Changeset = %q, want %q", parsed.Changeset, original.Changeset)
	}
	if !parsed.ReleaseDate.Equal(original.ReleaseDate) {
		t.Errorf("ReleaseDate = %v, want %v", parsed.ReleaseDate, original.ReleaseDate)
	}
	if parsed.Recommended != original.Recommended {
		t.Errorf("Recommended = %v, want %v", parsed.Recommended, original.Recommended)
	}
	if parsed.DownloadSize != original.DownloadSize {
		t.Errorf("DownloadSize = %d, want %d", parsed.DownloadSize, original.DownloadSize)
	}
	if len(parsed.Modules) != len(original.Modules) {
		t.Errorf("Modules length = %d, want %d", len(parsed.Modules), len(original.Modules))
	}
}
