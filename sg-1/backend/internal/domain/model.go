package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrForbidden           = errors.New("forbidden")
	ErrSlugTaken           = errors.New("project URL is already in use")
	ErrIdempotencyConflict = errors.New("idempotency key was used for a different request")
	slugPattern            = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	uuidPattern            = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

var reservedSlugs = map[string]bool{
	"admin": true, "api": true, "auth": true, "build": true,
	"login": true, "logout": true, "healthz": true, "readyz": true,
	"settings": true, "assets": true, "static": true, "projects": true,
	"departments": true, "favicon": true, "robots": true, "sitemap": true,
}

type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	PlatformAdmin bool   `json:"platform_admin"`
}

type Department struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

type Project struct {
	ID             string    `json:"id"`
	DepartmentID   string    `json:"department_id"`
	DepartmentName string    `json:"department_name"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	AccessMode     string    `json:"access_mode"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateProject struct {
	DepartmentID string `json:"department_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
}

func ValidateSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 48 || !slugPattern.MatchString(slug) {
		return errors.New("URL name must be 2–48 lowercase letters, numbers, or single hyphens, starting with a letter")
	}
	if reservedSlugs[slug] {
		return errors.New("that URL name is reserved by SG-1")
	}
	return nil
}

func (p CreateProject) Validate() error {
	if !uuidPattern.MatchString(p.DepartmentID) {
		return errors.New("a valid department is required")
	}
	if p.Name != strings.TrimSpace(p.Name) || utf8.RuneCountInString(p.Name) < 2 || utf8.RuneCountInString(p.Name) > 100 {
		return errors.New("project name must be 2–100 characters without surrounding spaces")
	}
	if utf8.RuneCountInString(p.Description) > 1000 {
		return errors.New("description must be at most 1000 characters")
	}
	return ValidateSlug(p.Slug)
}
