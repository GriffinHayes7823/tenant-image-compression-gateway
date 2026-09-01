package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/image-tenant-gateway/internal/gateway"
)

type service struct {
	tenants    *gateway.TenantRegistry
	compressor *gateway.Compressor
}

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	svc := &service{tenants: gateway.NewTenantRegistry(), compressor: gateway.NewCompressor(apiKey, nil)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tenants/{tenantID}", svc.onboard)
	mux.HandleFunc("PUT /admin/tenants/{tenantID}/state", svc.setState)
	mux.HandleFunc("POST /tenants/{tenantID}/images", svc.compressImage)
	addr := envOr("ADDR", ":8080")
	log.Printf("image tenant gateway listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *service) onboard(w http.ResponseWriter, r *http.Request) {
	if err := s.tenants.Onboard(r.PathValue("tenantID")); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"state": string(gateway.StateActive)})
}

func (s *service) setState(w http.ResponseWriter, r *http.Request) {
	var input struct {
		State gateway.AccountState `json:"state"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := s.tenants.SetState(r.PathValue("tenantID"), input.State); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": string(input.State)})
}

func (s *service) compressImage(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if err := s.tenants.AuthorizeCompression(tenantID); err != nil {
		writeDomainError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart image is required"})
		return
	}
	defer file.Close()
	image, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read image"})
		return
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		sum := sha256.Sum256(append([]byte(tenantID+":"), image...))
		requestID = hex.EncodeToString(sum[:])
	}
	data, err := s.compressor.Compress(r.Context(), image, header.Filename, tenantID+":"+requestID)
	if err != nil {
		var apiErr *gateway.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			writeJSON(w, apiErr.StatusCode, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "compression request failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"tenant_id":%q,"optimized":%s}`, tenantID, data)
}

func writeDomainError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	if errors.Is(err, gateway.ErrTenantUnknown) {
		status = http.StatusNotFound
	}
	if errors.Is(err, gateway.ErrAccountInactive) {
		status = http.StatusForbidden
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
