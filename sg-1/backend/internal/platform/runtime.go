package platform

import "context"

// Runtime keeps untrusted builds/workloads out of the production control host.
type Runtime interface {
	PrepareNetwork(context.Context, string) (string, error)
	Build(context.Context, string, string, map[string]string) (string, string, error)
	Start(context.Context, string, string, string, string) error
	Healthy(context.Context, string) error
	Stop(context.Context, string)
	Remove(context.Context, string)
	Resolve(context.Context, string, string) (string, error)
}
type localRuntime struct{ *Docker }

func (d localRuntime) Build(ctx context.Context, projectID, image string, files map[string]string) (string, string, error) {
	output, err := d.Docker.Build(ctx, image, files)
	return image, output, err
}
func (d localRuntime) Resolve(_ context.Context, _ string, name string) (string, error) {
	return name + ":8080", nil
}
func (s *Service) runtime() Runtime {
	if s.Cloud != nil {
		return s.Cloud
	}
	return localRuntime{s.Docker}
}
