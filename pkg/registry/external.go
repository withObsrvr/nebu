package registry

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ExternalProcessorDesc represents a description.yml from an external registry.
type ExternalProcessorDesc struct {
	Processor struct {
		Name        string   `yaml:"name"`
		Type        string   `yaml:"type"`
		Description string   `yaml:"description"`
		Version     string   `yaml:"version"`
		Language    string   `yaml:"language"`
		License     string   `yaml:"license"`
		Maintainers []string `yaml:"maintainers"`
	} `yaml:"processor"`
	Repo struct {
		GitHub string `yaml:"github"`
	} `yaml:"repo"`
	Install *InstallConfig `yaml:"install"`
	Docs    struct {
		QuickStart          string `yaml:"quick_start"`
		Examples            string `yaml:"examples"`
		ExtendedDescription string `yaml:"extended_description"`
	} `yaml:"docs"`
}

// ToProcessorEntry converts an external description to a ProcessorEntry.
func (d *ExternalProcessorDesc) ToProcessorEntry() ProcessorEntry {
	entry := ProcessorEntry{
		Name:        d.Processor.Name,
		Type:        d.Processor.Type,
		Description: d.Processor.Description,
		Source:      "community",
		Location: ProcessorLocation{
			Type: "module",
		},
	}

	if d.Docs.ExtendedDescription != "" {
		entry.LongDescription = d.Docs.ExtendedDescription
	}

	if len(d.Processor.Maintainers) > 0 || d.Repo.GitHub != "" {
		entry.Maintainer = &MaintainerInfo{}
		if len(d.Processor.Maintainers) > 0 {
			entry.Maintainer.Name = strings.Join(d.Processor.Maintainers, ", ")
		}
		if d.Repo.GitHub != "" {
			entry.Maintainer.URL = "https://github.com/" + d.Repo.GitHub
		}
	}

	// Derive go install path from repo.github + processor name
	if d.Repo.GitHub != "" {
		entry.Location.ModulePackage = fmt.Sprintf(
			"github.com/%s/processors/%s/cmd/%s",
			d.Repo.GitHub, d.Processor.Name, d.Processor.Name,
		)
	}

	if d.Install != nil {
		inst := *d.Install
		if inst.Version == "" {
			inst.Version = d.Processor.Version
		}
		entry.Install = &inst
	}

	return entry
}

// ExternalRegistry fetches processor metadata from a GitHub-hosted registry.
type ExternalRegistry struct {
	URL      string        // e.g. "github.com/withObsrvr/nebu-processor-registry"
	CacheDir string        // e.g. ~/.cache/nebu/registries/<hash>/
	TTL      time.Duration // default 1 hour
	client   *http.Client
}

// cacheMeta tracks when the cache was last updated.
type cacheMeta struct {
	FetchedAt  time.Time `json:"fetched_at"`
	Processors []string  `json:"processors"`
}

// NewExternalRegistry creates an ExternalRegistry from a GitHub repo URL.
func NewExternalRegistry(repoURL string) (*ExternalRegistry, error) {
	cacheBase, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(repoURL)))[:12]
	cacheDir := filepath.Join(cacheBase, hash)

	return &ExternalRegistry{
		URL:      repoURL,
		CacheDir: cacheDir,
		TTL:      1 * time.Hour,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func defaultCacheDir() (string, error) {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "nebu", "registries"), nil
}

// FetchAll discovers and fetches all processor descriptions from the external registry.
// Returns processor entries, using cache when available and fresh.
func (r *ExternalRegistry) FetchAll() ([]ProcessorEntry, error) {
	// Check cache first
	if entries, err := r.loadFromCache(); err == nil {
		return entries, nil
	}

	// Parse owner/repo from URL
	owner, repo, err := r.parseOwnerRepo()
	if err != nil {
		return nil, err
	}

	// Discover processor directories via GitHub API
	names, err := r.discoverProcessors(owner, repo)
	if err != nil {
		// If fetch fails, try stale cache
		if entries, cacheErr := r.loadStaleCache(); cacheErr == nil {
			fmt.Fprintf(os.Stderr, "Warning: could not refresh external registry, using cached data: %v\n", err)
			return entries, nil
		}
		return nil, fmt.Errorf("failed to discover processors from %s: %w", r.URL, err)
	}

	// Fetch each description.yml
	var entries []ProcessorEntry
	for _, name := range names {
		desc, err := r.fetchDescription(owner, repo, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch description for %s: %v\n", name, err)
			continue
		}
		entry := desc.ToProcessorEntry()
		entries = append(entries, entry)
	}

	// Write cache (uses entry.Name for filenames and meta consistency)
	if err := r.writeCache(entries); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write cache: %v\n", err)
	}

	return entries, nil
}

