package certmagic_s3

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
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

type S3 struct {
	logger *zap.Logger

	// S3
	Client         *minio.Client
	Host           string `json:"host"`
	Bucket         string `json:"bucket"`
	AccessID       string `json:"access_id"`
	SecretKey      string `json:"secret_key"`
	Prefix         string `json:"prefix"`
	Insecure       bool   `json:"insecure"`
	UseIamProvider bool   `json:"use_iam_provider"`
}

func init() {
	caddy.RegisterModule(S3{})
}

func (s3 *S3) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		var value string

		key := d.Val()

		if !d.Args(&value) {
			continue
		}

		switch key {
		case "host":
			s3.Host = value
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

	if s3.Bucket == "" {
		s3.Bucket = os.Getenv("S3_BUCKET")
	}

	if s3.AccessID == "" {
		s3.AccessID = os.Getenv("S3_ACCESS_ID")
	}

	if s3.SecretKey == "" {
		s3.SecretKey = os.Getenv("S3_SECRET_KEY")
	}

	if s3.Prefix == "" {
		s3.Prefix = os.Getenv("S3_PREFIX")
	}

	if !s3.Insecure {
		insecure := os.Getenv("S3_INSECURE")
		if insecure != "" {
			s3.Insecure, _ = strconv.ParseBool(insecure)
		}
	}
	secure := !s3.Insecure

	if !s3.UseIamProvider {
		boolVal := os.Getenv("S3_USE_IAM_PROVIDER")
		if boolVal != "" {
			s3.UseIamProvider, _ = strconv.ParseBool(boolVal)
		}
	}

	var creds *credentials.Credentials
	if s3.UseIamProvider {
		s3.logger.Info("use iam aws provider for credentials")
		creds = credentials.NewIAM("")
	} else {
		s3.logger.Info("use secret_key and access_id for credentials")
		creds = credentials.NewStaticV4(s3.AccessID, s3.SecretKey, "")
	}

	// S3 Client
	client, err := minio.New(s3.Host, &minio.Options{
		Creds:  creds,
		Secure: secure,
	})

	if err != nil {
		return err
	} else {
		s3.Client = client
	}

	return nil
}

func (S3) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "caddy.storage.s3",
		New: func() caddy.Module {
			return new(S3)
		},
	}
}

func (s3 S3) CertMagicStorage() (certmagic.Storage, error) {
	return s3, nil
}

func (s3 S3) Lock(ctx context.Context, key string) error {
	return nil
}

func (s3 S3) Unlock(ctx context.Context, key string) error {
	return nil
}

func (s3 S3) Store(ctx context.Context, key string, value []byte) error {
	key = s3.KeyPrefix(key)
	length := int64(len(value))

	s3.logger.Debug(fmt.Sprintf("Store: %s, %d bytes", key, length))

	_, err := s3.Client.PutObject(context.Background(), s3.Bucket, key, bytes.NewReader(value), length, minio.PutObjectOptions{})

	return err
}

func (s3 S3) Load(ctx context.Context, key string) ([]byte, error) {
	if !s3.Exists(ctx, key) {
		return nil, fs.ErrNotExist
	}

	key = s3.KeyPrefix(key)

	s3.logger.Debug(fmt.Sprintf("Load key: %s", key))

	object, err := s3.Client.GetObject(context.Background(), s3.Bucket, key, minio.GetObjectOptions{})

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(object)
}

func (s3 S3) Delete(ctx context.Context, key string) error {
	key = s3.KeyPrefix(key)

	s3.logger.Debug(fmt.Sprintf("Delete key: %s", key))

	return s3.Client.RemoveObject(context.Background(), s3.Bucket, key, minio.RemoveObjectOptions{})
}

func (s3 S3) Exists(ctx context.Context, key string) bool {
	key = s3.KeyPrefix(key)

	_, err := s3.Client.StatObject(context.Background(), s3.Bucket, key, minio.StatObjectOptions{})

	exists := err == nil

	s3.logger.Debug(fmt.Sprintf("Check exists: %s, %t", key, exists))

	return exists
}

func (s3 S3) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	objects := s3.Client.ListObjects(ctx, s3.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	})

	keys := make([]string, len(objects))

	for object := range objects {
		keys = append(keys, object.Key)
	}

	return keys, nil
}

func (s3 S3) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	key = s3.KeyPrefix(key)

	object, err := s3.Client.StatObject(context.Background(), s3.Bucket, key, minio.StatObjectOptions{})

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

func (s3 S3) KeyPrefix(key string) string {
	return path.Join(s3.Prefix, key)
}

func (s3 S3) String() string {
	return fmt.Sprintf("S3 Storage Host: %s, Bucket: %s, Prefix: %s", s3.Host, s3.Bucket, s3.Prefix)
}
