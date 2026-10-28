package domain

import (
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	for _, slug := range []string{"maintenance-tracker", "aa", "project-1", strings.Repeat("a", 48)} {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("valid slug %q: %v", slug, err)
		}
	}
	for _, slug := range []string{"", "a", "build", "api", "auth", "admin", "login", "settings", "robots", "Upper", "double--hyphen", "../secret", "a/b", "-name", "name-", "1name", "with_underscore", strings.Repeat("a", 49)} {
		if ValidateSlug(slug) == nil {
			t.Errorf("accepted invalid slug %q", slug)
		}
	}
}

func TestCreateProjectValidation(t *testing.T) {
	valid := CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Maintenance tracker", Slug: "maintenance-tracker"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CreateProject){
		func(p *CreateProject) { p.DepartmentID = "not-a-uuid" },
		func(p *CreateProject) { p.Name = " spaced " },
		func(p *CreateProject) { p.Name = "x" },
		func(p *CreateProject) { p.Description = strings.Repeat("é", 1001) },
		func(p *CreateProject) { p.Slug = "admin" },
	} {
		p := valid
		mutate(&p)
		if p.Validate() == nil {
			t.Errorf("accepted invalid project: %+v", p)
		}
	}
}
