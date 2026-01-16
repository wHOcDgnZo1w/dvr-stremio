package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Configuration
var (
	easyProxyURL      string
	easyProxyPassword string
	port              string
)

// Stremio manifest
var manifest = map[string]interface{}{
	"id":          "org.stremio.dvr-local",
	"version":     "1.0.0",
	"name":        "DVR Recordings",
	"description": "Local addon for EasyProxy DVR recordings",
	"resources":   []string{"catalog", "stream", "meta"},
	"types":       []string{"tv"},
	"catalogs": []map[string]interface{}{
		{
			"type": "tv",
			"id":   "dvr-recordings",
			"name": "DVR Recordings",
			"extra": []map[string]interface{}{
				{
					"name":       "genre",
					"isRequired": false,
					"options":    []string{"All Recordings"},
				},
				{
					"name":       "search",
					"isRequired": false,
				},
			},
		},
	},
	"idPrefixes": []string{"dvr:"},
}

// Recording represents an EasyProxy recording
type Recording struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	URL             string  `json:"url"`
	FilePath        string  `json:"file_path"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at"`
	StoppedAt       string  `json:"stopped_at,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	FileSizeBytes   int64   `json:"file_size_bytes,omitempty"`
	IsActive        bool    `json:"is_active"`
	ElapsedSeconds  float64 `json:"elapsed_seconds,omitempty"`
}

// RecordingsResponse from EasyProxy API
type RecordingsResponse struct {
	Recordings []Recording `json:"recordings"`
}

// StremioMeta for catalog items
type StremioMeta struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Poster      string `json:"poster,omitempty"`
	Description string `json:"description,omitempty"`
	ReleaseInfo string `json:"releaseInfo,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
}

// StremioStream for stream items
type StremioStream struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func init() {
	easyProxyURL = strings.TrimRight(getEnv("EASYPROXY_URL", "http://localhost:8080"), "/")
	easyProxyPassword = getEnv("EASYPROXY_PASSWORD", "")
	port = getEnv("PORT", "7001")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// CORS middleware
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// JSON response helper
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Fetch recordings from EasyProxy
func fetchRecordings() ([]Recording, error) {
	params := url.Values{}
	if easyProxyPassword != "" {
		params.Set("api_password", easyProxyPassword)
	}

	reqURL := fmt.Sprintf("%s/api/recordings?%s", easyProxyURL, params.Encode())

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if easyProxyPassword != "" {
		req.Header.Set("x-api-password", easyProxyPassword)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result RecordingsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Recordings, nil
}

// Format duration as human readable
func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// Format file size as human readable
func formatFileSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(bytes)
	unitIndex := 0
	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}
	return fmt.Sprintf("%.1f%s", size, units[unitIndex])
}

// Convert recording to Stremio meta
func recordingToMeta(rec Recording) StremioMeta {
	size := formatFileSize(rec.FileSizeBytes)

	var date string
	if rec.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, rec.StartedAt); err == nil {
			date = t.Format("2006-01-02")
		}
	}

	name := rec.Name
	if name == "" {
		name = "Unknown Recording"
	}

	var description string
	var runtime string

	// Handle active recordings differently
	if rec.IsActive && rec.Status == "recording" {
		elapsed := formatDuration(rec.ElapsedSeconds)
		name = "🔴 " + name
		description = "Recording in progress..."
		if elapsed != "" {
			description += fmt.Sprintf("\nElapsed: %s", elapsed)
		}
		if size != "" {
			description += fmt.Sprintf(" | Size: %s", size)
		}
		runtime = elapsed
	} else {
		duration := formatDuration(rec.DurationSeconds)
		details := []string{}
		if duration != "" {
			details = append(details, duration)
		}
		if size != "" {
			details = append(details, size)
		}
		if date != "" {
			details = append(details, date)
		}

		description = fmt.Sprintf("Status: %s", rec.Status)
		if len(details) > 0 {
			description += "\n" + strings.Join(details, " | ")
		}
		runtime = duration
	}

	return StremioMeta{
		ID:          "dvr:" + rec.ID,
		Type:        "tv",
		Name:        name,
		Description: description,
		ReleaseInfo: date,
		Runtime:     runtime,
	}
}

