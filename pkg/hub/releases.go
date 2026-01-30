package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

// UnityRelease represents a Unity release with its metadata
type UnityRelease struct {
	Version         string
	Changeset       string
	LTS             bool
	Stream          string // "LTS", "TECH", "BETA", "SUPPORTED"
	Modules         []ModuleInfo
	Installed       bool
	InstalledPath   string
	Architecture    string // "x86_64" or "arm64"
	ReleaseDate     time.Time
	Recommended     bool
	ReleaseNotesURL string
	DownloadSize    int64  // bytes
	InstalledSize   int64  // bytes
	SecurityAlert   string // Security alert message if any
}

// ModuleInfo represents a module available for a Unity version
type ModuleInfo struct {
	ID            string
	Name          string
	Description   string
	Category      string // "PLATFORM", "DEV_TOOL", "LANGUAGE_PACK", "DOCUMENTATION"
	Installed     bool
	Hidden        bool
	DownloadSize  int64 // bytes
	InstalledSize int64 // bytes
}

// IsVisible returns true if the module should be shown in UI
func (m ModuleInfo) IsVisible() bool {
	return !m.Hidden && m.Category == "PLATFORM"
}

// VersionStream represents a major.minor version stream (e.g., "2022.3 LTS")
type VersionStream struct {
	MajorMinor    string // e.g., "2022.3"
	DisplayName   string // e.g., "2022.3 LTS"
	TotalCount    int
	LatestVersion string
	LTS           bool
	IsUnity6      bool
}

// releasesFileData represents the structure of releases.json
type releasesFileData struct {
	Official []releasesFileEntry `json:"official"`
	Beta     []releasesFileEntry `json:"beta"`
}

type releasesFileEntry struct {
	Version      string                   `json:"version"`
	LTS          bool                     `json:"lts"`
	DownloadURL  string                   `json:"downloadUrl"`
	Arch         string                   `json:"arch"`
	Modules      []releasesFileModuleInfo `json:"modules"`
	DownloadSize int64                    `json:"downloadSize"`
}

type releasesFileModuleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
}

// graphQLReleasesRequest is the request for fetching releases via GraphQL
type graphQLReleasesRequest struct {
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
	Query         string         `json:"query"`
}

// graphQLReleasesResponse is the response from the Unity GraphQL API
type graphQLReleasesResponse struct {
	Data struct {
		GetUnityReleases struct {
			TotalCount int `json:"totalCount"`
			Edges      []struct {
				Node graphQLReleaseNode `json:"node"`
			} `json:"edges"`
		} `json:"getUnityReleases"`
	} `json:"data"`
}

type graphQLReleaseNode struct {
	Version       string    `json:"version"`
	ShortRevision string    `json:"shortRevision"`
	Stream        string    `json:"stream"`
	ReleaseDate   time.Time `json:"releaseDate"`
	Recommended   bool      `json:"recommended"`
	ReleaseNotes  struct {
		URL string `json:"url"`
	} `json:"releaseNotes"`
	Label *struct {
		LabelText string `json:"labelText"`
	} `json:"label"`
	Downloads []graphQLDownload `json:"downloads"`
}

type graphQLDigitalValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type graphQLDownload struct {
	Platform      string              `json:"platform"`
	Architecture  string              `json:"architecture"`
	DownloadSize  graphQLDigitalValue `json:"downloadSize"`
	InstalledSize graphQLDigitalValue `json:"installedSize"`
	Modules       []graphQLModule     `json:"modules"`
}

type graphQLModule struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Category      string              `json:"category"`
	Hidden        bool                `json:"hidden"`
	DownloadSize  graphQLDigitalValue `json:"downloadSize"`
	InstalledSize graphQLDigitalValue `json:"installedSize"`
}

// releasesCacheData represents the cached release data
type releasesCacheData struct {
	Streams   map[string]streamCacheEntry `json:"streams"`
	Releases  []releaseCacheEntry         `json:"releases"`
	UpdatedAt time.Time                   `json:"updatedAt"`
}

type streamCacheEntry struct {
	TotalCount    int    `json:"totalCount"`
	LatestVersion string `json:"latestVersion"`
	LTS           bool   `json:"lts"`
}

