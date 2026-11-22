// Package memes scrapes programming memes and keeps them as chat memory.
package memes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
)

const (
	// DefaultRedditBase is the listing host used for scraping.
	DefaultRedditBase = "https://old.reddit.com"
	// DefaultRedditUserAgent identifies the bot as a browser. Reddit rejects
	// most non-browser clients with a block page.
	DefaultRedditUserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"
	// DefaultRedditMinGap serializes reddit requests.
	DefaultRedditMinGap = 2 * time.Second

	redditMaxListingBytes = 5 << 20
)

// ErrRedditSession means Reddit refused the request because there is no
// usable signed-in session.
var ErrRedditSession = errors.New(
	"reddit needs a session cookie: set REDDIT_SESSION to the reddit_session value from a logged-in browser",
)

var (
	validSubreddit = regexp.MustCompile(`^[a-z0-9_]{2,21}$`)
	validPostID    = regexp.MustCompile(`^[A-Za-z0-9]+$`)
)

// Post is one image post from a subreddit top listing.
type Post struct {
	ID        string
	Subreddit string
	Title     string
	Author    string
	Permalink string
	ImageURL  string
	Score     int
	NSFW      bool
	CreatedAt time.Time
}

// RedditConfig configures a RedditClient.
type RedditConfig struct {
	BaseURL     string
	Session     string
	UserAgent   string
	IncludeNSFW bool
	MinGap      time.Duration
	HTTPClient  *http.Client
}

// RedditClient reads subreddit top listings from old.reddit HTML.
type RedditClient struct {
	httpClient  *http.Client
	baseURL     string
	session     string
	userAgent   string
	includeNSFW bool
	minGap      time.Duration

	now  func() time.Time
	mu   sync.Mutex
	last time.Time
}

// NewRedditClient creates a Reddit listing client.
func NewRedditClient(cfg RedditConfig) *RedditClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultRedditBase
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = DefaultRedditUserAgent
	}
	minGap := cfg.MinGap
	if minGap < 0 {
		minGap = 0
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &RedditClient{
		httpClient:  client,
		baseURL:     baseURL,
		session:     strings.TrimSpace(cfg.Session),
		userAgent:   userAgent,
		includeNSFW: cfg.IncludeNSFW,
		minGap:      minGap,
		now:         time.Now,
	}
}

// Configured reports whether a session cookie is set.
func (c *RedditClient) Configured() bool {
	return c != nil && strings.TrimSpace(c.session) != ""
}

// Top returns image posts from a subreddit top listing.
func (c *RedditClient) Top(ctx context.Context, subreddit, period string, limit int) ([]Post, error) {
	if c == nil {
		return nil, errors.New("reddit client is disabled")
	}
	if !c.Configured() {
		return nil, ErrRedditSession
	}

	subreddit = normalizeSubreddit(subreddit)
	if !validSubreddit.MatchString(subreddit) {
		return nil, fmt.Errorf("invalid subreddit name: %q", subreddit)
	}
	period = normalizePeriod(period)
	if limit <= 0 {
		limit = 25
	}
	listingLimit := limit
	if listingLimit > 100 {
		listingLimit = 100
	}

	endpoint := fmt.Sprintf(
		"%s/r/%s/top/?t=%s&limit=%d",
		c.baseURL,
		url.PathEscape(subreddit),
		url.QueryEscape(period),
		listingLimit,
	)
	body, err := c.get(ctx, endpoint, "text/html,application/xhtml+xml", redditMaxListingBytes)
	if err != nil {
		return nil, err
	}

	posts, err := parseTopListing(body, limit, c.includeNSFW)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 && isRedditAgeGate(body) {
		return nil, errors.New("this subreddit is age restricted: set MEMES_INCLUDE_NSFW=true")
	}
	for i := range posts {
		posts[i].Subreddit = subreddit
	}
	return posts, nil
}

func isRedditAgeGate(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "over18-gate") ||
		strings.Contains(text, "<title>over 18</title>") ||
		strings.Contains(text, "are you over 18")
}

