package platform

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/storage"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cb "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	er "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ec "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	it "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/aws/smithy-go"
)

//go:embed rds-ca.pem
var rdsCA string

type AWSRuntime struct {
	Store                                                                                                            *store.Store
	Objects                                                                                                          *storage.S3
	Region, Account, Prefix, Bucket, Cluster, NamespaceID, Namespace, TaskSG, ExecutionRole, BuildBoundary, LogGroup string
	Subnets                                                                                                          []string
	ECR                                                                                                              *ecr.Client
	ECS                                                                                                              *ecs.Client
	Builds                                                                                                           *codebuild.Client
	IAM                                                                                                              *iam.Client
	Discovery                                                                                                        *servicediscovery.Client
	Secrets                                                                                                          *secretsmanager.Client
	InitImage                                                                                                        string
}

var projectIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var workloadPattern = regexp.MustCompile(`^sg1-app-([a-f0-9-]{36})-[a-f0-9]{8}(-live)?$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func NewAWSRuntime(ctx context.Context, repo *store.Store, objects *storage.S3) (*AWSRuntime, error) {
	a := &AWSRuntime{Store: repo, Objects: objects, Region: os.Getenv("AWS_REGION"), Account: os.Getenv("SG1_AWS_ACCOUNT_ID"), Prefix: os.Getenv("SG1_AWS_PREFIX"), Bucket: os.Getenv("SG1_S3_BUCKET"), Cluster: os.Getenv("SG1_ECS_CLUSTER_ARN"), NamespaceID: os.Getenv("SG1_DISCOVERY_NAMESPACE_ID"), Namespace: os.Getenv("SG1_DISCOVERY_NAMESPACE"), TaskSG: os.Getenv("SG1_TASK_SECURITY_GROUP"), ExecutionRole: os.Getenv("SG1_TASK_EXECUTION_ROLE_ARN"), BuildBoundary: os.Getenv("SG1_BUILD_BOUNDARY_ARN"), LogGroup: os.Getenv("SG1_APP_LOG_GROUP"), Subnets: strings.Split(os.Getenv("SG1_TASK_SUBNETS"), ",")}
	a.InitImage = os.Getenv("SG1_INIT_IMAGE")
	if objects == nil || !regexp.MustCompile(`^[a-z][a-z0-9-]{2,16}$`).MatchString(a.Prefix) || !regexp.MustCompile(`^[0-9]{12}$`).MatchString(a.Account) {
		return nil, errors.New("AWS runtime account/prefix/storage is not configured")
	}
	for _, v := range []string{a.Region, a.Bucket, a.Cluster, a.NamespaceID, a.Namespace, a.TaskSG, a.ExecutionRole, a.BuildBoundary, a.LogGroup, strings.Join(a.Subnets, "")} {
		if v == "" {
			return nil, errors.New("AWS runtime configuration is incomplete")
		}
	}
	initPrefix := a.registry() + "/" + a.Prefix + "-api@"
	if !strings.HasPrefix(a.InitImage, initPrefix) || !digestPattern.MatchString(strings.TrimPrefix(a.InitImage, initPrefix)) {
		return nil, errors.New("a digest-pinned trusted init image is required")
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(a.Region))
	if err != nil {
		return nil, err
	}
	a.ECR = ecr.NewFromConfig(config)
	a.ECS = ecs.NewFromConfig(config)
	a.Builds = codebuild.NewFromConfig(config)
	a.IAM = iam.NewFromConfig(config)
	a.Discovery = servicediscovery.NewFromConfig(config)
	a.Secrets = secretsmanager.NewFromConfig(config)
	return a, nil
}
func awsCode(err error) string {
	var e smithy.APIError
	if errors.As(err, &e) {
		return e.ErrorCode()
	}
	return ""
}
func (a *AWSRuntime) repository(id string) string   { return a.Prefix + "-project-" + id }
func (a *AWSRuntime) registry() string              { return a.Account + ".dkr.ecr." + a.Region + ".amazonaws.com" }
func (a *AWSRuntime) buildProject(id string) string { return a.Prefix + "-" + id }

func (a *AWSRuntime) PrepareNetwork(ctx context.Context, id string) (string, error) {
	if !projectIDPattern.MatchString(id) {
		return "", errors.New("invalid project ID")
	}
	repository := a.repository(id)
	_, err := a.ECR.CreateRepository(ctx, &ecr.CreateRepositoryInput{RepositoryName: aws.String(repository), ImageTagMutability: er.ImageTagMutabilityImmutable, ImageScanningConfiguration: &er.ImageScanningConfiguration{ScanOnPush: true}, Tags: []er.Tag{{Key: aws.String("sg1-project"), Value: aws.String(id)}}})
	if err != nil && awsCode(err) != "RepositoryAlreadyExistsException" {
		return "", errors.New("ECR repository could not be prepared")
	}
	roleName := a.Prefix + "-build-" + id
	trust, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Principal": map[string]string{"Service": "codebuild.amazonaws.com"}, "Action": "sts:AssumeRole", "Condition": map[string]any{"StringEquals": map[string]string{"aws:SourceAccount": a.Account}, "ArnEquals": map[string]string{"aws:SourceArn": "arn:aws:codebuild:" + a.Region + ":" + a.Account + ":project/" + a.buildProject(id)}}}}})
	_, err = a.IAM.CreateRole(ctx, &iam.CreateRoleInput{RoleName: aws.String(roleName), AssumeRolePolicyDocument: aws.String(string(trust)), PermissionsBoundary: aws.String(a.BuildBoundary), Tags: []it.Tag{{Key: aws.String("sg1-project"), Value: aws.String(id)}}})
	if err != nil && awsCode(err) != "EntityAlreadyExists" {
		return "", errors.New("isolated build role could not be prepared")
	}
	role, err := a.IAM.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
	if err != nil {
		return "", err
	}
	owned := false
	for _, tag := range role.Role.Tags {
		if aws.ToString(tag.Key) == "sg1-project" && aws.ToString(tag.Value) == id {
			owned = true
		}
	}
	if !owned || role.Role.PermissionsBoundary == nil || aws.ToString(role.Role.PermissionsBoundary.PermissionsBoundaryArn) != a.BuildBoundary {
		return "", errors.New("build role ownership/boundary mismatch")
	}
	repoARN := "arn:aws:ecr:" + a.Region + ":" + a.Account + ":repository/" + repository
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject"}, "Resource": "arn:aws:s3:::" + a.Bucket + "/builds/" + id + "/*"},
		map[string]any{"Effect": "Allow", "Action": []string{"ecr:GetAuthorizationToken"}, "Resource": "*"},
		map[string]any{"Effect": "Allow", "Action": []string{"ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage", "ecr:InitiateLayerUpload", "ecr:UploadLayerPart", "ecr:CompleteLayerUpload", "ecr:PutImage"}, "Resource": repoARN},
		map[string]any{"Effect": "Allow", "Action": []string{"logs:CreateLogStream", "logs:PutLogEvents"}, "Resource": "arn:aws:logs:" + a.Region + ":" + a.Account + ":log-group:/aws/codebuild/" + a.Prefix + ":*"},
	}})
	if _, err = a.IAM.PutRolePolicy(ctx, &iam.PutRolePolicyInput{RoleName: aws.String(roleName), PolicyName: aws.String("project-only"), PolicyDocument: aws.String(string(policy))}); err != nil {
		return "", err
	}
	err = retryBuildRolePropagation(ctx, func(ctx context.Context) error {
		_, err := a.Builds.CreateProject(ctx, &codebuild.CreateProjectInput{Name: aws.String(a.buildProject(id)), ServiceRole: role.Role.Arn, TimeoutInMinutes: aws.Int32(10), QueuedTimeoutInMinutes: aws.Int32(10), ConcurrentBuildLimit: aws.Int32(1), Source: &cb.ProjectSource{Type: cb.SourceTypeS3, Location: aws.String(a.Bucket + "/builds/" + id + "/source.zip")}, Artifacts: &cb.ProjectArtifacts{Type: cb.ArtifactsTypeNoArtifacts}, Environment: &cb.ProjectEnvironment{Type: cb.EnvironmentTypeLinuxContainer, ComputeType: cb.ComputeTypeBuildGeneral1Small, Image: aws.String("aws/codebuild/standard:7.0"), PrivilegedMode: aws.Bool(true)}, LogsConfig: &cb.LogsConfig{CloudWatchLogs: &cb.CloudWatchLogsConfig{Status: cb.LogsConfigStatusTypeEnabled, GroupName: aws.String("/aws/codebuild/" + a.Prefix)}}})
		return err
	})
	if err != nil && awsCode(err) != "ResourceAlreadyExistsException" {
		return "", errors.New("isolated CodeBuild project could not be prepared")
	}
	return id, nil
}

// A newly created IAM role may not yet be assumable by CodeBuild. Retry only
// that propagation failure, with a deadline independent of the build timeout.
func retryBuildRolePropagation(ctx context.Context, create func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := create(ctx)
		var apiError smithy.APIError
		if attempt >= 6 || !errors.As(err, &apiError) || apiError.ErrorCode() != "InvalidInputException" || !strings.Contains(strings.ToLower(apiError.ErrorMessage()), "sts:assumerole") {
			return err
		}
		if err := pause(ctx, time.Second*time.Duration(1<<min(attempt, 3))); err != nil {
			return err
		}
	}
}

func buildArchive(files map[string]string) ([]byte, error) {
	if err := ValidateFiles(files); err != nil {
		return nil, err
	}
	copyFiles := map[string]string{}
	for k, v := range files {
		copyFiles[k] = v
	}
	copyFiles["sg1-rds-ca.pem"] = rdsCA
	copyFiles["Dockerfile"] += "\nCOPY --chmod=0444 sg1-rds-ca.pem /etc/ssl/certs/sg1-rds-ca.pem\n"
	copyFiles[".dockerignore"] += "\n!sg1-rds-ca.pem\n"
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range copyFiles {
		file, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = file.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

const isolatedBuildspec = `version: 0.2
phases:
  pre_build:
    commands:
      - aws ecr get-login-password --region "$AWS_DEFAULT_REGION" | docker login --username AWS --password-stdin "$SG1_REGISTRY"
  build:
    commands:
      - docker build --pull -t "$SG1_IMAGE" .
      - docker push "$SG1_IMAGE"
`

func (a *AWSRuntime) Build(ctx context.Context, id, tag string, files map[string]string) (string, string, error) {
	if !projectIDPattern.MatchString(id) {
		return "", "", errors.New("invalid project ID")
	}
	archive, err := buildArchive(files)
	if err != nil {
		return "", "", err
	}
	raw, _ := json.Marshal(files)
	hash := sha256.Sum256(append([]byte(id+tag), raw...))
	imageTag := hex.EncodeToString(hash[:])[:40]
	image := a.registry() + "/" + a.repository(id) + ":" + imageTag
	key := "builds/" + id + "/" + imageTag + ".zip"
	if err = a.Objects.Put(ctx, key, archive); err != nil {
		return "", "", errors.New("isolated build source upload failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = a.Objects.Delete(cleanup, key)
	}()
	existing, _ := a.ECR.DescribeImages(ctx, &ecr.DescribeImagesInput{RepositoryName: aws.String(a.repository(id)), ImageIds: []er.ImageIdentifier{{ImageTag: aws.String(imageTag)}}})
	if existing == nil || len(existing.ImageDetails) == 0 {
		build, err := a.Builds.StartBuild(ctx, &codebuild.StartBuildInput{IdempotencyToken: aws.String(imageTag), ProjectName: aws.String(a.buildProject(id)), SourceTypeOverride: cb.SourceTypeS3, SourceLocationOverride: aws.String(a.Bucket + "/" + key), BuildspecOverride: aws.String(isolatedBuildspec), EnvironmentVariablesOverride: []cb.EnvironmentVariable{{Name: aws.String("SG1_IMAGE"), Value: aws.String(image), Type: cb.EnvironmentVariableTypePlaintext}, {Name: aws.String("SG1_REGISTRY"), Value: aws.String(a.registry()), Type: cb.EnvironmentVariableTypePlaintext}}})
		if err != nil {
			return "", "", errors.New("CodeBuild could not start")
		}
		buildID := aws.ToString(build.Build.Id)
		if _, err = a.Store.Pool.Exec(ctx, `INSERT INTO aws_builds(id,project_id) VALUES($1,$2)`, buildID, id); err != nil {
			a.cancelBuild(buildID)
			return "", "", errors.New("cloud build tracking failed")
		}
		complete := false
		defer func() {
			if !complete {
				a.cancelBuild(buildID)
			} else {
				cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, _ = a.Store.Pool.Exec(cleanup, `DELETE FROM aws_builds WHERE id=$1`, buildID)
			}
		}()
		for {
			if err = pause(ctx, 3*time.Second); err != nil {
				return "", "", err
			}
			status, e := a.Builds.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{Ids: []string{buildID}})
			if e != nil || len(status.Builds) != 1 {
				return "", "", errors.New("CodeBuild status unavailable")
			}
			if string(status.Builds[0].BuildStatus) != "IN_PROGRESS" {
				complete = true
			}
			switch string(status.Builds[0].BuildStatus) {
			case "IN_PROGRESS":
				continue
			case "SUCCEEDED":
			default:
				return "", "Isolated CodeBuild build failed; review its restricted CloudWatch log.", errors.New("CodeBuild validation failed")
			}
			break
		}
	}
	images, err := a.ECR.DescribeImages(ctx, &ecr.DescribeImagesInput{RepositoryName: aws.String(a.repository(id)), ImageIds: []er.ImageIdentifier{{ImageTag: aws.String(imageTag)}}})
	if err != nil || len(images.ImageDetails) != 1 {
		return "", "", errors.New("ECR build artifact is unavailable")
	}
	digest := aws.ToString(images.ImageDetails[0].ImageDigest)
	if !digestPattern.MatchString(digest) {
		return "", "", errors.New("invalid ECR image digest")
	}
	// Publication is gated on a completed basic scan, not merely a successful push.
	scanCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		result, e := a.ECR.DescribeImageScanFindings(scanCtx, &ecr.DescribeImageScanFindingsInput{RepositoryName: aws.String(a.repository(id)), ImageId: &er.ImageIdentifier{ImageDigest: aws.String(digest)}})
		if e == nil && result.ImageScanStatus != nil && result.ImageScanStatus.Status == er.ScanStatusComplete && result.ImageScanFindings != nil {
			counts := result.ImageScanFindings.FindingSeverityCounts
			if counts["CRITICAL"] > 0 || counts["HIGH"] > 0 {
				return "", "", errors.New("ECR scan rejected HIGH/CRITICAL findings")
			}
			break
		}
		if e != nil && awsCode(e) != "ScanNotFoundException" {
			return "", "", errors.New("ECR image scan unavailable")
		}
		if err = pause(scanCtx, 4*time.Second); err != nil {
			return "", "", errors.New("ECR image scan did not complete")
		}
	}
	return a.registry() + "/" + a.repository(id) + "@" + digest, "", nil
}
func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *AWSRuntime) Start(ctx context.Context, image, name, id, databaseURL string) error {
	match := workloadPattern.FindStringSubmatch(name)
	if len(match) == 0 || match[1] != id || !projectIDPattern.MatchString(id) {
		return errors.New("invalid workload namespace")
	}
	prefix := a.registry() + "/" + a.repository(id) + "@"
	if !strings.HasPrefix(image, prefix) || !digestPattern.MatchString(strings.TrimPrefix(image, prefix)) {
		return errors.New("workload image must be a project-owned ECR digest")
	}
	if _, err := a.Store.Pool.Exec(ctx, `INSERT INTO aws_workloads(name,project_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, name, id); err != nil {
		return err
	}
	secretName := a.Prefix + "/workloads/" + name
	secret, err := a.Secrets.CreateSecret(ctx, &secretsmanager.CreateSecretInput{Name: aws.String(secretName), SecretString: aws.String(databaseURL)})
	var secretARN string
	if awsCode(err) == "ResourceExistsException" {
		existing, e := a.Secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(secretName)})
		if e != nil {
			return e
		}
		secretARN = aws.ToString(existing.ARN)
		_, err = a.Secrets.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{SecretId: aws.String(secretARN), SecretString: aws.String(databaseURL)})
	} else if err == nil {
		secretARN = aws.ToString(secret.ARN)
	}
	if err != nil {
		return errors.New("workload database secret could not be prepared")
	}
	if _, err = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads SET secret_arn=$2 WHERE name=$1`, name, secretARN); err != nil {
		return err
	}
	discovery, err := a.Discovery.CreateService(ctx, &servicediscovery.CreateServiceInput{Name: aws.String(name), CreatorRequestId: aws.String(name), DnsConfig: &sd.DnsConfig{NamespaceId: aws.String(a.NamespaceID), RoutingPolicy: sd.RoutingPolicyMultivalue, DnsRecords: []sd.DnsRecord{{Type: sd.RecordTypeA, TTL: aws.Int64(10)}}}, HealthCheckCustomConfig: &sd.HealthCheckCustomConfig{FailureThreshold: aws.Int32(1)}})
	var discoveryID, discoveryARN string
	if err != nil {
		var exists *sd.ServiceAlreadyExists
		if errors.As(err, &exists) {
			discoveryID = aws.ToString(exists.ServiceId)
			discoveryARN = aws.ToString(exists.ServiceArn)
		} else {
			return errors.New("private service discovery could not be prepared")
		}
	} else {
		discoveryID = aws.ToString(discovery.Service.Id)
		discoveryARN = aws.ToString(discovery.Service.Arn)
	}
	if _, err = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads SET discovery_id=$2 WHERE name=$1`, name, discoveryID); err != nil {
		return err
	}
	definition, err := a.ECS.RegisterTaskDefinition(ctx, a.taskDefinition(name, image, secretARN))
	if err != nil {
		return errors.New("Fargate task definition could not be registered")
	}
	task := aws.ToString(definition.TaskDefinition.TaskDefinitionArn)
	if _, err = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads SET task_definition=$2 WHERE name=$1`, name, task); err != nil {
		return err
	}
	service, err := a.ECS.CreateService(ctx, &ecs.CreateServiceInput{Cluster: aws.String(a.Cluster), ServiceName: aws.String(name), ClientToken: aws.String(name), TaskDefinition: aws.String(task), DesiredCount: aws.Int32(1), LaunchType: ec.LaunchTypeFargate, PlatformVersion: aws.String("1.4.0"), EnableExecuteCommand: false, NetworkConfiguration: &ec.NetworkConfiguration{AwsvpcConfiguration: &ec.AwsVpcConfiguration{Subnets: a.Subnets, SecurityGroups: []string{a.TaskSG}, AssignPublicIp: ec.AssignPublicIpDisabled}}, ServiceRegistries: []ec.ServiceRegistry{{RegistryArn: aws.String(discoveryARN)}}, DeploymentConfiguration: &ec.DeploymentConfiguration{MinimumHealthyPercent: aws.Int32(100), MaximumPercent: aws.Int32(200)}})
	if err != nil {
		return errors.New("private Fargate service could not start")
	}
	_, err = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads SET service_arn=$2 WHERE name=$1`, name, aws.ToString(service.Service.ServiceArn))
	return err
}

