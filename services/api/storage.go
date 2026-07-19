package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const maxAssetBytes int64 = 25 << 20

var safeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type ObjectStore struct {
	client *s3.Client
	signer *s3.PresignClient
	bucket string
}
type Asset struct {
	ID              string `json:"id"`
	FileName        string `json:"fileName"`
	ContentType     string `json:"contentType"`
	Size            int64  `json:"size"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Status          string `json:"status"`
	URL             string `json:"url,omitempty"`
	RejectionReason string `json:"rejectionReason,omitempty"`
}
type AssetRequest struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

func newObjectStore() *ObjectStore {
	creds := credentials.NewStaticCredentialsProvider(env("S3_ACCESS_KEY", "printstudio"), env("S3_SECRET_KEY", "printstudio-secret"), "")
	makeClient := func(endpoint string) *s3.Client {
		return s3.New(s3.Options{Region: env("S3_REGION", "us-east-1"), Credentials: creds, BaseEndpoint: aws.String(endpoint), UsePathStyle: true})
	}
	internal := makeClient(env("S3_ENDPOINT", "http://localhost:9000"))
	public := makeClient(env("S3_PUBLIC_ENDPOINT", "http://localhost:9000"))
	return &ObjectStore{client: internal, signer: s3.NewPresignClient(public), bucket: env("S3_BUCKET", "printstudio-assets")}
}
func (s *ObjectStore) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
	if err != nil {
		if _, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket}); err != nil {
			return err
		}
	}
	_, err = s.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: &s.bucket,
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{{
				AllowedHeaders: []string{"*"},
				AllowedMethods: []string{"GET", "PUT", "HEAD"},
				AllowedOrigins: []string{env("WEB_ORIGIN", "http://localhost:3000")},
				ExposeHeaders:  []string{"ETag"},
				MaxAgeSeconds:  aws.Int32(3600),
			}},
		},
	})
	if err != nil {
		// Some local MinIO builds return 501 for PutBucketCors; uploads can still work via signed URLs.
		var apiErr interface{ ErrorCode() string }
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotImplemented" {
			return nil
		}
		if strings.Contains(err.Error(), "NotImplemented") {
			return nil
		}
	}
	return err
}
func (s *ObjectStore) uploadURL(ctx context.Context, key, contentType string, size int64) (string, error) {
	out, err := s.signer.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, ContentType: &contentType, ContentLength: aws.Int64(size)}, func(o *s3.PresignOptions) { o.Expires = 15 * time.Minute })
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
func (s *ObjectStore) downloadURL(ctx context.Context, key string) (string, error) {
	out, err := s.signer.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key}, func(o *s3.PresignOptions) { o.Expires = time.Hour })
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
func (s *ObjectStore) delete(ctx context.Context, key string) {
	_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
}
func (s *ObjectStore) open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}
func (s *ObjectStore) inspect(ctx context.Context, key, expectedType string, expectedSize int64, expectedSHA string) (int, int, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return 0, 0, errors.New("uploaded object not found")
	}
	defer out.Body.Close()
	if out.ContentLength == nil || *out.ContentLength != expectedSize {
		return 0, 0, errors.New("stored size differs from declared size")
	}
	if out.ContentType == nil || *out.ContentType != expectedType {
		return 0, 0, errors.New("stored content type differs from declared type")
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, maxAssetBytes+1))
	if err != nil || int64(len(data)) > maxAssetBytes {
		return 0, 0, errors.New("object exceeds size limit")
	}
	realType := httpDetect(data)
	if realType != expectedType {
		return 0, 0, fmt.Errorf("file signature is %s, not %s", realType, expectedType)
	}
	actualSHA := fmt.Sprintf("%x", sha256.Sum256(data))
	if !strings.EqualFold(actualSHA, expectedSHA) {
		return 0, 0, errors.New("file checksum differs from the upload request")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, errors.New("image cannot be decoded")
	}
	if cfg.Width < 32 || cfg.Height < 32 {
		return 0, 0, errors.New("image must be at least 32 × 32 pixels")
	}
	if cfg.Width > 16000 || cfg.Height > 16000 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return 0, 0, errors.New("image dimensions exceed safety limits")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return 0, 0, errors.New("image pixel data is corrupted")
	}
	return cfg.Width, cfg.Height, nil
}
func validateAssetRequest(v AssetRequest) error {
	if v.Size <= 0 || v.Size > maxAssetBytes {
		return fmt.Errorf("file size must be between 1 byte and %d MB", maxAssetBytes>>20)
	}
	if v.ContentType != "image/png" && v.ContentType != "image/jpeg" {
		return errors.New("only verified PNG and JPEG originals are accepted")
	}
	if len(v.SHA256) != 64 {
		return errors.New("a lowercase SHA-256 checksum is required")
	}
	for _, c := range v.SHA256 {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return errors.New("SHA-256 checksum is invalid")
		}
	}
	ext := strings.ToLower(filepath.Ext(v.FileName))
	if (v.ContentType == "image/png" && ext != ".png") || (v.ContentType == "image/jpeg" && ext != ".jpg" && ext != ".jpeg") {
		return errors.New("filename extension does not match content type")
	}
	return nil
}
func cleanFileName(v string) string {
	v = filepath.Base(strings.TrimSpace(v))
	v = safeName.ReplaceAllString(v, "-")
	if len(v) > 120 {
		v = v[len(v)-120:]
	}
	return v
}
