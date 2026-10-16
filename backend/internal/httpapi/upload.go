package httpapi

import (
	"encoding/json"
	"liotom/whateverserver/internal/auth"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) initUpload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Parts       int32  `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Filename == "" {
		writeErr(w, http.StatusBadRequest, "filename required")
		return
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}

	user := auth.FromContext(r.Context())
	id := uuid.NewString()
	key := "raw/" + id + "/" + storage.SafeName(in.Filename)

	uploadID, err := s.st.CreateMultipart(r.Context(), key, in.ContentType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	season, ep := storage.ParseSE(in.Filename)
	_, err = s.db.Exec(r.Context(), `
		INSERT INTO media_files (id, uploader_id, filename, status, raw_key, season, episode)
		VALUES ($1, $2, $3, 'uploading', $4, $5, $6)`,
		id, user.ID, in.Filename, key, season, ep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var urls []string
	if in.Parts > 0 && in.Parts <= 10000 {
		urls = make([]string, in.Parts)
		for i := int32(1); i <= in.Parts; i++ {
			u, err := s.st.PresignPart(r.Context(), key, uploadID, i)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			urls[i-1] = u
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"mediaId":  id,
		"uploadId": uploadID,
		"urls":     urls,
	})
}

func (s *Server) presignPart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in struct {
		UploadID   string `json:"uploadId"`
		PartNumber int32  `json:"partNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.UploadID == "" || in.PartNumber < 1 {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	user := auth.FromContext(r.Context())
	var rawKey string
	err := s.db.QueryRow(r.Context(),
		`SELECT raw_key FROM media_files WHERE id=$1 AND uploader_id=$2 AND status='uploading'`,
		id, user.ID).Scan(&rawKey)
	if err != nil {
		writeErr(w, http.StatusNotFound, "upload not found")
		return
	}

	url, err := s.st.PresignPart(r.Context(), rawKey, in.UploadID, in.PartNumber)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in struct {
		UploadID string         `json:"uploadId"`
		Parts    []storage.Part `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.UploadID == "" || len(in.Parts) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid parts")
		return
	}

	user := auth.FromContext(r.Context())
	var rawKey string
	err := s.db.QueryRow(r.Context(),
		`SELECT raw_key FROM media_files WHERE id=$1 AND uploader_id=$2 AND status='uploading'`,
		id, user.ID).Scan(&rawKey)
	if err != nil {
		writeErr(w, http.StatusNotFound, "upload not found")
		return
	}

	if err := s.st.CompleteMultipart(r.Context(), rawKey, in.UploadID, in.Parts); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE media_files SET status='processing' WHERE id=$1`, id)
	if err := jobs.Enqueue(r.Context(), s.db, "process", map[string]string{"mediaId": id}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "processing"})
}
