package storage

import (
	"context"
	"fmt"
	"io"
	"liotom/whateverserver/internal/config"
	"log"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

type Part struct {
	PartNumber int32  `json:"partNumber"`
	ETag       string `json:"etag"`
}

func New(ctx context.Context, cfg config.Config) (*Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	mk := func(endpoint string) *s3.Client {
		return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		})
	}
	st := &Storage{
		client:  mk(cfg.S3Endpoint),
		presign: s3.NewPresignClient(mk(cfg.S3PublicEndpoint)),
		bucket:  cfg.S3Bucket,
	}
	_, _ = st.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &cfg.S3Bucket})
	return st, nil
}

// SetCORS lets browsers from origins PUT multipart parts directly and read
// the ETag header back. Only the API calls this: it owns ALLOWED_ORIGINS,
// and a second writer (the worker has no origins configured) would replace
// the rule with its defaults depending on which process started last.
func (s *Storage) SetCORS(ctx context.Context, origins []string) error {
	if len(origins) == 0 {
		return nil
	}
	_, err := s.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: &s.bucket,
		CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
			AllowedOrigins: origins,
			AllowedMethods: []string{"GET", "PUT"},
			AllowedHeaders: []string{"*"},
			ExposeHeaders:  []string{"ETag"},
			MaxAgeSeconds:  aws.Int32(3600),
		}}},
	})
	return err
}

func (s *Storage) CreateMultipart(ctx context.Context, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: &s.bucket, Key: &key, ContentType: &contentType,
	})
	if err != nil {
		return "", err
	}
	return *out.UploadId, nil
}

func (s *Storage) PresignPart(ctx context.Context, key, uploadID string, n int32) (string, error) {
	req, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket: &s.bucket, Key: &key, UploadId: &uploadID, PartNumber: &n,
	}, s3.WithPresignExpires(2*time.Hour))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// PresignDownload returns a browser-reachable URL that downloads key as filename.
func (s *Storage) PresignDownload(ctx context.Context, key, filename string) (string, error) {
	// ASCII fallback plus the RFC 5987 form for non-ASCII names
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, filename)
	disp := fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		ascii, strings.ReplaceAll(url.QueryEscape(filename), "+", "%20"))
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket, Key: &key, ResponseContentDisposition: &disp,
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// DeleteMedia removes everything stored for a media row: the original, any
// unfinished upload of it, and the transcoded output (which can exist even
// before hls_key is set, e.g. after a failed process). Best effort: errors
// are logged, the DB row is already gone.
func (s *Storage) DeleteMedia(ctx context.Context, mediaID, rawKey string) {
	for _, err := range []error{
		s.AbortMultipartUploads(ctx, "raw/"+mediaID+"/"),
		s.DeleteKey(ctx, rawKey),
		s.DeletePrefix(ctx, "hls/"+mediaID),
	} {
		if err != nil {
			log.Printf("delete media %s: %v", mediaID, err)
		}
	}
}

// AbortMultipartUploads cancels unfinished multipart uploads under prefix,
// releasing the parts already stored for them.
func (s *Storage) AbortMultipartUploads(ctx context.Context, prefix string) error {
	out, err := s.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
		Bucket: &s.bucket, Prefix: &prefix,
	})
	if err != nil {
		return err
	}
	for _, u := range out.Uploads {
		if _, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket: &s.bucket, Key: u.Key, UploadId: u.UploadId,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) CompleteMultipart(ctx context.Context, key, uploadID string, parts []Part) error {
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	cp := make([]types.CompletedPart, len(parts))
	for i, p := range parts {
		cp[i] = types.CompletedPart{PartNumber: aws.Int32(p.PartNumber), ETag: aws.String(p.ETag)}
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: &s.bucket, Key: &key, UploadId: &uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: cp},
	})
	return err
}

func (s *Storage) Get(ctx context.Context, key, rangeHeader string) (*s3.GetObjectOutput, error) {
	in := &s3.GetObjectInput{Bucket: &s.bucket, Key: &key}
	if rangeHeader != "" {
		in.Range = &rangeHeader
	}
	return s.client.GetObject(ctx, in)
}

func (s *Storage) Download(ctx context.Context, key, dst string) error {
	out, err := s.Get(ctx, key, "")
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, out.Body)
	return err
}

func (s *Storage) UploadFile(ctx context.Context, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	ct := ContentType(path)
	up := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = 64 << 20
		u.Concurrency = 4
	})
	_, err = up.Upload(ctx, &s3.PutObjectInput{
		Bucket: &s.bucket, Key: &key, Body: f, ContentType: &ct,
	})
	return err
}

func (s *Storage) UploadDir(ctx context.Context, prefix, dir string) error {
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		return s.UploadFile(ctx, prefix+"/"+filepath.ToSlash(rel), p)
	})
}

func (s *Storage) DeleteKey(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket, Key: &key,
	})
	return err
}

func (s *Storage) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		return nil
	}
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: &s.bucket, Prefix: aws.String(prefix + "/"),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		objs := make([]types.ObjectIdentifier, len(page.Contents))
		for i, o := range page.Contents {
			objs[i] = types.ObjectIdentifier{Key: o.Key}
		}
		_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &s.bucket,
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func ContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".vtt":
		return "text/vtt"
	case ".ass", ".ssa":
		return "text/x-ssa"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".ttf", ".otf":
		return "font/sfnt"
	}
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}
