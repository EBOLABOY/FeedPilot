package uploader

import (
	"strings"
	"testing"
)

func TestMarshalRSS_EnsuresItunesNamespace(t *testing.T) {
	rss := RSS{
		Version: "",
		Itunes:  "",
		Channel: Channel{
			Title:        "t",
			Link:         "https://example.com",
			Description:  "d",
			Language:     "zh-cn",
			ItunesAuthor: "a",
			ItunesImage:  ItunesImage{Href: "https://example.com/cover.jpg"},
			ItunesOwner:  ItunesOwner{Name: "n", Email: "e"},
		},
	}

	xmlBytes, err := marshalRSS(rss)
	if err != nil {
		t.Fatalf("marshalRSS error: %v", err)
	}

	out := string(xmlBytes)
	if strings.Contains(out, `xmlns:itunes=""`) {
		t.Fatalf("expected non-empty itunes namespace, got: %q", out)
	}
	if !strings.Contains(out, `xmlns:itunes="`+itunesNamespaceURL+`"`) {
		t.Fatalf("expected itunes namespace %q, got: %q", itunesNamespaceURL, out)
	}
	if !strings.Contains(out, `<rss version="2.0"`) {
		t.Fatalf("expected rss version=2.0, got: %q", out)
	}
}

func TestRetargetRSSPublicBase_RewritesEnclosureHost(t *testing.T) {
	rss := RSS{
		Version: "2.0",
		Itunes:  itunesNamespaceURL,
		Channel: Channel{
			Title:        "t",
			Link:         "https://old.example.com",
			Description:  "d",
			Language:     "zh-cn",
			ItunesAuthor: "a",
			ItunesImage:  ItunesImage{Href: "https://old.example.com/cover.jpg"},
			ItunesOwner:  ItunesOwner{Name: "n", Email: "e"},
			Items: []Item{
				{
					Title:       "ep1",
					Description: "x",
					PubDate:     "Thu, 01 Jan 2026 00:00:00 +0800",
					Guid:        "https://old.example.com/episodes/ep1.wav",
					Enclosure: Enclosure{
						URL:    "https://old.example.com/episodes/ep1.wav",
						Length: 1,
						Type:   "audio/wav",
					},
				},
			},
		},
	}

	retargetRSSPublicBase(&rss, "https://mypodcast.de5.net/")

	if rss.Channel.Link != "https://mypodcast.de5.net" {
		t.Fatalf("expected channel link rewritten, got %q", rss.Channel.Link)
	}
	if rss.Channel.ItunesImage.Href != "https://mypodcast.de5.net/cover.jpg" {
		t.Fatalf("expected itunes image rewritten, got %q", rss.Channel.ItunesImage.Href)
	}
	if rss.Channel.Items[0].Enclosure.URL != "https://mypodcast.de5.net/episodes/ep1.wav" {
		t.Fatalf("expected enclosure url rewritten, got %q", rss.Channel.Items[0].Enclosure.URL)
	}
	// GUID 保持不变，避免订阅客户端把历史节目当作“新节目”重复订阅。
	if rss.Channel.Items[0].Guid != "https://old.example.com/episodes/ep1.wav" {
		t.Fatalf("expected guid unchanged, got %q", rss.Channel.Items[0].Guid)
	}
}

func TestNormalizeRSS_EnsuresContentNamespace(t *testing.T) {
	rss := RSS{
		Version: "",
		Itunes:  "",
		Content: "",
		Channel: Channel{
			Title:       "t",
			Link:        "https://example.com",
			Description: "d",
			Language:    "zh-cn",
		},
	}

	xmlBytes, err := marshalRSS(rss)
	if err != nil {
		t.Fatalf("marshalRSS error: %v", err)
	}

	out := string(xmlBytes)
	if !strings.Contains(out, `xmlns:content="`+contentNamespaceURL+`"`) {
		t.Fatalf("expected content namespace %q, got: %q", contentNamespaceURL, out)
	}
}
