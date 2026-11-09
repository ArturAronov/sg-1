package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestAWSUploadsAreQuarantinedUntilClean(t *testing.T) {
	for _, status := range []string{"", "THREATS_FOUND", "UNSUPPORTED", "ACCESS_DENIED", "NO_THREATS_FOUND"} {
		t.Run(status, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("tagging") {
					w.Header().Set("Content-Type", "application/xml")
					_, _ = io.WriteString(w, `<Tagging xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><TagSet><Tag><Key>GuardDutyMalwareScanStatus</Key><Value>`+status+`</Value></Tag></TagSet></Tagging>`)
					return
				}
				reads++
				_, _ = io.WriteString(w, "private file")
			}))
			defer server.Close()
			client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: aws.AnonymousCredentials{}}, func(o *s3.Options) { o.BaseEndpoint = aws.String(server.URL); o.UsePathStyle = true })
			objects := &S3{client: client, bucket: "private", scanRequired: true}
			data, err := objects.Get(context.Background(), "uploads/project/attachments/file")
			if status != "NO_THREATS_FOUND" {
				if !errors.Is(err, ErrScanPending) || reads != 0 {
					t.Fatal("uncleared object reached the download path")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			body, _ := io.ReadAll(data)
			if string(body) != "private file" || reads != 1 {
				t.Fatal("clean object download failed")
			}
		})
	}
}
func TestUploadKeySeparatesQuarantinedObjects(t *testing.T) {
	if (&S3{scanRequired: true}).UploadKey("project", "file") != "uploads/project/attachments/file" {
		t.Fatal("AWS upload bypasses quarantine prefix")
	}
	if (&S3{}).UploadKey("project", "file") != "project/attachments/file" {
		t.Fatal("development file paths changed")
	}
}
