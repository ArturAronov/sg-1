package platform

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	"io"
	"strings"
	"testing"
)

func TestBuildRolePropagationRetry(t *testing.T) {
	calls := 0
	err := retryBuildRolePropagation(context.Background(), func(context.Context) error {
		calls++
		if calls == 1 {
			return &smithy.GenericAPIError{Code: "InvalidInputException", Message: "CodeBuild is not authorized to perform: sts:AssumeRole"}
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatal("new IAM role was not retried", calls, err)
	}
	invalid := &smithy.GenericAPIError{Code: "InvalidInputException", Message: "invalid project name"}
	calls = 0
	err = retryBuildRolePropagation(context.Background(), func(context.Context) error { calls++; return invalid })
	if err != invalid || calls != 1 {
		t.Fatal("non-propagation failure was retried", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err = retryBuildRolePropagation(ctx, func(context.Context) error {
		cancel()
		return &smithy.GenericAPIError{Code: "InvalidInputException", Message: "sts:AssumeRole unavailable"}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("retry ignored cancellation", err)
	}
}

func TestIsolatedBuildArchiveAndRDSCertificate(t *testing.T) {
	files := map[string]string{"Dockerfile": "FROM node:22-alpine\nUSER 10001\nCMD [\"node\",\"server.js\"]", "server.js": "// source", ".dockerignore": "*.pem"}
	archive, err := buildArchive(files)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, file := range reader.File {
		stream, e := file.Open()
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(stream)
		stream.Close()
		if e != nil {
			t.Fatal(e)
		}
		contents[file.Name] = string(data)
	}
	if !strings.Contains(contents["sg1-rds-ca.pem"], "BEGIN CERTIFICATE") || !strings.Contains(contents["Dockerfile"], "COPY --chmod=0444 sg1-rds-ca.pem /etc/ssl/certs/sg1-rds-ca.pem") {
		t.Fatal("trusted RDS certificate was not injected")
	}
	if !strings.HasSuffix(contents[".dockerignore"], "!sg1-rds-ca.pem\n") {
		t.Fatal("untrusted dockerignore can exclude the trusted CA")
	}
	if _, ok := files["sg1-rds-ca.pem"]; ok {
		t.Fatal("source snapshot was mutated")
	}
}

func TestFargateTaskDoesNotExposeControlCredentials(t *testing.T) {
	a := &AWSRuntime{ExecutionRole: "execution-role", InitImage: "trusted-init@sha256:digest", LogGroup: "app-logs", Region: "us-east-1"}
	definition := a.taskDefinition("candidate", "project-image@sha256:digest", "arn:aws:secretsmanager:region:account:secret:project-db")
	if definition.TaskRoleArn != nil || len(definition.ContainerDefinitions) != 2 {
		t.Fatal("unexpected task role or containers")
	}
	app := definition.ContainerDefinitions[1]
	if aws.ToString(app.User) != "10001" || !aws.ToBool(app.ReadonlyRootFilesystem) || len(app.Secrets) != 1 || aws.ToString(app.Secrets[0].Name) != "DATABASE_URL" {
		t.Fatal("application isolation contract is missing")
	}
	if len(app.Environment) != 1 || aws.ToString(app.Environment[0].Name) != "PORT" {
		t.Fatal("control credentials entered app environment")
	}
	if len(app.DependsOn) != 1 || app.DependsOn[0].Condition != "SUCCESS" {
		t.Fatal("writable temporary volume is not prepared before app startup")
	}
}
func TestCloudRuntimeRefusesUntrustedTargets(t *testing.T) {
	a := &AWSRuntime{Account: "123456789012", Region: "us-east-1", Prefix: "sg1-prod"}
	if err := a.Start(context.Background(), "attacker.example/image:latest", "metadata", "project", "secret"); err == nil {
		t.Fatal("untrusted workload accepted")
	}
	id := "00000000-0000-4000-8000-000000000001"
	if err := a.Start(context.Background(), "attacker.example/image:latest", "sg1-app-"+id+"-12345678", id, "secret"); err == nil {
		t.Fatal("non-project ECR image accepted")
	}
	if _, err := a.Resolve(context.Background(), id, "169.254.169.254"); err == nil {
		t.Fatal("metadata proxy target accepted")
	}
}
