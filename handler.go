package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/aisola/archivist/middleware"
)

type Handler struct {
	logger *zap.Logger
	b2 *B2
}

func New(l *zap.Logger, b2 *B2) *Handler {
	return &Handler{
		logger: l,
		b2: b2,
	}
}

func (h *Handler) info(r *http.Request, msg string, fields ...zap.Field) {
	if id := middleware.GetRequestID(r.Context()); id != "" {
		fields = append(fields, zap.String("request_id", id))
	}

	h.logger.Info(msg, fields...)
}

func (h *Handler) warning(r *http.Request, msg string, fields ...zap.Field) {
	if id := middleware.GetRequestID(r.Context()); id != "" {
		fields = append(fields, zap.String("request_id", id))
	}

	h.logger.Warn(msg, fields...)
}

func (h *Handler) error(r *http.Request, msg string, fields ...zap.Field) {
	if id := middleware.GetRequestID(r.Context()); id != "" {
		fields = append(fields, zap.String("request_id", id))
	}

	h.logger.Error(msg, fields...)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		buf bytes.Buffer

		fileName      = r.Header.Get("Archivist-File-Name")
		fileMediaType = r.Header.Get("Content-Type")
	)

	if fileName == "" || fileMediaType == "" {
		h.warning(r, "bad request, name or media type missing",
			zap.String("file_name", fileName),
			zap.String("file_media_type", fileMediaType),
		)
		http.Error(w, "file name and media type required", http.StatusBadRequest)
		return
	}

	hash, err := readAndHashBody(r, &buf)
	if err != nil {
		h.error(r, "failed to read body", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	size := buf.Len()

	fileID, err := h.b2.Upload(r.Context(), &buf, fileName, fileMediaType, hash, size)
	if err != nil {
		h.error(r, "failed to upload file", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.writeResponse(w, r, fileID, fileName, fileMediaType, hash, size)
}

func (h *Handler) writeResponse(w http.ResponseWriter, r *http.Request, fileID, fileName, fileMediaType, hash string, size int) {
	h.info(r, "file successfully uploaded",
		zap.String("file_id", fileID),
		zap.String("file_name", fileName),
		zap.String("file_media_type", fileMediaType),
		zap.String("file_hash", hash),
		zap.Int("file_size", size),
	)

	data, _ := json.Marshal(map[string]interface{}{
		"id": fileID,
		"name":       fileName,
		"media_type": fileMediaType,
		"sha1":       hash,
		"size":       size,
	})

	fmt.Fprint(w, string(data))
}

func readAndHashBody(r *http.Request, buf *bytes.Buffer) (string, error) {
	hash := sha1.New()
	tee := io.TeeReader(r.Body, buf)

	if _, err := io.Copy(hash, tee); err != nil {
		return "", fmt.Errorf("failed to copy body: %w", err)
	}

	sum := hash.Sum(nil)
	return hex.EncodeToString(sum[:]), nil
}

