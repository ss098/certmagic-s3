package certmagic_s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/certmagic"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

var (
	// implementing these interfaces
	_ caddy.Module          = (*S3)(nil)
	_ certmagic.Storage     = (*S3)(nil)
	_ certmagic.Locker      = (*S3)(nil)
	_ caddy.Provisioner     = (*S3)(nil)
	_ caddyfile.Unmarshaler = (*S3)(nil)
)

func init() {
	caddy.RegisterModule(&S3{})
}

type S3 struct {
	logger *zap.Logger
	client *minio.Client

	// S3 configuration
	Host           string `json:"host"`
	Bucket         string `json:"bucket"`
	AccessID       string `json:"access_id"`
	SecretKey      string `json:"secret_key"`
	Prefix         string `json:"prefix,omitempty"`
	Insecure       bool   `json:"insecure"`
	UseIamProvider bool   `json:"use_iam_provider"`
}

func (s3 *S3) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {

		key := d.Val()

		var value string
		if !d.Args(&value) {
			continue
		}

		switch key {
		case "host":
			s3.Host = value
			err := validateHost(s3.Host)
			if err != nil {
				return d.Err("Invalid usage of host in s3-storage config: " + err.Error())
			}
		case "bucket":
			s3.Bucket = value
		case "access_id":
			s3.AccessID = value
		case "secret_key":
			s3.SecretKey = value
		case "prefix":
			s3.Prefix = value
		case "insecure":
			insecure, err := strconv.ParseBool(value)
			if err != nil {
				return d.Err("Invalid usage of insecure in s3-storage config: " + err.Error())
			}
			s3.Insecure = insecure
		case "use_iam_provider":
			boolValue, err := strconv.ParseBool(value)
			if err != nil {
				return d.Err("Invalid usage of use_iam_provider in s3-storage config: " + err.Error())
			}
			s3.UseIamProvider = boolValue
		}

	}

	return nil
}

func (s3 *S3) Provision(ctx caddy.Context) error {
	s3.logger = ctx.Logger(s3)

	// Load Environment
	if s3.Host == "" {
		s3.Host = os.Getenv("S3_HOST")
	}

	err := validateHost(s3.Host)
	if err != nil {
		return err
	}

	if !s3.UseIamProvider {
		boolVal := os.Getenv("S3_USE_IAM_PROVIDER")
		if boolVal != "" {
			s3.UseIamProvider, err = strconv.ParseBool(boolVal)

			if err != nil {
				s3.UseIamProvider = false // default value
			}
		}
	}

	if s3.Bucket == "" {
		s3.Bucket = os.Getenv("S3_BUCKET")
		if s3.Bucket == "" {
			return errors.New("bucket is empty")
		}
	}

	if s3.AccessID == "" {
		s3.AccessID = os.Getenv("S3_ACCESS_ID")
		if s3.AccessID == "" && !s3.UseIamProvider {
			return errors.New("access_id is empty and use_iam_provider is false")
		}
	}

	if s3.SecretKey == "" {
		s3.SecretKey = os.Getenv("S3_SECRET_KEY")
		if s3.SecretKey == "" && !s3.UseIamProvider {
			return errors.New("secret_key is empty and use_iam_provider is false")
		}
	}

	if s3.Prefix == "" {
		s3.Prefix = os.Getenv("S3_PREFIX")
	}

	if !s3.Insecure {
		insecure := os.Getenv("S3_INSECURE")
		if insecure != "" {
			s3.Insecure, err = strconv.ParseBool(insecure)

			if err != nil {
				s3.Insecure = false // default value
			}
		}
	}
	secure := !s3.Insecure

	var creds *credentials.Credentials
	if s3.UseIamProvider {
		s3.logger.Info("using iam aws provider for credentials")
		creds = credentials.NewIAM("")
	} else {
		s3.logger.Info("using secret_key and access_id for credentials")
		creds = credentials.NewStaticV4(s3.AccessID, s3.SecretKey, "")
	}

	// S3 Client
	client, err := minio.New(s3.Host, &minio.Options{
		Creds:  creds,
		Secure: secure,
	})
	if err != nil {
		return err
	}

	s3.client = client
	return nil
}

