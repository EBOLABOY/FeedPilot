package workflow

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

type FeedItem struct {
	Title string
	URL   string
	ID    string
}

type FeedKind string

const (
	FeedKindUnknown FeedKind = "unknown"
	FeedKindAtom    FeedKind = "atom"
	FeedKindRSS     FeedKind = "rss"
)

func ParseFeed(data []byte) (title string, items []FeedItem, kind FeedKind, err error) {
	title, items, err = parseAtom(data)
	if err == nil && len(items) > 0 {
		return title, items, FeedKindAtom, nil
	}

	title, items, err = parseRSS(data)
	if err == nil && len(items) > 0 {
		return title, items, FeedKindRSS, nil
	}

	if err != nil {
		return "", nil, FeedKindUnknown, err
	}
	return "", nil, FeedKindUnknown, nil
}

func parseAtom(data []byte) (feedTitle string, items []FeedItem, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))

	var inEntry bool
	var cur FeedItem

	for {
		tok, tokErr := dec.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return "", nil, tokErr
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "entry":
				inEntry = true
				cur = FeedItem{}
			case "title":
				var v string
				if decodeErr := dec.DecodeElement(&v, &t); decodeErr != nil {
					continue
				}
				v = strings.TrimSpace(v)
				if inEntry {
					if cur.Title == "" {
						cur.Title = v
					}
				} else if feedTitle == "" {
					feedTitle = v
				}
			case "link":
				if !inEntry {
					continue
				}
				var href string
				var rel string
				for _, a := range t.Attr {
					switch strings.ToLower(a.Name.Local) {
					case "href":
						href = strings.TrimSpace(a.Value)
					case "rel":
						rel = strings.TrimSpace(a.Value)
					}
				}
				if href == "" {
					continue
				}
				if cur.URL == "" || rel == "alternate" {
					cur.URL = href
				}
			case "id":
				var v string
				if decodeErr := dec.DecodeElement(&v, &t); decodeErr != nil {
					continue
				}
				v = strings.TrimSpace(v)
				if inEntry {
					if cur.ID == "" {
						cur.ID = v
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "entry" && inEntry {
				inEntry = false
				if strings.TrimSpace(cur.URL) != "" {
					items = append(items, cur)
				}
			}
		}
	}

	return feedTitle, items, nil
}

func parseRSS(data []byte) (channelTitle string, items []FeedItem, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))

	var inItem bool
	var cur FeedItem

	for {
		tok, tokErr := dec.Token()
		if tokErr == io.EOF {
			break
		}
		if tokErr != nil {
			return "", nil, tokErr
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "item":
				inItem = true
				cur = FeedItem{}
			case "title":
				var v string
				if decodeErr := dec.DecodeElement(&v, &t); decodeErr != nil {
					continue
				}
				v = strings.TrimSpace(v)
				if inItem {
					if cur.Title == "" {
						cur.Title = v
					}
				} else if channelTitle == "" {
					channelTitle = v
				}
			case "link":
				if !inItem {
					continue
				}
				var v string
				if decodeErr := dec.DecodeElement(&v, &t); decodeErr != nil {
					continue
				}
				v = strings.TrimSpace(v)
				if v != "" && cur.URL == "" {
					cur.URL = v
				}
			case "guid":
				if !inItem {
					continue
				}
				var v string
				if decodeErr := dec.DecodeElement(&v, &t); decodeErr != nil {
					continue
				}
				v = strings.TrimSpace(v)
				if v != "" && cur.ID == "" {
					cur.ID = v
				}
			}
		case xml.EndElement:
			if t.Name.Local == "item" && inItem {
				inItem = false
				if strings.TrimSpace(cur.URL) != "" {
					items = append(items, cur)
				}
			}
		}
	}

	return channelTitle, items, nil
}