func (r *ExternalRegistry) parseOwnerRepo() (string, string, error) {
	// Handle "github.com/owner/repo" format
	url := r.URL
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "github.com/")
	url = strings.TrimSuffix(url, "/")

	parts := strings.SplitN(url, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid registry URL: %s (expected github.com/owner/repo)", r.URL)
	}
	return parts[0], parts[1], nil
}

// githubContent represents a single entry from the GitHub contents API.
type githubContent struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (r *ExternalRegistry) discoverProcessors(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/processors", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "nebu-cli")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var contents []githubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	var names []string
	for _, c := range contents {
		if c.Type == "dir" && !strings.HasPrefix(c.Name, ".") {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

func (r *ExternalRegistry) fetchDescription(owner, repo, name string) (*ExternalProcessorDesc, error) {
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/main/processors/%s/description.yml",
		owner, repo, name,
	)

	resp, err := r.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got HTTP %d for %s", resp.StatusCode, name)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var desc ExternalProcessorDesc
	if err := yaml.Unmarshal(body, &desc); err != nil {
		return nil, fmt.Errorf("failed to parse description.yml for %s: %w", name, err)
	}

	return &desc, nil
}

// loadFromCache loads processors from cache if it exists and is within TTL.
func (r *ExternalRegistry) loadFromCache() ([]ProcessorEntry, error) {
	meta, err := r.readCacheMeta()
	if err != nil {
		return nil, err
	}

	if time.Since(meta.FetchedAt) > r.TTL {
		return nil, fmt.Errorf("cache expired")
	}

	return r.readCachedEntries(meta.Processors)
}

// loadStaleCache loads processors from cache regardless of TTL.
func (r *ExternalRegistry) loadStaleCache() ([]ProcessorEntry, error) {
	meta, err := r.readCacheMeta()
	if err != nil {
		return nil, err
	}
	return r.readCachedEntries(meta.Processors)
}

func (r *ExternalRegistry) readCacheMeta() (*cacheMeta, error) {
	data, err := os.ReadFile(filepath.Join(r.CacheDir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta cacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (r *ExternalRegistry) readCachedEntries(names []string) ([]ProcessorEntry, error) {
	var entries []ProcessorEntry
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(r.CacheDir, "processors", name+".yml"))
		if err != nil {
			continue
		}
		var desc ExternalProcessorDesc
		if err := yaml.Unmarshal(data, &desc); err != nil {
			continue
		}
		entries = append(entries, desc.ToProcessorEntry())
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no cached entries found")
	}
	return entries, nil
}

func (r *ExternalRegistry) writeCache(entries []ProcessorEntry) error {
	procDir := filepath.Join(r.CacheDir, "processors")
	if err := os.MkdirAll(procDir, 0o755); err != nil {
		return err
	}

	var names []string
	for _, entry := range entries {
		desc := ExternalProcessorDesc{}
		desc.Processor.Name = entry.Name
		desc.Processor.Type = entry.Type
		desc.Processor.Description = entry.Description
		desc.Docs.ExtendedDescription = entry.LongDescription
		// entry.Install already has Version resolved; persisting it keeps
		// binary installs working from cache without a registry refetch.
		desc.Install = entry.Install

		if entry.Maintainer != nil {
			desc.Processor.Maintainers = []string{entry.Maintainer.Name}
			if entry.Maintainer.URL != "" {
				desc.Repo.GitHub = strings.TrimPrefix(entry.Maintainer.URL, "https://github.com/")
			}
		}

		data, err := yaml.Marshal(&desc)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(procDir, entry.Name+".yml"), data, 0o644); err != nil {
			continue
		}
		names = append(names, entry.Name)
	}

	meta := cacheMeta{
		FetchedAt:  time.Now(),
		Processors: names,
	}
	metaData, err := json.Marshal(&meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.CacheDir, "meta.json"), metaData, 0o644)
}
