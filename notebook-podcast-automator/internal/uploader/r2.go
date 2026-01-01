package uploader

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Uploader 封装 Cloudflare R2 上传逻辑
type R2Uploader struct {
	Client     *s3.Client
	BucketName string
	PublicURL  string
}

// NewR2Uploader 初始化 R2 客户端
func NewR2Uploader() (*R2Uploader, error) {
	accountID := os.Getenv("R2_ACCOUNT_ID")
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	bucketName := os.Getenv("R2_BUCKET_NAME")
	publicURL := os.Getenv("R2_PUBLIC_URL")

	if accountID == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("R2 配置不完整，请检查 R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY")
	}

	// R2 的 S3 API Endpoint 格式: https://<account_id>.r2.cloudflarestorage.com
	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	// 自定义 Resolver 以支持 R2
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: r2Endpoint,
		}, nil
	})

	// 加载配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("auto"), // R2 区域通常填 auto
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 R2 配置: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	return &R2Uploader{
		Client:     client,
		BucketName: bucketName,
		PublicURL:  publicURL,
	}, nil
}

// UploadFile 上传文件并返回公开访问链接
func (u *R2Uploader) UploadFile(ctx context.Context, localFilePath string, objectKey string) (string, error) {
	file, err := os.Open(localFilePath)
	if err != nil {
		return "", fmt.Errorf("无法打开本地文件: %w", err)
	}
	defer file.Close()

	// 根据文件扩展名设置 Content-Type
	contentType := ContentTypeForPath(localFilePath)

	fmt.Printf("   > 正在上传 %s 到 R2 (%s)...\n", filepath.Base(localFilePath), u.BucketName)

	_, err = u.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.BucketName),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("上传失败: %w", err)
	}

	// 拼接返回最终的公开 URL
	finalURL := publicURLForObjectKey(u.PublicURL, objectKey)
	return finalURL, nil
}

// UploadBytes 上传字节数据（用于 RSS XML 等）
func (u *R2Uploader) UploadBytes(ctx context.Context, data []byte, objectKey string, contentType string) (string, error) {
	fmt.Printf("   > 正在上传 %s 到 R2...\n", objectKey)

	_, err := u.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.BucketName),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("上传失败: %w", err)
	}

	finalURL := publicURLForObjectKey(u.PublicURL, objectKey)
	return finalURL, nil
}

// ContentTypeForPath 根据文件扩展名返回 MIME 类型
func ContentTypeForPath(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	case ".xml":
		return "application/xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

func publicURLForObjectKey(publicBase string, objectKey string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if base == "" {
		return key
	}
	if key == "" {
		return base
	}
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return base + "/" + strings.Join(parts, "/")
}
