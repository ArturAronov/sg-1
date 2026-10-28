package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type projectFile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *API) fileRoutes(mux *http.ServeMux) {
	wrap := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if a.Platform == nil || a.Files == nil {
				writeError(w, 503, "storage_unavailable", "Private object storage is not configured")
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("GET /v1/projects/{slug}/files", wrap(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		rows, err := a.Platform.Store.Pool.Query(r.Context(), `SELECT id::text,name,size,created_at FROM project_files WHERE project_id=$1 ORDER BY created_at DESC LIMIT 200`, p.ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		defer rows.Close()
		files := []projectFile{}
		for rows.Next() {
			var file projectFile
			if err = rows.Scan(&file.ID, &file.Name, &file.Size, &file.CreatedAt); err != nil {
				a.fail(w, r, err)
				return
			}
			files = append(files, file)
		}
		if err = rows.Err(); err != nil {
			a.fail(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"files": files})
	}))
	mux.HandleFunc("POST /v1/projects/{slug}/files", wrap(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if p.Role == "read" {
			writeError(w, 403, "forbidden", "Write access is required")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, (10<<20)+(64<<10))
		reader, err := r.MultipartReader()
		if err != nil {
			writeError(w, 400, "invalid_file", "Choose a file")
			return
		}
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "file" {
			writeError(w, 400, "invalid_file", "Choose one file")
			return
		}
		defer part.Close()
		name := part.FileName()
		if len(name) < 1 || len(name) > 200 || strings.ContainsAny(name, "/\\\x00\r\n") {
			writeError(w, 400, "invalid_file", "Use a filename of at most 200 bytes")
			return
		}
		data, err := io.ReadAll(io.LimitReader(part, (10<<20)+1))
		if err != nil || len(data) > 10<<20 {
			writeError(w, 413, "file_too_large", "Files must be at most 10 MiB")
			return
		}
		var file projectFile
		file.Name = name
		file.Size = int64(len(data))
		if err = a.Platform.Store.Pool.QueryRow(r.Context(), `SELECT gen_random_uuid()::text`).Scan(&file.ID); err != nil {
			a.fail(w, r, err)
			return
		}
		key := p.ID + "/attachments/" + file.ID
		if err = a.Files.Put(r.Context(), key, data); err != nil {
			a.fail(w, r, err)
			return
		}
		tx, err := a.Platform.Store.Pool.Begin(r.Context())
		if err != nil {
			_ = a.Files.Delete(r.Context(), key)
			a.fail(w, r, err)
			return
		}
		defer tx.Rollback(r.Context())
		err = tx.QueryRow(r.Context(), `INSERT INTO project_files(id,project_id,actor_id,name,size,object_key) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, file.ID, p.ID, currentUser(r).ID, file.Name, file.Size, key).Scan(&file.CreatedAt)
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO audit_events(actor_id,project_id,action) VALUES($1,$2,'file.uploaded')`, currentUser(r).ID, p.ID)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			_ = a.Files.Delete(r.Context(), key)
			a.fail(w, r, err)
			return
		}
		writeJSON(w, 201, map[string]any{"file": file})
	}))
	mux.HandleFunc("GET /v1/projects/{slug}/files/{file}", wrap(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Store.Project(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			a.fail(w, r, err)
			return
		}
		var name, key string
		var size int64
		err = a.Platform.Store.Pool.QueryRow(r.Context(), `SELECT name,object_key,size FROM project_files WHERE project_id=$1 AND id::text=$2`, p.ID, r.PathValue("file")).Scan(&name, &key, &size)
		if err != nil {
			writeError(w, 404, "not_found", "File not found")
			return
		}
		data, err := a.Files.Get(r.Context(), key)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		defer data.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.WriteHeader(200)
		_, _ = io.Copy(w, io.LimitReader(data, 10<<20))
	}))
}