func (*S3) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "caddy.storage.s3",
		New: func() caddy.Module {
			return &S3{}
		},
	}
}

func (s3 *S3) CertMagicStorage() (certmagic.Storage, error) {
	return s3, nil
}

func (s3 *S3) Lock(ctx context.Context, key string) error {
	return nil
}

func (s3 *S3) Unlock(ctx context.Context, key string) error {
	return nil
}

func (s3 *S3) Store(ctx context.Context, key string, value []byte) error {
	key = s3.KeyPrefix(key)
	length := int64(len(value))

	s3.logger.Debug(fmt.Sprintf("Store: %s, %d bytes", key, length))

	_, err := s3.client.PutObject(ctx, s3.Bucket, key, bytes.NewReader(value), length, minio.PutObjectOptions{})

	return err
}

func (s3 *S3) Load(ctx context.Context, key string) ([]byte, error) {
	if !s3.Exists(ctx, key) {
		return nil, fs.ErrNotExist
	}

	key = s3.KeyPrefix(key)

	s3.logger.Debug(fmt.Sprintf("Load key: %s", key))

	object, err := s3.client.GetObject(ctx, s3.Bucket, key, minio.GetObjectOptions{})

	if err != nil {
		return nil, err
	}

	defer object.Close()
	
	return io.ReadAll(object)
}

func (s3 *S3) Delete(ctx context.Context, key string) error {
	key = s3.KeyPrefix(key)

	s3.logger.Debug(fmt.Sprintf("Delete key: %s", key))

	return s3.client.RemoveObject(ctx, s3.Bucket, key, minio.RemoveObjectOptions{})
}

func (s3 *S3) Exists(ctx context.Context, key string) bool {
	key = s3.KeyPrefix(key)

	_, err := s3.client.StatObject(ctx, s3.Bucket, key, minio.StatObjectOptions{})

	exists := err == nil

	s3.logger.Debug(fmt.Sprintf("Check exists: %s, %t", key, exists))

	return exists
}

func (s3 *S3) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {

	objects := s3.client.ListObjects(ctx, s3.Bucket, minio.ListObjectsOptions{
		Prefix:    s3.KeyPrefix(prefix),
		Recursive: recursive,
	})

	keys := make([]string, len(objects))

	for object := range objects {
		keys = append(keys, s3.CutKeyPrefix(object.Key))
	}

	return keys, nil
}

func (s3 *S3) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	key = s3.KeyPrefix(key)

	object, err := s3.client.StatObject(ctx, s3.Bucket, key, minio.StatObjectOptions{})

	if err != nil {
		s3.logger.Error(fmt.Sprintf("Stat key: %s, error: %v", key, err))

		return certmagic.KeyInfo{}, nil
	}

	s3.logger.Debug(fmt.Sprintf("Stat key: %s, size: %d bytes", key, object.Size))

	return certmagic.KeyInfo{
		Key:        object.Key,
		Modified:   object.LastModified,
		Size:       object.Size,
		IsTerminal: strings.HasSuffix(object.Key, "/"),
	}, err
}

func (s3 *S3) KeyPrefix(key string) string {
	return path.Join(s3.Prefix, key)
}
func (s3 *S3) CutKeyPrefix(key string) string {
	cutted, _ := strings.CutPrefix(key, s3.Prefix)
	return cutted
}

func (s3 *S3) String() string {
	return fmt.Sprintf("S3 Storage Host: %s, Bucket: %s, Prefix: %s", s3.Host, s3.Bucket, s3.Prefix)
}

func validateHost(h string) error {
	u, err := url.Parse(h)
	if err != nil {
		return fmt.Errorf("invalid host: must be a hostname: %w", err)
	}
	if u.Scheme != "" {
		return errors.New("host must not contain a scheme prefix like https://")
	}
	return nil
}