func (c *RedditClient) get(ctx context.Context, endpoint, accept string, maxBytes int64) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building reddit request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if cookie := c.cookie(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching reddit: %w", err)
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil &&
		strings.HasPrefix(strings.ToLower(resp.Request.URL.Path), "/login") {
		return nil, ErrRedditSession
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrRedditSession
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("reddit rate limited the request")
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("reddit returned HTTP 404, the subreddit may not exist")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("reddit returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading reddit response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("reddit response is larger than %d bytes", maxBytes)
	}
	if isRedditLoginWall(body) {
		return nil, ErrRedditSession
	}
	return body, nil
}

func (c *RedditClient) wait(ctx context.Context) error {
	if c.minGap <= 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.last.IsZero() {
		if wait := c.minGap - c.now().Sub(c.last); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.last = c.now()
	return nil
}

func (c *RedditClient) cookie() string {
	raw := strings.TrimSpace(c.session)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if !strings.Contains(lower, "reddit_session=") {
		raw = "reddit_session=" + raw
	}
	if c.includeNSFW && !strings.Contains(lower, "over18=") {
		raw += "; over18=1"
	}
	return raw
}

func normalizeSubreddit(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "reddit.com") ||
		strings.HasPrefix(lower, "www.reddit.com") || strings.HasPrefix(lower, "old.reddit.com") {
		raw := name
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		if parsed, err := url.Parse(raw); err == nil {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			for i, part := range parts {
				if strings.EqualFold(part, "r") && i+1 < len(parts) {
					name = parts[i+1]
					break
				}
			}
		}
	}

	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "r/")
	return strings.Trim(name, "/")
}

func normalizePeriod(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "hour", "day", "week", "month", "year", "all":
		return strings.ToLower(strings.TrimSpace(period))
	default:
		return "week"
	}
}

func isRedditLoginWall(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, `id="login-form"`) ||
		strings.Contains(text, `id="login"`) ||
		strings.Contains(text, "you must be logged in") ||
		strings.Contains(text, "whoa there, pardner") ||
		strings.Contains(text, "<title>welcome to reddit</title>") ||
		strings.Contains(text, "<title>blocked</title>")
}

