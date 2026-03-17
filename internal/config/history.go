package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// HistoryEntry represents a single recently opened file.
type HistoryEntry struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"last_opened"`
	Count      int       `json:"count"`
}

// QueryEntry represents a single search query.
type QueryEntry struct {
	Query    string    `json:"query"`
	LastUsed time.Time `json:"last_used"`
}

// History stores the list of recent files and search queries.
type History struct {
	RecentFiles  []HistoryEntry          `json:"recent_files"`
	QueryHistory map[string][]QueryEntry `json:"query_history"`
	path         string
}

// LoadHistory loads history from ~/.config/boomerang/history.json.
func LoadHistory() (*History, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "boomerang", "history.json")
	
	h := &History{
		path:         path,
		QueryHistory: make(map[string][]QueryEntry),
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return h, err
	}
	
	if err := json.Unmarshal(data, h); err != nil {
		return h, err
	}

	if h.QueryHistory == nil {
		h.QueryHistory = make(map[string][]QueryEntry)
	}
	
	return h, nil
}

// Save saves history to disk.
func (h *History) Save() error {
	if h.path == "" {
		return nil
	}
	
	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(h.path), 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(h.path, data, 0644)
}

// Add adds or updates a file in history.
func (h *History) Add(absPath string) {
	idx := -1
	for i, entry := range h.RecentFiles {
		if entry.Path == absPath {
			idx = i
			break
		}
	}
	
	if idx >= 0 {
		h.RecentFiles[idx].LastOpened = time.Now()
		h.RecentFiles[idx].Count++
	} else {
		h.RecentFiles = append(h.RecentFiles, HistoryEntry{
			Path:       absPath,
			LastOpened: time.Now(),
			Count:      1,
		})
	}
	
	// Sort by last opened descending.
	sort.Slice(h.RecentFiles, func(i, j int) bool {
		return h.RecentFiles[i].LastOpened.After(h.RecentFiles[j].LastOpened)
	})
	
	// Cap at 500 entries.
	if len(h.RecentFiles) > 500 {
		h.RecentFiles = h.RecentFiles[:500]
	}
}

// AddQuery adds or updates a search query for a specific tab.
func (h *History) AddQuery(tabName string, query string) {
	if query == "" {
		return
	}

	if h.QueryHistory == nil {
		h.QueryHistory = make(map[string][]QueryEntry)
	}

	queries := h.QueryHistory[tabName]
	idx := -1
	for i, q := range queries {
		if q.Query == query {
			idx = i
			break
		}
	}

	if idx >= 0 {
		queries[idx].LastUsed = time.Now()
	} else {
		queries = append(queries, QueryEntry{
			Query:    query,
			LastUsed: time.Now(),
		})
	}

	// Sort by last used descending.
	sort.Slice(queries, func(i, j int) bool {
		return queries[i].LastUsed.After(queries[j].LastUsed)
	})

	// Cap at 50 entries.
	if len(queries) > 50 {
		queries = queries[:50]
	}

	h.QueryHistory[tabName] = queries
}

// GetQueries returns the list of queries for a tab.
func (h *History) GetQueries(tabName string) []string {
	if h.QueryHistory == nil {
		return nil
	}
	entries := h.QueryHistory[tabName]
	res := make([]string, len(entries))
	for i, e := range entries {
		res[i] = e.Query
	}
	return res
}

// Remove removes a file from history.
func (h *History) Remove(absPath string) {
	for i, entry := range h.RecentFiles {
		if entry.Path == absPath {
			h.RecentFiles = append(h.RecentFiles[:i], h.RecentFiles[i+1:]...)
			break
		}
	}
}

// GetBoost returns a score boost for a given file path.
func (h *History) GetBoost(absPath string) int {
	for _, entry := range h.RecentFiles {
		if entry.Path == absPath {
			// Boost based on recency and count.
			// Max boost ~350.
			boost := 200 // base recency boost
			boost += entry.Count * 10
			if boost > 350 {
				boost = 350
			}
			return boost
		}
	}
	return 0
}