// Handler: Manifest
func handleManifest(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, manifest)
}

// Handler: Catalog
func handleCatalog(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/catalog/")
	parts := strings.Split(strings.TrimSuffix(path, ".json"), "/")

	if len(parts) < 2 || parts[0] != "tv" || !strings.HasPrefix(parts[1], "dvr-recordings") {
		jsonResponse(w, map[string][]StremioMeta{"metas": {}})
		return
	}

	// Extract search query if present (format: dvr-recordings/search=query)
	searchQuery := ""
	if len(parts) > 2 {
		for _, part := range parts[2:] {
			if strings.HasPrefix(part, "search=") {
				searchQuery = strings.ToLower(strings.TrimPrefix(part, "search="))
				// URL decode the search query
				if decoded, err := url.QueryUnescape(searchQuery); err == nil {
					searchQuery = strings.ToLower(decoded)
				}
			}
		}
	}

	log.Printf("[DVR] Fetching recordings catalog (search: %q)...", searchQuery)
	recordings, err := fetchRecordings()
	if err != nil {
		log.Printf("[DVR] Error fetching recordings: %v", err)
		jsonResponse(w, map[string][]StremioMeta{"metas": {}})
		return
	}

	// Separate active and completed recordings
	var active []Recording
	var completed []Recording
	for _, rec := range recordings {
		// Apply search filter if present
		if searchQuery != "" {
			if !strings.Contains(strings.ToLower(rec.Name), searchQuery) {
				continue
			}
		}

		if rec.IsActive && rec.Status == "recording" {
			active = append(active, rec)
		} else {
			hasValidFile := rec.FileSizeBytes > 0
			isFinished := rec.Status == "completed" || rec.Status == "stopped" || rec.Status == "failed"
			if isFinished && hasValidFile {
				completed = append(completed, rec)
			}
		}
	}

	// Sort active by start time (newest first)
	sort.Slice(active, func(i, j int) bool {
		return active[i].StartedAt > active[j].StartedAt
	})

	// Sort completed by date (newest first)
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].StartedAt > completed[j].StartedAt
	})

	// Combine: active first, then completed
	valid := append(active, completed...)

	metas := make([]StremioMeta, len(valid))
	for i, rec := range valid {
		metas[i] = recordingToMeta(rec)
	}

	log.Printf("[DVR] Returning %d recordings", len(metas))
	jsonResponse(w, map[string][]StremioMeta{"metas": metas})
}

// Handler: Meta
func handleMeta(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/meta/")
	parts := strings.Split(strings.TrimSuffix(path, ".json"), "/")

	if len(parts) != 2 || parts[0] != "tv" || !strings.HasPrefix(parts[1], "dvr:") {
		jsonResponse(w, map[string]interface{}{"meta": nil})
		return
	}

	recordingID := strings.TrimPrefix(parts[1], "dvr:")

	recordings, err := fetchRecordings()
	if err != nil {
		jsonResponse(w, map[string]interface{}{"meta": nil})
		return
	}

	for _, rec := range recordings {
		if rec.ID == recordingID {
			jsonResponse(w, map[string]StremioMeta{"meta": recordingToMeta(rec)})
			return
		}
	}

	jsonResponse(w, map[string]interface{}{"meta": nil})
}

// Handler: Stream
func handleStream(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/stream/")
	parts := strings.Split(strings.TrimSuffix(path, ".json"), "/")

	if len(parts) != 2 || parts[0] != "tv" || !strings.HasPrefix(parts[1], "dvr:") {
		jsonResponse(w, map[string][]StremioStream{"streams": {}})
		return
	}

	recordingID := strings.TrimPrefix(parts[1], "dvr:")

	params := url.Values{}
	if easyProxyPassword != "" {
		params.Set("api_password", easyProxyPassword)
	}

	// Check if recording is active
	recordings, err := fetchRecordings()
	var isActive bool
	if err == nil {
		for _, rec := range recordings {
			if rec.ID == recordingID && rec.IsActive && rec.Status == "recording" {
				isActive = true
				break
			}
		}
	}

	log.Printf("[DVR] Stream request for recording: %s (active: %v)", recordingID, isActive)

	var streams []StremioStream

	if isActive {
		// Active recording: offer Stop & Watch
		stopURL := fmt.Sprintf("%s/record/stop/%s?%s", easyProxyURL, recordingID, params.Encode())
		streams = append(streams, StremioStream{URL: stopURL, Title: "⏹️ Stop & Watch"})
	} else {
		// Completed recording: offer Play and Delete
		streamURL := fmt.Sprintf("%s/api/recordings/%s/stream?%s", easyProxyURL, recordingID, params.Encode())
		deleteURL := fmt.Sprintf("%s/api/recordings/%s/delete?%s", easyProxyURL, recordingID, params.Encode())
		streams = append(streams, StremioStream{URL: streamURL, Title: "▶️ Play Recording"})
		streams = append(streams, StremioStream{URL: deleteURL, Title: "🗑️ Delete Recording"})
	}

	jsonResponse(w, map[string][]StremioStream{"streams": streams})
}