type releaseCacheEntry struct {
	Version         string             `json:"version"`
	Changeset       string             `json:"changeset"`
	LTS             bool               `json:"lts"`
	Stream          string             `json:"stream"`
	ReleaseDate     time.Time          `json:"releaseDate,omitempty"`
	Recommended     bool               `json:"recommended,omitempty"`
	ReleaseNotesURL string             `json:"releaseNotesUrl,omitempty"`
	DownloadSize    int64              `json:"downloadSize,omitempty"`
	InstalledSize   int64              `json:"installedSize,omitempty"`
	Modules         []moduleCacheEntry `json:"modules,omitempty"`
}

type moduleCacheEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Hidden        bool   `json:"hidden,omitempty"`
	DownloadSize  int64  `json:"downloadSize,omitempty"`
	InstalledSize int64  `json:"installedSize,omitempty"`
}

// baseMajorVersions is the baseline list of major versions (fallback)
var baseMajorVersions = []string{
	"6000.0", "6000.1", "6000.2", "6000.3", // Unity 6
	"2023.1", "2023.2", "2023.3",
	"2022.1", "2022.2", "2022.3",
	"2021.1", "2021.2", "2021.3",
	"2020.1", "2020.2", "2020.3",
	"2019.4",
}

// DiscoverMajorVersions discovers all major versions from multiple sources
func (c *Client) DiscoverMajorVersions() []string {
	seen := make(map[string]bool)

	// 1. Fetch from GraphQL API (authoritative source)
	if apiVersions, err := c.fetchMajorVersionsFromAPI(); err == nil {
		for _, v := range apiVersions {
			seen[v] = true
		}
	}

	// 2. Fallback to baseline if API failed
	if len(seen) == 0 {
		for _, v := range baseMajorVersions {
			seen[v] = true
		}
	} else {
		// Always add Unity 6 versions (SUPPORTED stream API may not return them)
		for _, v := range baseMajorVersions {
			if strings.HasPrefix(v, "6000.") {
				seen[v] = true
			}
		}
	}

	// 3. Extract from cache (may have versions not in current API response)
	if !c.NoCache {
		if cache, err := c.LoadCache(); err == nil && cache != nil {
			for _, entry := range cache.Releases {
				mm := GetMajorMinorFromVersion(entry.Version)
				if mm != "" {
					seen[mm] = true
				}
			}
			for mm := range cache.Streams {
				seen[mm] = true
			}
		}
	}

	// 4. Extract from Unity Hub's releases.json
	if releases, err := c.LoadReleasesFromFile(); err == nil {
		for _, r := range releases {
			mm := GetMajorMinorFromVersion(r.Version)
			if mm != "" {
				seen[mm] = true
			}
		}
	}

	// Convert to slice and sort
	var result []string
	for mm := range seen {
		result = append(result, mm)
	}

	sort.Slice(result, func(i, j int) bool {
		return compareVersions(result[i]+".0", result[j]+".0") > 0
	})

	return result
}

