package provider

import (
	"context"
	"errors"
	"testing"
)

func TestMockProviderSearchAndMetadata(t *testing.T) {
	p := NewMockProvider()
	ctx := context.Background()
	account, err := p.Login(ctx, "mock", nil)
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != "mock" {
		t.Fatalf("account=%q", account.ID)
	}
	videos, err := p.Search(ctx, Query{Text: "quiet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) == 0 {
		t.Fatal("mock search returned nothing")
	}
	v, err := p.GetVideo(ctx, videos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != videos[0].ID {
		t.Fatalf("searched %q, fetched %q", videos[0].ID, v.ID)
	}
	channels, err := p.SyncSubscriptions(ctx, account.ID)
	if err != nil || len(channels) == 0 {
		t.Fatalf("subscriptions: %v %d", err, len(channels))
	}
	if _, err := p.GetVideo(ctx, "not-here"); !errors.As(err, new(ErrNotFound)) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestYouTubeProviderDoesNotClaimAuthentication(t *testing.T) {
	p := NewYouTubeProvider(YouTubeConfig{})
	ctx := context.Background()
	if _, err := p.Login(ctx, "oauth", LoginPayload{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("login should be unauthenticated, got %v", err)
	}
	if _, err := p.SyncSubscriptions(ctx, "acct"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("sync should be unauthenticated, got %v", err)
	}
	if err := p.Download(ctx, DownloadRequest{VideoID: "x"}, func(int64, int64) {}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("download should be unauthenticated, got %v", err)
	}
	if p.Name() != "youtube" {
		t.Fatalf("name=%q", p.Name())
	}
}
