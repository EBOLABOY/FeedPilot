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