// fetchMajorVersionsFromAPI fetches all major versions from GraphQL API
func (c *Client) fetchMajorVersionsFromAPI() ([]string, error) {
	// Query all streams to get complete version list
	query := `query GetMajorVersions {
  lts: getUnityReleaseMajorVersions(stream: LTS) { version }
  tech: getUnityReleaseMajorVersions(stream: TECH) { version }
  beta: getUnityReleaseMajorVersions(stream: BETA) { version }
  supported: getUnityReleaseMajorVersions(stream: SUPPORTED) { version }
}`

	reqBody := graphQLReleasesRequest{
		OperationName: "GetMajorVersions",
		Variables:     map[string]any{},
		Query:         query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://services.unity.com/graphql", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			LTS []struct {
				Version string `json:"version"`
			} `json:"lts"`
			Tech []struct {
				Version string `json:"version"`
			} `json:"tech"`
			Beta []struct {
				Version string `json:"version"`
			} `json:"beta"`
			Supported []struct {
				Version string `json:"version"`
			} `json:"supported"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	for _, v := range result.Data.LTS {
		seen[v.Version] = true
	}
	for _, v := range result.Data.Tech {
		seen[v.Version] = true
	}
	for _, v := range result.Data.Beta {
		seen[v.Version] = true
	}
	for _, v := range result.Data.Supported {
		seen[v.Version] = true
	}

	var versions []string
	for v := range seen {
		versions = append(versions, v)
	}

	return versions, nil
}

// GetReleasesFilePath returns the path to Unity Hub's releases.json
func (c *Client) GetReleasesFilePath() string {
	var basePath string

	switch runtime.GOOS {
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "UnityHub")
	case "windows":
		basePath = filepath.Join(os.Getenv("APPDATA"), "UnityHub")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "UnityHub")
	default:
		return ""
	}

	return filepath.Join(basePath, "releases.json")
}

// getCacheFilePath returns the path to uniforge's release cache
func (c *Client) getReleaseCacheFilePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "uniforge", "releases-cache.json")
}

// LoadReleasesFromFile loads releases from Unity Hub's releases.json
func (c *Client) LoadReleasesFromFile() ([]UnityRelease, error) {
	releasesFilePath := c.GetReleasesFilePath()
	if releasesFilePath == "" {
		return nil, fmt.Errorf("could not determine releases file path")
	}

	data, err := os.ReadFile(releasesFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []UnityRelease{}, nil
		}
		return nil, fmt.Errorf("failed to read releases file: %w", err)
	}

	var releasesData releasesFileData
	if err := json.Unmarshal(data, &releasesData); err != nil {
		return nil, fmt.Errorf("failed to parse releases file: %w", err)
	}

	var result []UnityRelease

	// Process official releases
	for _, entry := range releasesData.Official {
		release := c.convertFileEntryToRelease(entry)
		release.Stream = "TECH"
		if entry.LTS {
			release.Stream = "LTS"
		}
		result = append(result, release)
	}

	// Process beta releases
	for _, entry := range releasesData.Beta {
		release := c.convertFileEntryToRelease(entry)
		release.Stream = "BETA"
		result = append(result, release)
	}

	return result, nil
}

// convertFileEntryToRelease converts a releases.json entry to UnityRelease
func (c *Client) convertFileEntryToRelease(entry releasesFileEntry) UnityRelease {
	release := UnityRelease{
		Version:      entry.Version,
		LTS:          entry.LTS,
		Architecture: entry.Arch,
	}

	// Extract changeset from download URL
	// Format: https://download.unity3d.com/download_unity/ffc62b691db5/...
	if entry.DownloadURL != "" {
		parts := strings.Split(entry.DownloadURL, "/")
		for i, part := range parts {
			if part == "download_unity" && i+1 < len(parts) {
				release.Changeset = parts[i+1]
				break
			}
		}
	}

	// Convert modules
	for _, mod := range entry.Modules {
		release.Modules = append(release.Modules, ModuleInfo{
			ID:          mod.ID,
			Name:        mod.Name,
			Description: mod.Description,
			Category:    mod.Category,
			Hidden:      !mod.Visible,
		})
	}

	return release
}

// FetchStreams fetches stream metadata (totalCount, latestVersion) from GraphQL API
func (c *Client) FetchStreams() ([]VersionStream, error) {
	majorVersions := c.DiscoverMajorVersions()

	var streams []VersionStream
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(majorVersions))

	for _, majorMinor := range majorVersions {
		wg.Add(1)
		go func(mm string) {
			defer wg.Done()

			stream, err := c.fetchStreamMetadata(mm)
			if err != nil {
				ui.Debug("Failed to fetch stream metadata", "version", mm, "error", err)
				errChan <- err
				return
			}

			if stream.TotalCount > 0 {
				mu.Lock()
				streams = append(streams, stream)
				mu.Unlock()
			}
		}(majorMinor)
	}

	wg.Wait()
	close(errChan)

	// Sort streams by version (newest first)
	sort.Slice(streams, func(i, j int) bool {
		return compareVersions(streams[i].MajorMinor+".0", streams[j].MajorMinor+".0") > 0
	})

	return streams, nil
}

// fetchStreamMetadata fetches metadata for a single stream
func (c *Client) fetchStreamMetadata(majorMinor string) (VersionStream, error) {
	query := `query GetRelease($limit: Int, $version: String!) {
  getUnityReleases(
    limit: $limit
    version: $version
    entitlements: [XLTS]
  ) {
    totalCount
    edges {
      node {
        version
        stream
      }
    }
  }
}`

	reqBody := graphQLReleasesRequest{
		OperationName: "GetRelease",
		Variables: map[string]any{
			"version": majorMinor,
			"limit":   1,
		},
		Query: query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return VersionStream{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://services.unity.com/graphql", bytes.NewBuffer(jsonBody))
	if err != nil {
		return VersionStream{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return VersionStream{}, fmt.Errorf("failed to fetch from Unity API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return VersionStream{}, fmt.Errorf("failed to read response: %w", err)
	}

	var graphQLResp graphQLReleasesResponse
	if err := json.Unmarshal(body, &graphQLResp); err != nil {
		return VersionStream{}, fmt.Errorf("failed to parse response: %w", err)
	}

	stream := VersionStream{
		MajorMinor: majorMinor,
		TotalCount: graphQLResp.Data.GetUnityReleases.TotalCount,
		IsUnity6:   strings.HasPrefix(majorMinor, "6000"),
	}

	if len(graphQLResp.Data.GetUnityReleases.Edges) > 0 {
		node := graphQLResp.Data.GetUnityReleases.Edges[0].Node
		stream.LatestVersion = node.Version
		stream.LTS = node.Stream == "LTS"
	}

	// Build display name
	stream.DisplayName = majorMinor
	if stream.IsUnity6 {
		stream.DisplayName = fmt.Sprintf("Unity 6 (%s)", majorMinor)
	}
	if stream.LTS {
		stream.DisplayName += " LTS"
	}

	return stream, nil
}

// FetchReleasesForStream fetches all releases for a specific stream
func (c *Client) FetchReleasesForStream(majorMinor string) ([]UnityRelease, error) {
	return c.FetchReleasesFromGraphQL([]string{majorMinor})
}

// FetchReleasesFromGraphQL fetches releases from Unity's GraphQL API
func (c *Client) FetchReleasesFromGraphQL(majorMinorVersions []string) ([]UnityRelease, error) {
	if len(majorMinorVersions) == 0 {
		return nil, nil
	}

	// Build a single GraphQL query with aliases for all versions
	query := c.buildBatchReleasesQuery(majorMinorVersions)

	reqBody := graphQLReleasesRequest{
		OperationName: "GetAllReleases",
		Variables:     map[string]any{},
		Query:         query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://services.unity.com/graphql", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from Unity API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return c.parseBatchReleasesResponse(body)
}

// buildBatchReleasesQuery builds a GraphQL query with aliases for multiple versions
func (c *Client) buildBatchReleasesQuery(versions []string) string {
	var sb strings.Builder
	sb.WriteString("query GetAllReleases {\n")

	fragment := `
    edges {
      node {
        version
        shortRevision
        stream
        releaseDate
        recommended
        releaseNotes { url }
        label { labelText }
        downloads {
          ... on UnityReleaseHubDownload {
            platform
            architecture
            downloadSize { value unit }
            installedSize { value unit }
            modules {
              id
              name
              description
              category
              hidden
              downloadSize { value unit }
              installedSize { value unit }
            }
          }
        }
      }
    }`

	for _, v := range versions {
		// Convert version to valid GraphQL alias (e.g., "2022.3" -> "v2022_3")
		alias := "v" + strings.ReplaceAll(v, ".", "_")
		sb.WriteString(fmt.Sprintf("  %s: getUnityReleases(version: \"%s\", limit: 200, entitlements: [XLTS]) {%s}\n", alias, v, fragment))
	}

	sb.WriteString("}")
	return sb.String()
}

// parseBatchReleasesResponse parses the batch response with dynamic aliases
func (c *Client) parseBatchReleasesResponse(body []byte) ([]UnityRelease, error) {
	// Parse as generic map since aliases are dynamic
	var resp struct {
		Data map[string]struct {
			Edges []struct {
				Node graphQLReleaseNode `json:"node"`
			} `json:"edges"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	currentPlatform, currentArch := c.detectPlatformArch()
	var allReleases []UnityRelease

	for _, versionData := range resp.Data {
		for _, edge := range versionData.Edges {
			release := c.convertNodeToRelease(edge.Node, currentPlatform, currentArch)
			allReleases = append(allReleases, release)
		}
	}

	return allReleases, nil
}

