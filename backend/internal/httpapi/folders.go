package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Library folders: a series, its seasons, and so on. Folders nest through
// parentId (null = top level); media and torrents point at one via folderId.

type FolderItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parentId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		`SELECT id, name, parent_id, created_at FROM folders ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []FolderItem{}
	for rows.Next() {
		var f FolderItem
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, f)
	}
	writeJSON(w, http.StatusOK, items)
}

func folderName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, s != "" && utf8.RuneCountInString(s) <= 100
}

func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name, ok := folderName(in.Name)
	if !ok {
		writeErr(w, http.StatusBadRequest, "name must be 1-100 characters")
		return
	}
	id := uuid.NewString()
	_, err := s.db.Exec(r.Context(),
		`INSERT INTO folders (id, name, parent_id) VALUES ($1, $2, $3)`, id, name, emptyToNil(in.ParentID))
	if isForeignKeyViolation(err) {
		writeErr(w, http.StatusBadRequest, "parent folder not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// PATCH /api/folders/{id} {name?, parentId?}: rename and/or move (parentId null = top level).
func (s *Server) updateFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in struct {
		Name     *string     `json:"name"`
		ParentID opt[string] `json:"parentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil {
		name, ok := folderName(*in.Name)
		if !ok {
			writeErr(w, http.StatusBadRequest, "name must be 1-100 characters")
			return
		}
		in.Name = &name
	}
	if in.ParentID.Set {
		in.ParentID.V = emptyToNil(in.ParentID.V)
	}
	if in.ParentID.V != nil {
		// Moving a folder into itself or one of its own subfolders would make a cycle.
		var cycle bool
		err := s.db.QueryRow(r.Context(), `
			WITH RECURSIVE up AS (
				SELECT id, parent_id FROM folders WHERE id = $1
				UNION
				SELECT f.id, f.parent_id FROM folders f JOIN up ON f.id = up.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, *in.ParentID.V, id).Scan(&cycle)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cycle {
			writeErr(w, http.StatusBadRequest, "a folder can't go inside itself")
			return
		}
	}
	tag, err := s.db.Exec(r.Context(), `
		UPDATE folders
		SET name      = COALESCE($2, name),
		    parent_id = CASE WHEN $3::boolean THEN $4::text ELSE parent_id END
		WHERE id = $1`, id, in.Name, in.ParentID.Set, in.ParentID.V)
	if isForeignKeyViolation(err) {
		writeErr(w, http.StatusBadRequest, "parent folder not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/folders/{id}: the folder's subfolders and media move up to its
// parent; nothing inside is deleted.
func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var parent *string
	err = tx.QueryRow(r.Context(), `SELECT parent_id FROM folders WHERE id=$1 FOR UPDATE`, id).Scan(&parent)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, q := range []string{
		`UPDATE folders SET parent_id=$2 WHERE parent_id=$1`,
		`UPDATE media_files SET folder_id=$2 WHERE folder_id=$1`,
		`UPDATE torrents SET folder_id=$2 WHERE folder_id=$1`,
	} {
		if _, err := tx.Exec(r.Context(), q, id, parent); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM folders WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// emptyToNil treats "" like "no folder".
func emptyToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