// Handler: Homepage
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Get the host from the request to build the manifest URL
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	manifestURL := fmt.Sprintf("%s://%s/manifest.json", scheme, host)
	stremioURL := fmt.Sprintf("stremio://%s/manifest.json", host)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>DVR Recordings - Stremio Addon</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
        }
        .container {
            text-align: center;
            padding: 2rem;
            max-width: 500px;
        }
        .icon {
            font-size: 4rem;
            margin-bottom: 1rem;
        }
        h1 {
            font-size: 2rem;
            margin-bottom: 0.5rem;
            font-weight: 600;
        }
        .subtitle {
            color: #8892b0;
            margin-bottom: 2rem;
            font-size: 1.1rem;
        }
        .install-btn {
            display: inline-block;
            background: #7b2cbf;
            color: #fff;
            padding: 1rem 2.5rem;
            border-radius: 50px;
            text-decoration: none;
            font-size: 1.1rem;
            font-weight: 500;
            transition: all 0.3s ease;
            box-shadow: 0 4px 15px rgba(123, 44, 191, 0.4);
        }
        .install-btn:hover {
            background: #9d4edd;
            transform: translateY(-2px);
            box-shadow: 0 6px 20px rgba(123, 44, 191, 0.5);
        }
        .manual {
            margin-top: 2rem;
            padding-top: 1.5rem;
            border-top: 1px solid #2a2a4a;
        }
        .manual p {
            color: #8892b0;
            font-size: 0.9rem;
            margin-bottom: 0.5rem;
        }
        .manifest-url {
            background: #0d1117;
            padding: 0.75rem 1rem;
            border-radius: 8px;
            font-family: monospace;
            font-size: 0.85rem;
            color: #58a6ff;
            word-break: break-all;
            cursor: pointer;
            transition: background 0.2s;
        }
        .manifest-url:hover {
            background: #161b22;
        }
        .features {
            display: flex;
            justify-content: center;
            gap: 2rem;
            margin: 2rem 0;
            flex-wrap: wrap;
        }
        .feature {
            color: #8892b0;
            font-size: 0.9rem;
        }
        .feature span {
            display: block;
            font-size: 1.5rem;
            margin-bottom: 0.25rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">📼</div>
        <h1>DVR Recordings</h1>
        <p class="subtitle">Access your EasyProxy DVR recordings in Stremio</p>

        <div class="features">
            <div class="feature"><span>📺</span>Browse</div>
            <div class="feature"><span>🔍</span>Search</div>
            <div class="feature"><span>▶️</span>Play</div>
        </div>

        <a href="%s" class="install-btn">Install Addon</a>

        <div class="manual">
            <p>Or copy the manifest URL:</p>
            <div class="manifest-url" onclick="navigator.clipboard.writeText('%s')">%s</div>
        </div>
    </div>
</body>
</html>`, stremioURL, manifestURL, manifestURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/manifest.json", corsMiddleware(handleManifest))
	http.HandleFunc("/catalog/", corsMiddleware(handleCatalog))
	http.HandleFunc("/meta/", corsMiddleware(handleMeta))
	http.HandleFunc("/stream/", corsMiddleware(handleStream))

	log.Printf("[DVR] Stremio DVR addon running at http://localhost:%s", port)
	log.Printf("[DVR] EasyProxy URL: %s", easyProxyURL)
	log.Printf("[DVR] Install addon: http://localhost:%s/manifest.json", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
