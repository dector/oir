// Package gh is a minimal GitHub Releases API client.
package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/dector/oir/internal/version"
)

// DefaultBaseURL is the public GitHub API endpoint.
const DefaultBaseURL = "https://api.github.com"

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	HTTP      *http.Client
}

// NewClient builds a client, picking up GITHUB_TOKEN or GH_TOKEN if present.
func NewClient() *Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	return &Client{
		BaseURL:   DefaultBaseURL,
		Token:     token,
		UserAgent: "oir/" + version.Version,
		HTTP:      &http.Client{},
	}
}

// Release is the subset of a GitHub release oir cares about.
type Release struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset is a single downloadable file attached to a release.
type Asset struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Size               int64     `json:"size"`
	Digest             string    `json:"digest"`
	BrowserDownloadURL string    `json:"browser_download_url"`
	CreatedAt          time.Time `json:"created_at"`
}

// LatestRelease returns the release GitHub marks as latest.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*Release, error) {
	var rel Release
	path := fmt.Sprintf("/repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.getJSON(ctx, path, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// ReleaseByTag returns a specific release by tag name.
func (c *Client) ReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error) {
	var rel Release
	path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s",
		url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag))
	if err := c.getJSON(ctx, path, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// Releases lists releases, newest first.
func (c *Client) Releases(ctx context.Context, owner, repo string, perPage int) ([]Release, error) {
	if perPage <= 0 {
		perPage = 30
	}

	var rels []Release
	path := fmt.Sprintf("/repos/%s/%s/releases?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), perPage)
	if err := c.getJSON(ctx, path, &rels); err != nil {
		return nil, err
	}

	return rels, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req, "application/vnd.github+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.apiError(resp)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// OpenAsset starts downloading an asset and returns the raw response body.
// The caller must close it.
func (c *Client) OpenAsset(ctx context.Context, assetURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, "application/octet-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	return resp, nil
}

func (c *Client) setHeaders(req *http.Request, accept string) {
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found: %s", trimBody(body))
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		reset := resp.Header.Get("X-RateLimit-Reset")
		hint := "set GITHUB_TOKEN to raise the limit"
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			hint = fmt.Sprintf("rate limit resets at %s; %s",
				time.Unix(ts, 0).Format(time.RFC3339), hint)
		}
		return fmt.Errorf("GitHub API rate limit exceeded: %s", hint)
	}

	msg := trimBody(body)
	var apiErr struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		msg = apiErr.Message
	}

	return fmt.Errorf("GitHub API error: %s: %s", resp.Status, msg)
}

func trimBody(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}
	return string(b)
}

// ErrNoReleases is returned when a repository has no usable releases.
var ErrNoReleases = errors.New("no releases found")
