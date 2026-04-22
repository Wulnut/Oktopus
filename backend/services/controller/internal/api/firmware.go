package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/db"
)

var firmwareUploadServiceURL = getEnvOrDefault("FIRMWARE_UPLOAD_URL", "http://firmware-upload:8006")
var firmwarePublicBaseURL = getEnvOrDefault("FIRMWARE_BASE_URL", "http://file-server:8004/firmwares")

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// GET /api/firmware
func (a *Api) listFirmware(w http.ResponseWriter, r *http.Request) {
	list, err := a.tenantDB(r).ListFirmware(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.Firmware{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// POST /api/firmware  (multipart/form-data: file + name + build_version + phase + download_url)
func (a *Api) uploadFirmware(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(500 << 20); err != nil && err != http.ErrNotMultipart {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	buildVersion := r.FormValue("build_version")
	vendor := r.FormValue("vendor")
	model := r.FormValue("model")
	hwVersion := r.FormValue("hw_version")
	if name == "" || buildVersion == "" {
		http.Error(w, "name and build_version are required", http.StatusBadRequest)
		return
	}

	phase := db.FirmwarePhase(r.FormValue("phase"))
	if phase == "" {
		phase = db.PhaseInternalTesting
	}
	if phase != db.PhaseInternalTesting && phase != db.PhaseRelease {
		http.Error(w, "invalid phase", http.StatusBadRequest)
		return
	}

	var fileName, fingerprint, downloadURL string
	var fileSize int64

	file, header, err := r.FormFile("file")
	if err != nil && err != http.ErrMissingFile {
		http.Error(w, "error reading file: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err == nil && file != nil {
		defer file.Close()

		// Sanitize filename to prevent path traversal
		fileName = filepath.Base(header.Filename)
		if fileName == "." || fileName == ".." || strings.ContainsAny(fileName, `/\`) {
			http.Error(w, "Invalid filename", http.StatusBadRequest)
			return
		}

		// Stream firmware to temp file instead of buffering in memory
		h := sha256.New()
		tmpFile, tmpErr := os.CreateTemp(os.TempDir(), "firmware-*")
		if tmpErr != nil {
			http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		written, copyErr := io.Copy(tmpFile, io.TeeReader(file, h))
		if copyErr != nil {
			http.Error(w, "Failed to write firmware: "+copyErr.Error(), http.StatusInternalServerError)
			return
		}
		fingerprint = hex.EncodeToString(h.Sum(nil))
		fileSize = written

		tenantSlug := middleware.GetTenantSlug(r)
		if fwdErr := forwardFileToUploadService(tmpFile.Name(), fileName, tenantSlug, r.Header.Get("Authorization")); fwdErr != nil {
			http.Error(w, "upload to file server failed: "+fwdErr.Error(), http.StatusBadGateway)
			return
		}
		downloadURL = firmwarePublicBaseURL + "/" + tenantSlug + "/" + fileName
	} else {
		downloadURL = r.FormValue("download_url")
		if downloadURL == "" {
			http.Error(w, "either a file or download_url is required", http.StatusBadRequest)
			return
		}
		fileName = r.FormValue("file_name")
		fingerprint = r.FormValue("fingerprint")
	}

	fw := db.Firmware{
		Name:         name,
		Vendor:       vendor,
		Model:        model,
		HWVersion:    hwVersion,
		BuildVersion: buildVersion,
		FileSize:     fileSize,
		Fingerprint:  fingerprint,
		Phase:        phase,
		DownloadURL:  downloadURL,
		FileName:     fileName,
	}

	created, err := a.tenantDB(r).CreateFirmware(r.Context(), fw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// DELETE /api/firmware/{id}
func (a *Api) deleteFirmware(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	fw, err := a.tenantDB(r).GetFirmware(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fw.FileName != "" {
		tenantSlug := middleware.GetTenantSlug(r)
		// Best-effort delete from file service — ignore errors
		deleteFileFromUploadService(fw.FileName, tenantSlug, r.Header.Get("Authorization"))
	}
	if err := a.tenantDB(r).DeleteFirmware(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Disable campaigns that referenced this firmware
	if err := a.tenantDB(r).DisableCampaignsByFirmware(r.Context(), id); err != nil {
		log.Printf("Warning: failed to disable campaigns for firmware %s: %v", id.Hex(), err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/firmware/{id}
func (a *Api) updateFirmware(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name         string `json:"name"`
		Vendor       string `json:"vendor"`
		Model        string `json:"model"`
		HWVersion    string `json:"hw_version"`
		BuildVersion string `json:"build_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.BuildVersion == "" {
		http.Error(w, "name and build_version are required", http.StatusBadRequest)
		return
	}
	fw := db.Firmware{
		Name:         body.Name,
		Vendor:       body.Vendor,
		Model:        body.Model,
		HWVersion:    body.HWVersion,
		BuildVersion: body.BuildVersion,
	}
	matched, err := a.tenantDB(r).UpdateFirmware(r.Context(), id, fw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if matched == 0 {
		http.Error(w, "firmware not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/firmware/{id}/phase
func (a *Api) updateFirmwarePhase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Phase db.FirmwarePhase `json:"phase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Phase != db.PhaseInternalTesting && body.Phase != db.PhaseRelease {
		http.Error(w, "phase must be 'internal_testing' or 'release'", http.StatusBadRequest)
		return
	}
	if err := a.tenantDB(r).UpdateFirmwarePhase(r.Context(), id, body.Phase); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func forwardFileToUploadService(tmpPath, fileName, tenantSlug, authHeader string) error {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		f, err := os.Open(tmpPath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer f.Close()
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
		pw.Close()
	}()

	uploadURL := fmt.Sprintf("%s/upload?tenant=%s", firmwareUploadServiceURL, url.QueryEscape(tenantSlug))
	req, err := http.NewRequest(http.MethodPost, uploadURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", authHeader)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload service returned %d", resp.StatusCode)
	}
	return nil
}

func deleteFileFromUploadService(fileName, tenantSlug, authHeader string) {
	req, err := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/delete?tenant=%s&name=%s", firmwareUploadServiceURL, url.QueryEscape(tenantSlug), url.QueryEscape(fileName)), nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