func (a *AWSRuntime) taskDefinition(name, image, secretARN string) *ecs.RegisterTaskDefinitionInput {
	tmp := []ec.MountPoint{{SourceVolume: aws.String("tmp"), ContainerPath: aws.String("/tmp"), ReadOnly: aws.Bool(false)}}
	capabilities := &ec.LinuxParameters{Capabilities: &ec.KernelCapabilities{Drop: []string{"ALL"}}}
	return &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(name), RequiresCompatibilities: []ec.Compatibility{ec.CompatibilityFargate}, NetworkMode: ec.NetworkModeAwsvpc,
		Cpu: aws.String("512"), Memory: aws.String("1024"), ExecutionRoleArn: aws.String(a.ExecutionRole),
		RuntimePlatform: &ec.RuntimePlatform{CpuArchitecture: ec.CPUArchitectureX8664, OperatingSystemFamily: ec.OSFamilyLinux},
		Volumes:         []ec.Volume{{Name: aws.String("tmp")}},
		// No task role: generated containers never receive workload AWS credentials.
		ContainerDefinitions: []ec.ContainerDefinition{
			{Name: aws.String("prepare-tmp"), Image: aws.String(a.InitImage), Essential: aws.Bool(false), User: aws.String("0"),
				EntryPoint: []string{"/bin/sh", "-c"}, Command: []string{"chmod 1777 /tmp"}, ReadonlyRootFilesystem: aws.Bool(true), LinuxParameters: capabilities, MountPoints: tmp},
			{Name: aws.String("app"), Image: aws.String(image), Essential: aws.Bool(true), User: aws.String("10001"),
				ReadonlyRootFilesystem: aws.Bool(true), LinuxParameters: capabilities, MountPoints: tmp,
				DependsOn:        []ec.ContainerDependency{{ContainerName: aws.String("prepare-tmp"), Condition: ec.ContainerConditionSuccess}},
				PortMappings:     []ec.PortMapping{{ContainerPort: aws.Int32(8080), Protocol: ec.TransportProtocolTcp}},
				Environment:      []ec.KeyValuePair{{Name: aws.String("PORT"), Value: aws.String("8080")}},
				Secrets:          []ec.Secret{{Name: aws.String("DATABASE_URL"), ValueFrom: aws.String(secretARN)}},
				LogConfiguration: &ec.LogConfiguration{LogDriver: ec.LogDriverAwslogs, Options: map[string]string{"awslogs-group": a.LogGroup, "awslogs-region": a.Region, "awslogs-stream-prefix": name}}},
		},
	}
}
func (a *AWSRuntime) Healthy(ctx context.Context, name string) error {
	check, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		state, err := a.ECS.DescribeServices(check, &ecs.DescribeServicesInput{Cluster: aws.String(a.Cluster), Services: []string{name}})
		if err != nil {
			return errors.New("Fargate service status unavailable")
		}
		if len(state.Services) == 1 && state.Services[0].RunningCount == 1 && state.Services[0].PendingCount == 0 {
			return nil
		}
		if err = pause(check, 3*time.Second); err != nil {
			return errors.New("Fargate service did not start")
		}
	}
}
func (a *AWSRuntime) Stop(ctx context.Context, name string) {
	if !workloadPattern.MatchString(name) {
		return
	}
	_, _ = a.ECS.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(a.Cluster), Service: aws.String(name), DesiredCount: aws.Int32(0)})
}
func (a *AWSRuntime) Remove(_ context.Context, name string) {
	if !workloadPattern.MatchString(name) {
		return
	}
	// Cloud teardown must finish independently even after a build is cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, _ = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads SET cleanup_requested=true WHERE name=$1`, name)
	var service, discovery, definition, secret string
	if a.Store.Pool.QueryRow(ctx, `SELECT service_arn,discovery_id,task_definition,secret_arn FROM aws_workloads WHERE name=$1`, name).Scan(&service, &discovery, &definition, &secret) != nil {
		return
	}
	if service == "" {
		state, err := a.ECS.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(a.Cluster), Services: []string{name}})
		if err != nil {
			return
		}
		if len(state.Services) == 1 && aws.ToString(state.Services[0].Status) != "INACTIVE" {
			service = aws.ToString(state.Services[0].ServiceArn)
		}
	}
	if service != "" {
		a.Stop(ctx, name)
		_, err := a.ECS.DeleteService(ctx, &ecs.DeleteServiceInput{Cluster: aws.String(a.Cluster), Service: aws.String(service), Force: aws.Bool(true)})
		if err != nil {
			return
		}
		for {
			state, err := a.ECS.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(a.Cluster), Services: []string{name}})
			if err == nil && (len(state.Services) == 0 || aws.ToString(state.Services[0].Status) == "INACTIVE") {
				break
			}
			if pause(ctx, 3*time.Second) != nil {
				return
			}
		}
	}
	if discovery != "" {
		for {
			_, err := a.Discovery.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: aws.String(discovery)})
			if err == nil || awsCode(err) == "ServiceNotFound" {
				break
			}
			if pause(ctx, 3*time.Second) != nil {
				return
			}
		}
	}
	if definition != "" {
		if _, err := a.ECS.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: aws.String(definition)}); err != nil {
			return
		}
	}
	if secret != "" {
		if _, err := a.Secrets.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String(secret), ForceDeleteWithoutRecovery: aws.Bool(true)}); err != nil {
			return
		}
	}
	_, _ = a.Store.Pool.Exec(ctx, `DELETE FROM aws_workloads WHERE name=$1`, name)
}
func (a *AWSRuntime) Resolve(ctx context.Context, id, name string) (string, error) {
	match := workloadPattern.FindStringSubmatch(name)
	if len(match) == 0 || match[1] != id {
		return "", errors.New("invalid workload target")
	}
	var exists bool
	if err := a.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM aws_workloads WHERE name=$1 AND project_id=$2 AND service_arn<>'')`, name, id).Scan(&exists); err != nil || !exists {
		return "", errors.New("workload is unavailable")
	}
	return fmt.Sprintf("%s.%s:8080", name, a.Namespace), nil
}

func (a *AWSRuntime) cancelBuild(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, _ = a.Store.Pool.Exec(ctx, `UPDATE aws_builds SET cancel_requested=true WHERE id=$1`, id)
	_, _ = a.Builds.StopBuild(ctx, &codebuild.StopBuildInput{Id: aws.String(id)})
	for ctx.Err() == nil {
		state, err := a.Builds.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{Ids: []string{id}})
		if err == nil && len(state.Builds) == 1 && string(state.Builds[0].BuildStatus) != "IN_PROGRESS" {
			_, _ = a.Store.Pool.Exec(ctx, `DELETE FROM aws_builds WHERE id=$1`, id)
			return
		}
		if pause(ctx, 2*time.Second) != nil {
			return
		}
	}
}
func (a *AWSRuntime) Reconcile(ctx context.Context, s *Service) {
	// Recover orphan candidates/builds after a stopped controller, without touching
	// recorded deployments or workloads whose build lease is still live.
	_, _ = a.Store.Pool.Exec(ctx, `UPDATE aws_workloads w SET cleanup_requested=true WHERE created_at<now()-interval '15 minutes' AND NOT EXISTS(SELECT 1 FROM deployments d WHERE d.container_name=w.name) AND NOT EXISTS(SELECT 1 FROM build_jobs b WHERE b.project_id=w.project_id AND b.lease_until>now() AND b.status NOT IN ('succeeded','failed','cancelled'))`)
	_, _ = a.Store.Pool.Exec(ctx, `UPDATE aws_builds w SET cancel_requested=true WHERE created_at<now()-interval '15 minutes' AND NOT EXISTS(SELECT 1 FROM build_jobs b WHERE b.project_id=w.project_id AND b.lease_until>now() AND b.status NOT IN ('succeeded','failed','cancelled'))`)
	rows, err := a.Store.Pool.Query(ctx, `SELECT name FROM aws_workloads WHERE cleanup_requested`)
	if err != nil {
		return
	}
	names := []string{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			names = append(names, name)
		}
	}
	rows.Close()
	for _, name := range names {
		a.Remove(ctx, name)
	}
	rows, err = a.Store.Pool.Query(ctx, `SELECT id FROM aws_builds WHERE cancel_requested`)
	if err != nil {
		return
	}
	names = nil
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			names = append(names, name)
		}
	}
	rows.Close()
	for _, name := range names {
		a.cancelBuild(name)
	}
	rows, err = a.Store.Pool.Query(ctx, `SELECT id::text,project_id::text FROM build_jobs b WHERE status NOT IN ('queued','succeeded','failed','cancelled') AND message='Cloud cleanup pending; the previous deployment is unchanged.' AND NOT EXISTS(SELECT 1 FROM aws_workloads w WHERE w.project_id=b.project_id AND cleanup_requested) AND NOT EXISTS(SELECT 1 FROM aws_builds w WHERE w.project_id=b.project_id AND cancel_requested)`)
	if err != nil {
		return
	}
	type job struct{ id, project string }
	jobs := []job{}
	for rows.Next() {
		var j job
		if rows.Scan(&j.id, &j.project) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		_ = s.finishBuild(ctx, j.id, j.project, errors.New("cloud operation stopped before publication"))
	}
}
