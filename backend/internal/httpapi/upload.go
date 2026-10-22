package httpapi

import (
	"encoding/json"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) initUpload(w http.ResponseWriter, r *http.Request) {
	// Part URLs are presigned one at a time (POST /uploads/{id}/presign) right
	// before each part is sent, so long uploads don't outlive their URLs.
	var in struct {
		Filename    string  `json:"filename"`
		ContentType string  `json:"contentType"`
		FolderID    *string `json:"folderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Filename == "" {
		writeErr(w, http.StatusBadRequest, "filename required")
		return
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}

	id := uuid.NewString()
	key := "raw/" + id + "/" + storage.SafeName(in.Filename)

	uploadID, err := s.st.CreateMultipart(r.Context(), key, in.ContentType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	season, ep := storage.ParseSE(in.Filename)
	_, err = s.db.Exec(r.Context(), `
		INSERT INTO media_files (id, uploaded_by, filename, status, raw_key, season, episode, folder_id)
		VALUES ($1, $2, $3, 'uploading', $4, $5, $6, $7)`,
		id, nickname(r), in.Filename, key, season, ep, emptyToNil(in.FolderID))
	if isForeignKeyViolation(err) {
		writeErr(w, http.StatusBadRequest, "folder not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"mediaId":  id,
		"uploadId": uploadID,
	})
}

func (s *Server) presignPart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in struct {
		UploadID   string `json:"uploadId"`
		PartNumber int32  `json:"partNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.UploadID == "" ||
		in.PartNumber < 1 || in.PartNumber > 10000 { // S3's part limit
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	var rawKey string
	err := s.db.QueryRow(r.Context(),
		`SELECT raw_key FROM media_files WHERE id=$1 AND status='uploading'`,
		id).Scan(&rawKey)
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

	var rawKey string
	err := s.db.QueryRow(r.Context(),
		`SELECT raw_key FROM media_files WHERE id=$1 AND status='uploading'`,
		id).Scan(&rawKey)
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
