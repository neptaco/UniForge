package unity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type changesetCache struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	changeset string
	timestamp time.Time
}

var (
	csCache = &changesetCache{
		cache: make(map[string]cacheEntry),
	}
	cacheExpiration = 24 * time.Hour
)

// GraphQL request/response structures
type graphQLRequest struct {
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Query         string                 `json:"query"`
}

type graphQLResponse struct {
	Data struct {
		GetUnityReleases struct {
			Edges []struct {
				Node struct {
					Version          string `json:"version"`
					UnityHubDeepLink string `json:"unityHubDeepLink"`
					Stream           string `json:"stream"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"getUnityReleases"`
	} `json:"data"`
}

// GetChangesetForVersion fetches the changeset for a specific Unity version
func GetChangesetForVersion(version string) (string, error) {
	// Check cache first
	if changeset := getFromCache(version); changeset != "" {
		logrus.Debugf("Using cached changeset for %s: %s", version, changeset)
		return changeset, nil
	}

	// Extract major.minor version (e.g., "2022.3" from "2022.3.59f1")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid version format: %s", version)
	}
	majorMinor := parts[0] + "." + parts[1]

	logrus.Debugf("Fetching changeset for version %s from Unity API", version)

	// Prepare GraphQL query
	query := `query GetRelease($limit: Int, $skip: Int, $version: String!, $stream: [UnityReleaseStream!]) {
  getUnityReleases(
    limit: $limit
    skip: $skip
    stream: $stream
    version: $version
    entitlements: [XLTS]
  ) {
    edges {
      node {
        version
        unityHubDeepLink
        stream
      }
    }
  }
}`

	reqBody := graphQLRequest{
		OperationName: "GetRelease",
		Variables: map[string]interface{}{
			"version": majorMinor,
			"limit":   200,
		},
		Query: query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request
	req, err := http.NewRequest("POST", "https://services.unity.com/graphql", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch from Unity API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var graphQLResp graphQLResponse
	if err := json.Unmarshal(body, &graphQLResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Find the specific version
	for _, edge := range graphQLResp.Data.GetUnityReleases.Edges {
		if edge.Node.Version == version {
			// Extract changeset from deep link
			// Format: unityhub://2022.3.59f1/630718f645a5
			deepLink := edge.Node.UnityHubDeepLink
			parts := strings.Split(deepLink, "/")
			if len(parts) >= 2 {
				changeset := parts[len(parts)-1]
				
				// Cache the result
				putToCache(version, changeset)
				
				logrus.Debugf("Found changeset for %s: %s", version, changeset)
				return changeset, nil
			}
		}
	}

	return "", fmt.Errorf("changeset not found for version %s", version)
}

func getFromCache(version string) string {
	csCache.mu.RLock()
	defer csCache.mu.RUnlock()

	if entry, ok := csCache.cache[version]; ok {
		if time.Since(entry.timestamp) < cacheExpiration {
			return entry.changeset
		}
	}
	return ""
}

func putToCache(version, changeset string) {
	csCache.mu.Lock()
	defer csCache.mu.Unlock()

	csCache.cache[version] = cacheEntry{
		changeset: changeset,
		timestamp: time.Now(),
	}
}