// convertNodeToRelease converts a GraphQL node to UnityRelease
func (c *Client) convertNodeToRelease(node graphQLReleaseNode, platform, arch string) UnityRelease {
	release := UnityRelease{
		Version:         node.Version,
		Changeset:       node.ShortRevision,
		Stream:          node.Stream,
		LTS:             node.Stream == "LTS",
		ReleaseDate:     node.ReleaseDate,
		Recommended:     node.Recommended,
		ReleaseNotesURL: node.ReleaseNotes.URL,
	}

	// Set security alert if label exists
	if node.Label != nil && node.Label.LabelText != "" {
		release.SecurityAlert = node.Label.LabelText
	}

	for _, dl := range node.Downloads {
		if dl.Platform == platform && dl.Architecture == arch {
			release.DownloadSize = int64(dl.DownloadSize.Value)
			release.InstalledSize = int64(dl.InstalledSize.Value)

			for _, mod := range dl.Modules {
				release.Modules = append(release.Modules, ModuleInfo{
					ID:            mod.ID,
					Name:          mod.Name,
					Description:   mod.Description,
					Category:      mod.Category,
					Hidden:        mod.Hidden,
					DownloadSize:  int64(mod.DownloadSize.Value),
					InstalledSize: int64(mod.InstalledSize.Value),
				})
			}
			break
		}
	}

	return release
}

