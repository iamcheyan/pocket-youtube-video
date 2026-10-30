package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/domain"
)

// YouTubeConfig controls yt-dlp invocation. Binary defaults to yt-dlp;
// no cookies or account credentials are used by this provider.
type YouTubeConfig struct {
	Binary     string
	MaxResults int
}
type YouTubeProvider struct{ config YouTubeConfig }

func NewYouTubeProvider(c YouTubeConfig) *YouTubeProvider {
	if c.Binary == "" {
		c.Binary = "yt-dlp"
	}
	if c.MaxResults <= 0 {
		c.MaxResults = 20
	}
	return &YouTubeProvider{config: c}
}
func (*YouTubeProvider) Name() string { return "youtube" }
func (*YouTubeProvider) Login(ctx context.Context, kind string, payload LoginPayload) (domain.Account, error) {
	return domain.Account{}, ErrUnauthenticated
}
func (*YouTubeProvider) Logout(ctx context.Context, id string) error { return nil }
func (*YouTubeProvider) SyncSubscriptions(ctx context.Context, id string) ([]domain.Channel, error) {
	return nil, ErrUnauthenticated
}
func (*YouTubeProvider) SyncPlaylists(ctx context.Context, id string) ([]domain.Playlist, error) {
	return nil, ErrUnauthenticated
}
func (*YouTubeProvider) SyncWatchLater(ctx context.Context, id string) ([]domain.Video, error) {
	return nil, ErrUnauthenticated
}
func (p *YouTubeProvider) Search(ctx context.Context, q Query) ([]domain.Video, error) {
	if strings.TrimSpace(q.Text) == "" {
		return []domain.Video{}, nil
	}
	limit := q.Limit
	if limit <= 0 || limit > p.config.MaxResults {
		limit = p.config.MaxResults
	}
	args := []string{"--dump-single-json", "--flat-playlist", "--no-warnings", fmt.Sprintf("ytsearch%d:%s", limit, q.Text)}
	out, err := exec.CommandContext(ctx, p.config.Binary, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp YouTube search: %w", err)
	}
	var result struct {
		Entries []struct {
			ID, Title, URL, Thumbnail string
			Duration                  float64
			Description               string
			UploadDate                string
			Channel                   string
			ChannelID                 string
		} `json:"entries"`
	}
	if err = json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("decode yt-dlp search result: %w", err)
	}
	videos := make([]domain.Video, 0, len(result.Entries))
	for _, e := range result.Entries {
		id := e.ID
		if id == "" {
			continue
		}
		u := e.URL
		if !strings.HasPrefix(u, "http") {
			u = "https://www.youtube.com/watch?v=" + id
		}
		d := domain.Video{ID: id, Title: e.Title, URL: u, Thumbnail: e.Thumbnail, Duration: int(e.Duration), Description: e.Description}
		if e.UploadDate != "" {
			d.PublishedAt, _ = time.Parse("20060102", e.UploadDate)
		}
		videos = append(videos, d)
	}
	return videos, nil
}
func (p *YouTubeProvider) GetVideo(ctx context.Context, id string) (domain.Video, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Video{}, ErrNotFound{Kind: "video", ID: id}
	}
	out, err := exec.CommandContext(ctx, p.config.Binary, "--dump-single-json", "--no-warnings", "https://www.youtube.com/watch?v="+id).Output()
	if err != nil {
		return domain.Video{}, fmt.Errorf("yt-dlp YouTube metadata: %w", err)
	}
	var e struct {
		ID, Title, URL, Thumbnail string
		Duration                  float64
		Description               string
		UploadDate                string
		Channel                   string
		ChannelID                 string
	}
	if err = json.Unmarshal(out, &e); err != nil {
		return domain.Video{}, err
	}
	if e.ID == "" {
		return domain.Video{}, ErrNotFound{Kind: "video", ID: id}
	}
	if e.URL == "" {
		e.URL = "https://www.youtube.com/watch?v=" + e.ID
	}
	v := domain.Video{ID: e.ID, Title: e.Title, URL: e.URL, Thumbnail: e.Thumbnail, Duration: int(e.Duration), Description: e.Description, ChannelID: e.ChannelID}
	if e.UploadDate != "" {
		v.PublishedAt, _ = time.Parse("20060102", e.UploadDate)
	}
	return v, nil
}
func (p *YouTubeProvider) GetChannel(ctx context.Context, id string) (domain.Channel, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Channel{}, ErrNotFound{Kind: "channel", ID: id}
	}
	return domain.Channel{}, ErrUnauthenticated
}
func (p *YouTubeProvider) Download(ctx context.Context, r DownloadRequest, progress Progress) error {
	return ErrUnauthenticated
}

var _ VideoProvider = (*YouTubeProvider)(nil)
