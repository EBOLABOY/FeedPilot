package uploader

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ================= XML 结构定义 =================

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Itunes  string   `xml:"xmlns:itunes,attr"`
	Content string   `xml:"xmlns:content,attr,omitempty"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title         string      `xml:"title"`
	Link          string      `xml:"link"`
	Description   string      `xml:"description"`
	Language      string      `xml:"language"`
	LastBuildDate string      `xml:"lastBuildDate,omitempty"`
	ItunesAuthor  string      `xml:"itunes:author"`
	ItunesImage   ItunesImage `xml:"itunes:image"`
	ItunesOwner   ItunesOwner `xml:"itunes:owner"`
	Items         []Item      `xml:"item"`
}

type ItunesImage struct {
	Href string `xml:"href,attr"`
}

type ItunesOwner struct {
	Name  string `xml:"itunes:name"`
	Email string `xml:"itunes:email"`
}

type Item struct {
	Title          string    `xml:"title"`
	Description    string    `xml:"description"`
	PubDate        string    `xml:"pubDate"`
	Guid           string    `xml:"guid"`
	Enclosure      Enclosure `xml:"enclosure"`
	ItunesDuration string    `xml:"itunes:duration,omitempty"`
	ItunesExplicit string    `xml:"itunes:explicit,omitempty"`
}

type Enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// ================= 核心逻辑 =================

// UpdateRSS 下载 feed.xml，追加新条目，然后重新上传
func (u *R2Uploader) UpdateRSS(ctx context.Context, rssFilename string, newItem Item) error {
	fmt.Println("   > 正在更新 RSS Feed...")

	// 1. 尝试下载现有的 feed.xml
	var rss RSS
	existingBytes, err := u.downloadBytes(ctx, rssFilename)

	if err != nil {
		// 文件不存在，创建新的 RSS
		fmt.Println("   > 未找到现有 RSS，将创建新文件...")
		rss = u.createNewRSS()
	} else {
		// 解析现有的 XML
		if err := xml.Unmarshal(existingBytes, &rss); err != nil {
			return fmt.Errorf("XML 解析失败: %w", err)
		}
		fmt.Printf("   > 找到现有 RSS，包含 %d 期节目\n", len(rss.Channel.Items))
	}

	// 2. 插入新条目到切片的最前面 (LIFO - 最新一期在最上)
	rss.Channel.Items = append([]Item{newItem}, rss.Channel.Items...)

	// 更新 LastBuildDate
	rss.Channel.LastBuildDate = time.Now().Format(time.RFC1123Z)

	// 3. 序列化回 XML
	output, err := xml.MarshalIndent(rss, "", "  ")
	if err != nil {
		return fmt.Errorf("XML 序列化失败: %w", err)
	}
	// 添加 XML 头
	finalXML := []byte(xml.Header + string(output))

	// 4. 上传回 R2
	_, err = u.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.BucketName),
		Key:         aws.String(rssFilename),
		Body:        bytes.NewReader(finalXML),
		ContentType: aws.String("application/xml; charset=utf-8"),
	})

	if err != nil {
		return fmt.Errorf("RSS 上传失败: %w", err)
	}

	fmt.Printf("   > RSS 更新成功，现在共 %d 期节目\n", len(rss.Channel.Items))
	return nil
}

// downloadBytes 下载文件字节流
func (u *R2Uploader) downloadBytes(ctx context.Context, key string) ([]byte, error) {
	resp, err := u.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// createNewRSS 初始化默认 RSS 结构
func (u *R2Uploader) createNewRSS() RSS {
	// 从环境变量读取配置，提供默认值
	title := getEnvOrDefault("PODCAST_TITLE", "WeChat Daily Podcast")
	description := getEnvOrDefault("PODCAST_DESCRIPTION", "自动生成的微信公众号文章播客摘要")
	author := getEnvOrDefault("PODCAST_AUTHOR", "AI Assistant")
	ownerName := getEnvOrDefault("PODCAST_OWNER_NAME", "Admin")
	ownerEmail := getEnvOrDefault("PODCAST_OWNER_EMAIL", "admin@example.com")

	return RSS{
		Version: "2.0",
		Itunes:  "http://www.itunes.com/dtds/podcast-1.0.dtd",
		Channel: Channel{
			Title:         title,
			Link:          u.PublicURL,
			Description:   description,
			Language:      "zh-cn",
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			ItunesAuthor:  author,
			ItunesOwner: ItunesOwner{
				Name:  ownerName,
				Email: ownerEmail,
			},
			ItunesImage: ItunesImage{
				Href: u.PublicURL + "/cover.jpg",
			},
			Items: []Item{},
		},
	}
}

// getEnvOrDefault 获取环境变量，如果为空则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