// detectPlatformArch returns the current platform and architecture for GraphQL
func (c *Client) detectPlatformArch() (platform, arch string) {
	switch runtime.GOOS {
	case "darwin":
		platform = "MAC_OS"
	case "windows":
		platform = "WINDOWS"
	case "linux":
		platform = "LINUX"
	default:
		platform = "WINDOWS"
	}

	if runtime.GOARCH == "arm64" {
		arch = "ARM64"
	} else {
		arch = "X86_64"
	}

	return platform, arch
}

// ClearCache removes the cache file
func (c *Client) ClearCache() error {
	cachePath := c.getReleaseCacheFilePath()

	if err := os.Remove(cachePath); err != nil {
		if os.IsNotExist(err) {
			// Cache doesn't exist, nothing to clear
			return nil
		}
		return err
	}
	return nil
}

// LoadCache loads cached releases
func (c *Client) LoadCache() (*releasesCacheData, error) {
	cachePath := c.getReleaseCacheFilePath()

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cache releasesCacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// SaveCache saves releases to cache
func (c *Client) SaveCache(streams []VersionStream, releases []UnityRelease) error {
	cachePath := c.getReleaseCacheFilePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}

	cache := releasesCacheData{
		Streams:   make(map[string]streamCacheEntry),
		UpdatedAt: time.Now(),
	}

	for _, s := range streams {
		cache.Streams[s.MajorMinor] = streamCacheEntry{
			TotalCount:    s.TotalCount,
			LatestVersion: s.LatestVersion,
			LTS:           s.LTS,
		}
	}

	for _, r := range releases {
		entry := releaseCacheEntry{
			Version:         r.Version,
			Changeset:       r.Changeset,
			LTS:             r.LTS,
			Stream:          r.Stream,
			ReleaseDate:     r.ReleaseDate,
			Recommended:     r.Recommended,
			ReleaseNotesURL: r.ReleaseNotesURL,
			DownloadSize:    r.DownloadSize,
			InstalledSize:   r.InstalledSize,
		}

		// Convert modules
		for _, mod := range r.Modules {
			entry.Modules = append(entry.Modules, moduleCacheEntry{
				ID:            mod.ID,
				Name:          mod.Name,
				Category:      mod.Category,
				Hidden:        mod.Hidden,
				DownloadSize:  mod.DownloadSize,
				InstalledSize: mod.InstalledSize,
			})
		}

		cache.Releases = append(cache.Releases, entry)
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0644)
}

// CheckCacheValidity checks if cache is valid by comparing totalCount
func (c *Client) CheckCacheValidity(cache *releasesCacheData, currentStreams []VersionStream) bool {
	if cache == nil || len(cache.Streams) == 0 {
		return false
	}

	for _, stream := range currentStreams {
		cached, exists := cache.Streams[stream.MajorMinor]
		if !exists {
			return false
		}
		if cached.TotalCount != stream.TotalCount {
			ui.Debug("Cache invalid: totalCount changed", "stream", stream.MajorMinor,
				"cached", cached.TotalCount, "current", stream.TotalCount)
			return false
		}
	}

	return true
}

