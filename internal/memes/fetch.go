package memes

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultImageMaxBytes = 8 << 20
	imageReadMaxBytes    = 12 << 20
)

var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// Image is a downloaded meme image.
type Image struct {
	Data      []byte
	MediaType string
	Ext       string
}

// ImageFetcher downloads meme images from approved image hosts.
type ImageFetcher struct {
	httpClient *http.Client
	userAgent  string
	maxBytes   int64
	allowURL   func(string) bool
}

// NewImageFetcher creates an image downloader.
func NewImageFetcher(userAgent string, maxMB int) *ImageFetcher {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		userAgent = DefaultRedditUserAgent
	}
	maxBytes := int64(maxMB) << 20
	if maxBytes <= 0 {
		maxBytes = defaultImageMaxBytes
	}
	return &ImageFetcher{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many image redirects")
				}
				if !isMemeImageURL(req.URL.String()) {
					return errors.New("image redirect host is not allowed")
				}
				return nil
			},
		},
		userAgent: userAgent,
		maxBytes:  maxBytes,
	}
}

// Fetch downloads one image.
func (f *ImageFetcher) Fetch(ctx context.Context, rawURL string) (Image, error) {
	if f == nil {
		return Image{}, errors.New("image fetcher is disabled")
	}
	allowURL := f.allowURL
	if allowURL == nil {
		allowURL = isMemeImageURL
	}
	if !allowURL(rawURL) {
		return Image{}, fmt.Errorf("image host is not allowed: %s", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Image{}, fmt.Errorf("building image request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return Image{}, fmt.Errorf("fetching image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Image{}, fmt.Errorf("image server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > f.maxBytes {
		return Image{}, fmt.Errorf("image is larger than %d bytes", f.maxBytes)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return Image{}, fmt.Errorf("reading image: %w", err)
	}
	if int64(len(data)) > f.maxBytes {
		return Image{}, fmt.Errorf("image is larger than %d bytes", f.maxBytes)
	}
	if len(data) == 0 {
		return Image{}, errors.New("image is empty")
	}

	mediaType := http.DetectContentType(data)
	if _, ok := allowedImageTypes[mediaType]; !ok && mediaType == "application/octet-stream" {
		headerType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
		if _, ok := allowedImageTypes[headerType]; ok {
			mediaType = headerType
		}
	}
	ext, ok := allowedImageTypes[mediaType]
	if !ok {
		return Image{}, fmt.Errorf("unsupported image type: %s", mediaType)
	}

	return Image{Data: data, MediaType: mediaType, Ext: ext}, nil
}

// DataURL returns the image as a base64 data URL for the vision model.
func (i Image) DataURL() string {
	if len(i.Data) == 0 || i.MediaType == "" {
		return ""
	}
	return "data:" + i.MediaType + ";base64," + base64.StdEncoding.EncodeToString(i.Data)
}

// SaveImage writes an image into dir and returns its base file name.
func SaveImage(dir, id string, image Image) (string, error) {
	if !validPostID.MatchString(strings.TrimSpace(id)) {
		return "", fmt.Errorf("invalid meme id: %q", id)
	}
	if _, ok := allowedImageTypes[image.MediaType]; !ok {
		return "", fmt.Errorf("unsupported image type: %s", image.MediaType)
	}
	if len(image.Data) == 0 {
		return "", errors.New("image is empty")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("cannot create meme image directory: %w", err)
	}

	name := id + image.Ext
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".meme-*")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary image: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return "", fmt.Errorf("cannot set image permissions: %w", err)
	}
	if _, err := tmp.Write(image.Data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("cannot write image: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("cannot close image: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("cannot save image: %w", err)
	}
	return name, nil
}

// LoadImageFile reads a stored meme image and reports its media type.
func LoadImageFile(dir, name string) ([]byte, string, error) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return nil, "", errors.New("meme image name is empty")
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("cannot stat meme image: %w", err)
	}
	if info.Size() > imageReadMaxBytes {
		return nil, "", fmt.Errorf("meme image is larger than %d bytes", imageReadMaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read meme image: %w", err)
	}
	mediaType := http.DetectContentType(data)
	if _, ok := allowedImageTypes[mediaType]; !ok {
		mediaType = "application/octet-stream"
	}
	return data, mediaType, nil
}

// DeleteImage removes one stored meme image.
func DeleteImage(dir, name string) error {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return nil
	}
	err := os.Remove(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