// parseTopListing extracts image posts from an old.reddit top listing.
func parseTopListing(body []byte, limit int, includeNSFW bool) ([]Post, error) {
	doc, err := xhtml.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parsing reddit listing: %w", err)
	}

	if limit <= 0 {
		limit = 25
	}

	posts := make([]Post, 0, limit)
	seen := make(map[string]bool)
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "div" && hasClass(n, "thing") {
			if post, ok := listingPost(n, includeNSFW); ok {
				if !seen[post.ID] {
					seen[post.ID] = true
					posts = append(posts, post)
				}
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

func listingPost(n *xhtml.Node, includeNSFW bool) (Post, bool) {
	if strings.EqualFold(attrValue(n, "data-promoted"), "true") {
		return Post{}, false
	}

	fullname := attrValue(n, "data-fullname")
	if !strings.HasPrefix(fullname, "t3_") {
		return Post{}, false
	}
	id := strings.TrimPrefix(fullname, "t3_")
	if !validPostID.MatchString(id) {
		return Post{}, false
	}

	nsfw := strings.EqualFold(attrValue(n, "data-nsfw"), "true") || hasClass(n, "over18")
	if nsfw && !includeNSFW {
		return Post{}, false
	}

	title := ""
	if anchor := firstMatch(n, func(x *xhtml.Node) bool {
		return x.Type == xhtml.ElementNode && x.Data == "a" && hasClass(x, "title")
	}); anchor != nil {
		title = strings.TrimSpace(textContent(anchor))
	}
	if title == "" {
		return Post{}, false
	}

	destination := cleanURL(attrValue(n, "data-url"))
	imageURL := ""
	switch {
	case isMemeImageURL(destination):
		imageURL = destination
	case isGalleryURL(destination):
		imageURL = galleryCover(listingThumbnail(n))
	}
	if imageURL == "" {
		return Post{}, false
	}

	permalink := permalinkURL(attrValue(n, "data-permalink"))
	if permalink == "" {
		permalink = "https://www.reddit.com/comments/" + id
	}

	return Post{
		ID:        id,
		Title:     title,
		Author:    strings.TrimSpace(attrValue(n, "data-author")),
		Permalink: permalink,
		ImageURL:  imageURL,
		Score:     listingScore(n),
		NSFW:      nsfw,
		CreatedAt: listingCreatedAt(n),
	}, true
}

func listingThumbnail(n *xhtml.Node) string {
	raw := ""
	if anchor := firstMatch(n, func(x *xhtml.Node) bool {
		return x.Type == xhtml.ElementNode && x.Data == "a" && hasClass(x, "thumbnail")
	}); anchor != nil {
		if img := firstMatch(anchor, func(x *xhtml.Node) bool {
			return x.Type == xhtml.ElementNode && x.Data == "img"
		}); img != nil {
			raw = attrValue(img, "src")
		}
	}
	if raw == "" {
		raw = attrValue(n, "data-thumbnail")
	}
	return cleanURL(raw)
}

func listingScore(n *xhtml.Node) int {
	score := firstMatch(n, func(x *xhtml.Node) bool {
		return (x.Type == xhtml.ElementNode && (x.Data == "div" || x.Data == "span")) && hasClass(x, "score")
	})
	if score == nil {
		return 0
	}

	raw := strings.TrimSpace(attrValue(score, "title"))
	if raw == "" {
		raw = strings.TrimSpace(textContent(score))
	}
	raw = strings.ReplaceAll(raw, ",", "")
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func listingCreatedAt(n *xhtml.Node) time.Time {
	if tag := firstMatch(n, func(x *xhtml.Node) bool {
		return x.Type == xhtml.ElementNode && x.Data == "time"
	}); tag != nil {
		if raw := strings.TrimSpace(attrValue(tag, "datetime")); raw != "" {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				return parsed.UTC()
			}
		}
	}

	raw := strings.TrimSpace(attrValue(n, "data-timestamp"))
	if raw == "" {
		return time.Time{}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func isMemeImageURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "i.redd.it", "preview.redd.it", "i.imgur.com":
	default:
		return false
	}
	switch strings.ToLower(pathExt(u.Path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func isGalleryURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	if !isRedditHost(u.Hostname()) {
		return false
	}
	return strings.Contains(strings.ToLower(u.Path), "/gallery/")
}

func galleryCover(thumbnail string) string {
	u, err := url.Parse(strings.TrimSpace(thumbnail))
	if err != nil || !strings.EqualFold(u.Hostname(), "preview.redd.it") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 1 {
		return ""
	}
	name := parts[0]
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return ""
	}
	return "https://i.redd.it/" + name
}

func permalinkURL(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		if !strings.Contains(u.Path, "/comments/") {
			return ""
		}
		return "https://www.reddit.com" + u.Path
	}
	if !isRedditHost(u.Hostname()) || !strings.Contains(u.Path, "/comments/") {
		return ""
	}
	return "https://www.reddit.com" + u.Path
}

func cleanURL(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

func pathExt(path string) string {
	if dot := strings.LastIndex(path, "."); dot >= 0 {
		return path[dot:]
	}
	return ""
}

func isRedditHost(host string) bool {
	switch strings.ToLower(host) {
	case "reddit.com", "www.reddit.com", "old.reddit.com", "np.reddit.com", "m.reddit.com":
		return true
	default:
		return false
	}
}

func hasClass(n *xhtml.Node, want string) bool {
	for _, attr := range n.Attr {
		if !strings.EqualFold(attr.Key, "class") {
			continue
		}
		for _, class := range strings.Fields(attr.Val) {
			if class == want {
				return true
			}
		}
	}
	return false
}

func attrValue(n *xhtml.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func firstMatch(n *xhtml.Node, pred func(*xhtml.Node) bool) *xhtml.Node {
	if pred(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := firstMatch(child, pred); found != nil {
			return found
		}
	}
	return nil
}

func textContent(n *xhtml.Node) string {
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(x *xhtml.Node) {
		if x.Type == xhtml.TextNode {
			builder.WriteString(x.Data)
		}
		for child := x.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return builder.String()
}