// ConvertCacheToReleases converts cached entries to UnityRelease
func (c *Client) ConvertCacheToReleases(cache *releasesCacheData) []UnityRelease {
	var releases []UnityRelease
	for _, entry := range cache.Releases {
		release := UnityRelease{
			Version:         entry.Version,
			Changeset:       entry.Changeset,
			LTS:             entry.LTS,
			Stream:          entry.Stream,
			ReleaseDate:     entry.ReleaseDate,
			Recommended:     entry.Recommended,
			ReleaseNotesURL: entry.ReleaseNotesURL,
			DownloadSize:    entry.DownloadSize,
			InstalledSize:   entry.InstalledSize,
		}

		// Convert modules
		for _, mod := range entry.Modules {
			release.Modules = append(release.Modules, ModuleInfo{
				ID:            mod.ID,
				Name:          mod.Name,
				Category:      mod.Category,
				Hidden:        mod.Hidden,
				DownloadSize:  mod.DownloadSize,
				InstalledSize: mod.InstalledSize,
			})
		}

		releases = append(releases, release)
	}
	return releases
}

// GetAllReleases loads releases from cache or API, enriches with install status
func (c *Client) GetAllReleases() ([]UnityRelease, error) {
	// Load from releases.json (has module info)
	localReleases, err := c.LoadReleasesFromFile()
	if err != nil {
		ui.Debug("Failed to load releases from file", "error", err)
		localReleases = []UnityRelease{}
	}

	// Fetch from GraphQL API (has all versions)
	majorVersions := c.DiscoverMajorVersions()
	apiReleases, err := c.FetchReleasesFromGraphQL(majorVersions)
	if err != nil {
		ui.Debug("Failed to fetch releases from GraphQL", "error", err)
	}

	// Merge: API releases + local releases (local has module info)
	releases := mergeReleases(apiReleases, localReleases)

	// Deduplicate releases by version
	releases = deduplicateReleases(releases)

	// Enrich with install status
	releases = c.EnrichReleasesWithInstallStatus(releases)

	// Sort by release date (newest first), fallback to version comparison
	sort.Slice(releases, func(i, j int) bool {
		// If both have release dates, sort by date
		if !releases[i].ReleaseDate.IsZero() && !releases[j].ReleaseDate.IsZero() {
			return releases[i].ReleaseDate.After(releases[j].ReleaseDate)
		}
		// Fallback to version comparison
		return compareVersions(releases[i].Version, releases[j].Version) > 0
	})

	return releases, nil
}

// mergeReleases merges API releases with local releases
// API releases have metadata (releaseDate, recommended, etc.)
// Local releases may have additional module info
func mergeReleases(apiReleases, localReleases []UnityRelease) []UnityRelease {
	// Create a map of local releases for quick lookup
	localMap := make(map[string]UnityRelease)
	for _, r := range localReleases {
		localMap[r.Version] = r
	}

	var result []UnityRelease

	// Add API releases, enriching with local module info if available
	for _, apiRelease := range apiReleases {
		if localRelease, exists := localMap[apiRelease.Version]; exists {
			// Use API release (has metadata), but add local modules if API has none
			if len(apiRelease.Modules) == 0 && len(localRelease.Modules) > 0 {
				apiRelease.Modules = localRelease.Modules
			}
			delete(localMap, apiRelease.Version)
		}
		result = append(result, apiRelease)
	}

	// Add remaining local releases that weren't in API
	for _, r := range localReleases {
		if _, exists := localMap[r.Version]; exists {
			result = append(result, r)
		}
	}

	return result
}

// deduplicateReleases removes duplicate versions, keeping the one with more module info
func deduplicateReleases(releases []UnityRelease) []UnityRelease {
	seen := make(map[string]int) // version -> index in result
	var result []UnityRelease

	for _, r := range releases {
		if idx, exists := seen[r.Version]; exists {
			// Keep the one with more modules or changeset
			existing := result[idx]
			if len(r.Modules) > len(existing.Modules) ||
				(r.Changeset != "" && existing.Changeset == "") {
				result[idx] = r
			}
		} else {
			seen[r.Version] = len(result)
			result = append(result, r)
		}
	}

	return result
}

