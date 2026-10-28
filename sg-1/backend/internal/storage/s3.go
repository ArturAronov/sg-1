package storage

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3 struct {
	client *s3.Client
	bucket string
}

func New(ctx context.Context, bucket, endpoint, region, accessKey, secret string) (*S3, error) {
	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if endpoint != "" {
		options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secret, "")))
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(config, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})
	return &S3{client: client, bucket: bucket}, nil
}

func (s *S3) Initialize(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket}); err != nil {
		if _, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket}); err != nil {
			return err
		}
	}
	// Warm and validate real object I/O, not just bucket metadata. Local stores
	// may allocate their first data volume asynchronously on the first write.
	const key = "__sg1_platform_health/ready"
	if err := s.Put(ctx, key, []byte("ready")); err != nil {
		return err
	}
	body, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(body, 1024))
	body.Close()
	if err != nil {
		return err
	}
	if string(data) != "ready" {
		return errors.New("object storage integrity check failed")
	}
	return s.Delete(ctx, key)
}
func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(data), ContentType: aws.String("application/octet-stream")})
	return err
}
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}
func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}