// EnrichReleasesWithInstallStatus adds install status to releases
func (c *Client) EnrichReleasesWithInstallStatus(releases []UnityRelease) []UnityRelease {
	// Get installed editors
	installedEditors, err := c.ListInstalledEditors()
	if err != nil {
		ui.Debug("Failed to list installed editors", "error", err)
		return releases
	}

	// Create a map of installed versions
	installedMap := make(map[string]EditorInfo)
	for _, editor := range installedEditors {
		installedMap[editor.Version] = editor
	}

	// Update releases with install status
	for i := range releases {
		if editor, ok := installedMap[releases[i].Version]; ok {
			releases[i].Installed = true
			releases[i].InstalledPath = editor.Path

			// Enrich modules with install status
			for j := range releases[i].Modules {
				releases[i].Modules[j].Installed = c.IsModuleInstalled(editor.Path, releases[i].Modules[j].ID)
			}
		}
	}

	return releases
}

// GetCommonModules returns a list of commonly used modules
func GetCommonModules() []ModuleInfo {
	return []ModuleInfo{
		{ID: "android", Name: "Android Build Support", Category: "PLATFORM"},
		{ID: "ios", Name: "iOS Build Support", Category: "PLATFORM"},
		{ID: "webgl", Name: "WebGL Build Support", Category: "PLATFORM"},
		{ID: "windows-il2cpp", Name: "Windows Build Support (IL2CPP)", Category: "PLATFORM"},
		{ID: "linux-il2cpp", Name: "Linux Build Support (IL2CPP)", Category: "PLATFORM"},
		{ID: "mac-il2cpp", Name: "Mac Build Support (IL2CPP)", Category: "PLATFORM"},
	}
}

// compareVersions compares two Unity version strings
// Returns: >0 if v1 > v2, <0 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Parse version like "2022.3.60f1", "6000.4.0b3", "6000.4.0a5"
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)

	// Compare each part
	for i := 0; i < len(p1) && i < len(p2); i++ {
		if p1[i] > p2[i] {
			return 1
		}
		if p1[i] < p2[i] {
			return -1
		}
	}

	// If all parts are equal, longer version is greater
	return len(p1) - len(p2)
}

// parseVersionParts parses a version string into comparable integer parts
// Format: major.minor.patch[a|b|f]N -> [major, minor, patch, releaseType, releaseNum]
// releaseType: alpha=1, beta=2, final=3
func parseVersionParts(version string) []int {
	var parts []int

	// Split by dot
	dotParts := strings.Split(version, ".")
	for i, part := range dotParts {
		if i == len(dotParts)-1 {
			// Last part may contain suffix like "60f1", "0b3", "0a5"
			num, releaseType, releaseNum := parseVersionSuffix(part)
			parts = append(parts, num, releaseType, releaseNum)
		} else {
			var num int
			_, _ = fmt.Sscanf(part, "%d", &num)
			parts = append(parts, num)
		}
	}

	return parts
}

// parseVersionSuffix parses "60f1" -> (60, 3, 1), "0b3" -> (0, 2, 3), "0a5" -> (0, 1, 5)
func parseVersionSuffix(part string) (num, releaseType, releaseNum int) {
	// Find where the letter starts
	letterIdx := -1
	for i, c := range part {
		if c == 'a' || c == 'b' || c == 'f' {
			letterIdx = i
			break
		}
	}

	if letterIdx == -1 {
		// No suffix, treat as final release
		_, _ = fmt.Sscanf(part, "%d", &num)
		return num, 3, 0
	}

	// Parse number before letter
	_, _ = fmt.Sscanf(part[:letterIdx], "%d", &num)

	// Parse release type
	switch part[letterIdx] {
	case 'a':
		releaseType = 1 // alpha
	case 'b':
		releaseType = 2 // beta
	case 'f':
		releaseType = 3 // final
	}

	// Parse release number after letter
	if letterIdx+1 < len(part) {
		_, _ = fmt.Sscanf(part[letterIdx+1:], "%d", &releaseNum)
	}

	return
}

// FilterReleasesByVersion filters releases that match a version prefix
func FilterReleasesByVersion(releases []UnityRelease, prefix string) []UnityRelease {
	if prefix == "" {
		return releases
	}

	prefix = strings.ToLower(prefix)
	var result []UnityRelease
	for _, r := range releases {
		if strings.Contains(strings.ToLower(r.Version), prefix) {
			result = append(result, r)
		}
	}
	return result
}

// GetMajorMinorFromVersion extracts major.minor from a version string
func GetMajorMinorFromVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
