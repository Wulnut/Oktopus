# Oktopus Roadmap Features Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement three major feature groups: (A) Firmware management UI + file upload, (B) Per-device dashboard with metrics/controls, and (C) Network topology view.

**Architecture:** All new backend endpoints follow the existing pattern in `internal/api/` — register in `api.go`, implement handler functions, use `sendUspMsg`/`NatsReq` for device communication, and MongoDB for persistence. Frontend adds new pages/sections following the Next.js page + section pattern with MUI components and `useBackendContext().httpRequest` for API calls.

**Tech Stack:** Go (Gorilla Mux, NATS, MongoDB driver, protobuf), Next.js 14, React 18, Material UI 5, ApexCharts (react-apexcharts), Socket.IO

---

## Overview

### Feature A — Firmware Management
New MongoDB collection `firmware` in db `general`. New Go handler file `firmware.go`. New nginx route for `/firmwares`. New frontend page at `/firmware` and section component.

### Feature B — Per-Device Dashboard
New MongoDB collection `device_metrics` in db `usp` for time-series. New handler file `deviceinfo.go`. Extend existing USP device detail page with new tabs: Overview, Network, Performance. Extend sidebar nav.

### Feature C — Network Topology
New handler file `topology.go`. New frontend section `devices-topology.js` on the device detail page.

---

---

# FEATURE A: Firmware Management

---

## Task A1: Firmware MongoDB schema and DB layer

**Files:**
- Create: `backend/services/controller/internal/db/firmware.go`
- Modify: `backend/services/controller/internal/db/db.go`

**Context:** The existing db package in `internal/db/` initializes MongoDB collections in `NewDatabase()` and exposes typed CRUD functions. The `UspMessage` type in `message.go` is the best reference pattern. MongoDB driver is `go.mongodb.org/mongo-driver/mongo`.

**Step 1: Add the Firmware struct and CRUD to a new file**

Create `backend/services/controller/internal/db/firmware.go`:
```go
package db

import (
    "context"
    "time"

    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

type FirmwarePhase string
const (
    PhaseInternalTesting FirmwarePhase = "internal_testing"
    PhaseRelease         FirmwarePhase = "release"
)

type Firmware struct {
    ID           primitive.ObjectID `bson:"_id,omitempty"   json:"id"`
    Name         string             `bson:"name"            json:"name"`
    BuildVersion string             `bson:"build_version"   json:"build_version"`
    FileSize     int64              `bson:"file_size"       json:"file_size"`
    Fingerprint  string             `bson:"fingerprint"     json:"fingerprint"`
    Phase        FirmwarePhase      `bson:"phase"           json:"phase"`
    DownloadURL  string             `bson:"download_url"    json:"download_url"`
    FileName     string             `bson:"file_name"       json:"file_name"`
    CreatedAt    time.Time          `bson:"created_at"      json:"created_at"`
    UpdatedAt    time.Time          `bson:"updated_at"      json:"updated_at"`
}

func (d *Database) ListFirmware(ctx context.Context) ([]Firmware, error) {
    cursor, err := d.firmware.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
    if err != nil {
        return nil, err
    }
    var results []Firmware
    if err := cursor.All(ctx, &results); err != nil {
        return nil, err
    }
    return results, nil
}

func (d *Database) CreateFirmware(ctx context.Context, fw Firmware) (Firmware, error) {
    fw.ID = primitive.NewObjectID()
    fw.CreatedAt = time.Now()
    fw.UpdatedAt = time.Now()
    _, err := d.firmware.InsertOne(ctx, fw)
    return fw, err
}

func (d *Database) DeleteFirmware(ctx context.Context, id primitive.ObjectID) error {
    _, err := d.firmware.DeleteOne(ctx, bson.M{"_id": id})
    return err
}

func (d *Database) GetFirmware(ctx context.Context, id primitive.ObjectID) (Firmware, error) {
    var fw Firmware
    err := d.firmware.FindOne(ctx, bson.M{"_id": id}).Decode(&fw)
    return fw, err
}

func (d *Database) UpdateFirmwarePhase(ctx context.Context, id primitive.ObjectID, phase FirmwarePhase) error {
    _, err := d.firmware.UpdateOne(ctx,
        bson.M{"_id": id},
        bson.M{"$set": bson.M{"phase": phase, "updated_at": time.Now()}})
    return err
}
```

**Step 2: Register the collection in db.go**

In `backend/services/controller/internal/db/db.go`, add the `firmware` field to the `Database` struct and initialize it in `NewDatabase`. Follow the exact same pattern as `d.messages = db.Collection("messages")`.

Look at lines around the struct definition and `NewDatabase` function. Add:
```go
// In Database struct:
firmware *mongo.Collection

// In NewDatabase, in the "general" database section:
d.firmware = generalDb.Collection("firmware")
```

Also add a unique index on `name`:
```go
d.firmware.Indexes().CreateOne(d.ctx, mongo.IndexModel{
    Keys:    bson.D{{Key: "name", Value: 1}},
    Options: options.Index().SetUnique(true),
})
```

**Step 3: Verify it compiles**
```bash
cd backend/services/controller && go build ./...
```
Expected: no errors.

**Step 4: Commit**
```bash
git add backend/services/controller/internal/db/firmware.go backend/services/controller/internal/db/db.go
git commit -m "feat(firmware): add Firmware MongoDB schema and CRUD layer"
```

---

## Task A2: Firmware file upload service

**Files:**
- Create: `backend/services/utils/firmware-upload/firmware-upload.js`
- Create: `backend/services/utils/firmware-upload/package.json`
- Create: `backend/services/utils/firmware-upload/build/Dockerfile`
- Create: `backend/services/utils/firmware-upload/build/Makefile`

**Context:** The `container-upload` service at `deploy/compose/container-upload-service/container-upload-service.js` is the reference pattern — it's a Node.js HTTP service using `formidable` for multipart file parsing. Port will be `8006`. Firmware files are stored at `deploy/compose/firmwares/` which is already volume-mounted in file-server.

**Step 1: Create package.json**
```json
{
  "name": "firmware-upload",
  "version": "1.0.0",
  "main": "firmware-upload.js",
  "dependencies": {
    "formidable": "^3.5.1"
  }
}
```

**Step 2: Create the upload service**

Create `backend/services/utils/firmware-upload/firmware-upload.js`:
```js
const http = require('http');
const fs = require('fs');
const path = require('path');
const { IncomingForm } = require('formidable');

const PORT = process.env.SERVER_PORT || 8006;
const FIRMWARE_DIR = process.env.FIRMWARE_DIR || '/app/firmwares';

// Ensure directory exists
fs.mkdirSync(FIRMWARE_DIR, { recursive: true });

const server = http.createServer((req, res) => {
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, DELETE, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type');

    if (req.method === 'OPTIONS') {
        res.writeHead(204); res.end(); return;
    }

    if (!req.headers['authorization']) {
        res.writeHead(401, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'Unauthorized' })); return;
    }

    if (req.method === 'POST' && req.url === '/upload') {
        const form = new IncomingForm({ maxFileSize: 500 * 1024 * 1024 }); // 500MB
        form.parse(req, (err, fields, files) => {
            if (err) {
                res.writeHead(400, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: err.message })); return;
            }
            const file = Array.isArray(files.file) ? files.file[0] : files.file;
            if (!file) {
                res.writeHead(400, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: 'No file provided' })); return;
            }
            const origName = file.originalFilename || file.newFilename;
            const destPath = path.join(FIRMWARE_DIR, origName);
            fs.rename(file.filepath, destPath, (renameErr) => {
                if (renameErr) {
                    res.writeHead(500, { 'Content-Type': 'application/json' });
                    res.end(JSON.stringify({ error: renameErr.message })); return;
                }
                const stats = fs.statSync(destPath);
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({
                    message: 'Upload successful',
                    file_name: origName,
                    file_size: stats.size,
                }));
            });
        });
        return;
    }

    if (req.method === 'DELETE' && req.url.startsWith('/delete')) {
        const url = new URL(req.url, `http://localhost`);
        const fileName = url.searchParams.get('name');
        if (!fileName || fileName.includes('/') || fileName.includes('..')) {
            res.writeHead(400, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'Invalid file name' })); return;
        }
        const filePath = path.join(FIRMWARE_DIR, fileName);
        fs.unlink(filePath, (err) => {
            if (err) {
                res.writeHead(404, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: 'File not found' })); return;
            }
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ message: 'Deleted', file_name: fileName }));
        });
        return;
    }

    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Not found' }));
});

server.listen(PORT, () => console.log(`Firmware upload service on port ${PORT}`));
```

**Step 3: Create Dockerfile**

Create `backend/services/utils/firmware-upload/build/Dockerfile`:
```dockerfile
FROM node:20-alpine
WORKDIR /app
COPY ../firmware-upload.js .
COPY ../package.json .
RUN npm install --production
CMD ["node", "firmware-upload.js"]
```

**Step 4: Create Makefile**

Create `backend/services/utils/firmware-upload/build/Makefile` (copy pattern from `file-server/build/Makefile`, change `DOCKER_APP=firmware-upload`).

**Step 5: Commit**
```bash
git add backend/services/utils/firmware-upload/
git commit -m "feat(firmware): add firmware file upload service (Node.js, port 8006)"
```

---

## Task A3: Firmware REST API handlers in controller

**Files:**
- Create: `backend/services/controller/internal/api/firmware.go`
- Modify: `backend/services/controller/internal/api/api.go`

**Context:** The controller API lives in `internal/api/`. Each feature has its own `.go` file (e.g., `wifi.go`, `history.go`, `fwupdate.go`). Routes are registered in `api.go` in the `StartApi()` method on the `iot` sub-router. The `Api` struct has fields `nc` (NATS conn), `js` (JetStream), `db` (Database), `r` (router), `nc` comes from the bridge. Read `api.go` lines 20-45 for the `Api` struct definition.

**Step 1: Create firmware.go**

Create `backend/services/controller/internal/api/firmware.go`:
```go
package api

import (
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/gorilla/mux"
    "go.mongodb.org/mongo-driver/bson/primitive"

    "github.com/OktopUSP/oktopus/backend/services/controller/internal/db"
)

const firmwareUploadURL = "http://firmware-upload:8006/upload"
const firmwareDeleteURL = "http://firmware-upload:8006/delete"
const firmwareBaseURL   = "http://file-server:8004/firmwares"

// GET /api/firmware
func (a *Api) listFirmware(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    list, err := a.db.ListFirmware(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(list)
}

// POST /api/firmware  (multipart/form-data: file + metadata fields)
func (a *Api) uploadFirmware(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(500 << 20) // 500 MB
    file, header, err := r.FormFile("file")
    if err != nil && err != http.ErrMissingFile {
        http.Error(w, "file required", http.StatusBadRequest)
        return
    }

    var fileName, fingerprint string
    var fileSize int64
    var downloadURL string

    if file != nil {
        defer file.Close()
        // Compute MD5 fingerprint and size while reading
        h := md5.New()
        data, readErr := io.ReadAll(io.TeeReader(file, h))
        if readErr != nil {
            http.Error(w, readErr.Error(), http.StatusInternalServerError)
            return
        }
        fingerprint = hex.EncodeToString(h.Sum(nil))
        fileSize = int64(len(data))
        fileName = header.Filename

        // Forward the file to the upload service
        tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("fw-%d-%s", time.Now().UnixNano(), fileName))
        if err := os.WriteFile(tmpPath, data, 0644); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        defer os.Remove(tmpPath)

        if err := forwardFileUpload(tmpPath, fileName, r.Header.Get("Authorization")); err != nil {
            http.Error(w, "upload to file server failed: "+err.Error(), http.StatusBadGateway)
            return
        }
        downloadURL = firmwareBaseURL + "/" + fileName
    }

    // If no file but downloadURL provided
    if file == nil {
        downloadURL = r.FormValue("download_url")
        if downloadURL == "" {
            http.Error(w, "file or download_url required", http.StatusBadRequest)
            return
        }
        fileName = r.FormValue("file_name")
        fingerprint = r.FormValue("fingerprint")
    }

    phase := db.FirmwarePhase(r.FormValue("phase"))
    if phase == "" {
        phase = db.PhaseInternalTesting
    }

    fw := db.Firmware{
        Name:         r.FormValue("name"),
        BuildVersion: r.FormValue("build_version"),
        FileSize:     fileSize,
        Fingerprint:  fingerprint,
        Phase:        phase,
        DownloadURL:  downloadURL,
        FileName:     fileName,
    }

    created, err := a.db.CreateFirmware(r.Context(), fw)
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
    fw, err := a.db.GetFirmware(r.Context(), id)
    if err != nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    // Delete file from upload service if it was an uploaded file
    if fw.FileName != "" {
        deleteFromFileServer(fw.FileName, r.Header.Get("Authorization"))
    }
    if err := a.db.DeleteFirmware(r.Context(), id); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
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
        http.Error(w, "invalid phase", http.StatusBadRequest)
        return
    }
    if err := a.db.UpdateFirmwarePhase(r.Context(), id, body.Phase); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}

func forwardFileUpload(tmpPath, fileName, authHeader string) error {
    // Use curl as the simplest approach — avoids multipart complexity in Go
    // In production this would use net/http with multipart writer
    return nil // placeholder - implement with proper multipart forwarding
}

func deleteFromFileServer(fileName, authHeader string) {
    // best-effort delete — ignore errors
}
```

Note: The `forwardFileUpload` function needs a proper multipart implementation. Replace the placeholder with:
```go
import (
    "bytes"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
)

func forwardFileUpload(tmpPath, fileName, authHeader string) error {
    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)
    part, err := writer.CreateFormFile("file", fileName)
    if err != nil { return err }
    data, err := os.ReadFile(tmpPath)
    if err != nil { return err }
    part.Write(data)
    writer.Close()

    req, err := http.NewRequest("POST", firmwareUploadURL, body)
    if err != nil { return err }
    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Set("Authorization", authHeader)
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("upload service returned %d", resp.StatusCode)
    }
    return nil
}
```

**Step 2: Register routes in api.go**

In `backend/services/controller/internal/api/api.go`, inside `StartApi()`, add after the existing `iot` routes (find the section with `iot.HandleFunc`):
```go
// Firmware management
iot.HandleFunc("/firmware", a.listFirmware).Methods(http.MethodGet)
iot.HandleFunc("/firmware", a.uploadFirmware).Methods(http.MethodPost)
iot.HandleFunc("/firmware/{id}", a.deleteFirmware).Methods(http.MethodDelete)
iot.HandleFunc("/firmware/{id}/phase", a.updateFirmwarePhase).Methods(http.MethodPut)
```

Note: `iot` sub-router is prefixed with `/api/device` — check `api.go` router setup. If `iot` is the `/api/device` sub-router, use a separate sub-router or use the main router with the `/api/firmware` prefix. Looking at the route table, firmware should be at `/api/firmware`, NOT under `/api/device`. Register on the `dash` or a new sub-router with `/api` prefix.

Actual registration should be:
```go
// After dash sub-router setup:
a.r.HandleFunc("/api/firmware", a.listFirmware).Methods(http.MethodGet)
a.r.HandleFunc("/api/firmware", a.uploadFirmware).Methods(http.MethodPost)
a.r.HandleFunc("/api/firmware/{id}", a.deleteFirmware).Methods(http.MethodDelete)
a.r.HandleFunc("/api/firmware/{id}/phase", a.updateFirmwarePhase).Methods(http.MethodPut)
// Apply JWT middleware to these routes by wrapping or using a sub-router
```

The cleanest approach: add a `firmware` sub-router parallel to `iot` and `dash`:
```go
firmware := a.r.PathPrefix("/api/firmware").Subrouter()
firmware.Use(middleware.Middleware)
firmware.HandleFunc("", a.listFirmware).Methods(http.MethodGet)
firmware.HandleFunc("", a.uploadFirmware).Methods(http.MethodPost)
firmware.HandleFunc("/{id}", a.deleteFirmware).Methods(http.MethodDelete)
firmware.HandleFunc("/{id}/phase", a.updateFirmwarePhase).Methods(http.MethodPut)
```

**Step 3: Build to verify**
```bash
cd backend/services/controller && go build ./...
```

**Step 4: Commit**
```bash
git add backend/services/controller/internal/api/firmware.go backend/services/controller/internal/api/api.go
git commit -m "feat(firmware): add firmware REST API endpoints (list, upload, delete, phase)"
```

---

## Task A4: Nginx and compose wiring for firmware

**Files:**
- Modify: `deploy/compose/nginx.conf`
- Modify: `deploy/compose/docker-compose.yaml`
- Create: `deploy/compose/.env.firmware-upload`

**Step 1: Add nginx routes**

In `deploy/compose/nginx.conf`, add two location blocks (after the `/images` block):
```nginx
location /firmwares {
    proxy_pass http://host.docker.internal:8004;
    proxy_set_header Host $host;
    proxy_read_timeout 300s;
}

location /api/firmware/upload {
    proxy_pass http://host.docker.internal:8006/upload;
    proxy_set_header Host $host;
    client_max_body_size 500m;
    proxy_read_timeout 300s;
    proxy_request_buffering off;
}

location /api/firmware {
    proxy_pass http://host.docker.internal:8000;
    proxy_set_header Host $host;
    limit_req zone=api_limit burst=20 nodelay;
}
```

Note: The `/api/firmware/upload` (POST) goes to the upload service (port 8006), while GET/DELETE/PUT for firmware metadata goes to the controller (port 8000). This means the upload action in the frontend must POST to `/api/firmware/upload` and the metadata CRUD uses `/api/firmware`. Alternatively, keep all on the controller and have the controller proxy to the upload service — this is what `firmware.go` handler does above. In that case, only add the `/api/firmware` block pointing to port 8000, and `client_max_body_size 500m`.

**Recommended approach** (simpler): Keep everything through the controller. Only add:
```nginx
location /api/firmware {
    proxy_pass http://host.docker.internal:8000;
    client_max_body_size 500m;
    proxy_read_timeout 300s;
    proxy_set_header Host $host;
}

location /firmwares {
    proxy_pass http://host.docker.internal:8004;
    proxy_set_header Host $host;
}
```

**Step 2: Add firmware-upload service to docker-compose.yaml**

Add the service entry (following the `container-upload` pattern):
```yaml
firmware-upload:
  build:
    context: ../backend/services/utils/firmware-upload
    dockerfile: build/Dockerfile
  env_file:
    - .env.firmware-upload
  networks:
    oktopus:
      ipv4_address: 172.16.235.21
  volumes:
    - ./firmwares:/app/firmwares
```

**Step 3: Create env file**

Create `deploy/compose/.env.firmware-upload`:
```
SERVER_PORT=8006
FIRMWARE_DIR=/app/firmwares
```

**Step 4: Update controller env to know the firmware-upload service hostname**

In `deploy/compose/.env.controller`, add:
```
FIRMWARE_UPLOAD_URL=http://firmware-upload:8006
```

Then update the constant in `firmware.go` to use `os.Getenv("FIRMWARE_UPLOAD_URL")` with fallback.

**Step 5: Commit**
```bash
git add deploy/compose/nginx.conf deploy/compose/docker-compose.yaml deploy/compose/.env.firmware-upload
git commit -m "feat(firmware): add nginx routing and docker-compose service for firmware upload"
```

---

## Task A5: Frontend Firmware page

**Files:**
- Create: `frontend/src/pages/firmware.js`
- Create: `frontend/src/sections/firmware/firmware-table.js`
- Create: `frontend/src/sections/firmware/firmware-upload-dialog.js`
- Modify: `frontend/src/layouts/dashboard/config.js`

**Context:** Pages follow the pattern in `frontend/src/pages/containers-store.js` (best reference — it has file upload, table, dialogs, delete confirmations). Sections follow the pattern of existing sections in `frontend/src/sections/`. Use `useBackendContext().httpRequest`. All icons from `@heroicons/react/24/outline` wrapped in `SvgIcon`.

**Step 1: Create the upload dialog component**

Create `frontend/src/sections/firmware/firmware-upload-dialog.js`:
```jsx
import { useState, useRef } from 'react';
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, Stack, FormControl, InputLabel,
  Select, MenuItem, LinearProgress, Typography, Box
} from '@mui/material';
import { useBackendContext } from '../../contexts/backend-context';

export const FirmwareUploadDialog = ({ open, onClose, onSuccess }) => {
  const { httpRequest, setAlert } = useBackendContext();
  const [loading, setLoading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [fields, setFields] = useState({
    name: '', build_version: '', phase: 'internal_testing', download_url: ''
  });
  const [file, setFile] = useState(null);
  const fileRef = useRef();

  const handleSubmit = async () => {
    if (!fields.name || !fields.build_version) {
      setAlert({ severity: 'error', message: 'Name and build version are required' });
      return;
    }
    if (!file && !fields.download_url) {
      setAlert({ severity: 'error', message: 'Upload a file or provide a download URL' });
      return;
    }
    setLoading(true);
    const formData = new FormData();
    formData.append('name', fields.name);
    formData.append('build_version', fields.build_version);
    formData.append('phase', fields.phase);
    if (file) formData.append('file', file);
    if (fields.download_url) formData.append('download_url', fields.download_url);

    // Use fetch directly for multipart (httpRequest uses JSON)
    const token = localStorage.getItem('token');
    try {
      const resp = await fetch('/api/firmware', {
        method: 'POST',
        headers: { Authorization: token },
        body: formData,
      });
      if (!resp.ok) {
        const msg = await resp.text();
        setAlert({ severity: 'error', message: msg });
      } else {
        setAlert({ severity: 'success', message: 'Firmware uploaded' });
        onSuccess();
        onClose();
      }
    } catch (e) {
      setAlert({ severity: 'error', message: e.message });
    }
    setLoading(false);
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Upload Firmware</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField label="Name" value={fields.name} onChange={e => setFields(f => ({...f, name: e.target.value}))} required />
          <TextField label="Build Version" value={fields.build_version} onChange={e => setFields(f => ({...f, build_version: e.target.value}))} required />
          <FormControl>
            <InputLabel>Phase</InputLabel>
            <Select value={fields.phase} label="Phase" onChange={e => setFields(f => ({...f, phase: e.target.value}))}>
              <MenuItem value="internal_testing">Internal Testing</MenuItem>
              <MenuItem value="release">Release</MenuItem>
            </Select>
          </FormControl>
          <Button variant="outlined" onClick={() => fileRef.current.click()}>
            {file ? file.name : 'Select Firmware File (.bin)'}
          </Button>
          <input ref={fileRef} type="file" accept=".bin,.BIN" hidden onChange={e => setFile(e.target.files[0])} />
          <Typography variant="caption" color="text.secondary">— or provide download URL —</Typography>
          <TextField label="Download URL (optional)" value={fields.download_url} onChange={e => setFields(f => ({...f, download_url: e.target.value}))} />
          {loading && <LinearProgress />}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={loading}>Cancel</Button>
        <Button onClick={handleSubmit} variant="contained" disabled={loading}>Upload</Button>
      </DialogActions>
    </Dialog>
  );
};
```

**Step 2: Create the firmware table section**

Create `frontend/src/sections/firmware/firmware-table.js`:
```jsx
import { useState } from 'react';
import {
  Card, Table, TableBody, TableCell, TableContainer,
  TableHead, TableRow, IconButton, Chip, Tooltip, Box,
  CardHeader, Button, SvgIcon
} from '@mui/material';
import { TrashIcon, ArrowUpCircleIcon } from '@heroicons/react/24/outline';

const phaseColor = { internal_testing: 'warning', release: 'success' };
const phaseLabel = { internal_testing: 'Internal Testing', release: 'Release' };

export const FirmwareTable = ({ items, onDelete, onPhaseChange, onUpload }) => {
  return (
    <Card>
      <CardHeader
        title="Firmware Images"
        action={
          <Button variant="contained" onClick={onUpload}
            startIcon={<SvgIcon fontSize="small"><ArrowUpCircleIcon /></SvgIcon>}>
            Upload
          </Button>
        }
      />
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Build Version</TableCell>
              <TableCell>File Size</TableCell>
              <TableCell>Fingerprint (MD5)</TableCell>
              <TableCell>Phase</TableCell>
              <TableCell>Download URL</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {items.map(fw => (
              <TableRow key={fw.id} hover>
                <TableCell>{fw.name}</TableCell>
                <TableCell>{fw.build_version}</TableCell>
                <TableCell>{fw.file_size ? `${(fw.file_size / 1024 / 1024).toFixed(2)} MB` : '—'}</TableCell>
                <TableCell sx={{ fontFamily: 'monospace', fontSize: 12 }}>{fw.fingerprint || '—'}</TableCell>
                <TableCell>
                  <Chip
                    label={phaseLabel[fw.phase] || fw.phase}
                    color={phaseColor[fw.phase] || 'default'}
                    size="small"
                    onClick={() => onPhaseChange(fw.id, fw.phase === 'internal_testing' ? 'release' : 'internal_testing')}
                    sx={{ cursor: 'pointer' }}
                  />
                </TableCell>
                <TableCell>
                  {fw.download_url
                    ? <a href={fw.download_url} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>{fw.download_url}</a>
                    : '—'}
                </TableCell>
                <TableCell>{fw.created_at ? new Date(fw.created_at).toLocaleDateString() : '—'}</TableCell>
                <TableCell align="right">
                  <Tooltip title="Delete">
                    <IconButton color="error" onClick={() => onDelete(fw.id)}>
                      <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
                    </IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
            {items.length === 0 && (
              <TableRow><TableCell colSpan={8} align="center">No firmware images uploaded yet.</TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Card>
  );
};
```

**Step 3: Create the firmware page**

Create `frontend/src/pages/firmware.js`:
```jsx
import { useEffect, useState, useCallback } from 'react';
import Head from 'next/head';
import { Box, Container } from '@mui/material';
import { DashboardLayout } from '../layouts/dashboard/layout';
import { FirmwareTable } from '../sections/firmware/firmware-table';
import { FirmwareUploadDialog } from '../sections/firmware/firmware-upload-dialog';
import { useBackendContext } from '../contexts/backend-context';

const FirmwarePage = () => {
  const { httpRequest, setAlert } = useBackendContext();
  const [items, setItems] = useState([]);
  const [uploadOpen, setUploadOpen] = useState(false);

  const loadFirmware = useCallback(async () => {
    const { status, result } = await httpRequest('/api/firmware', 'GET');
    if (status === 200) setItems(result || []);
  }, [httpRequest]);

  useEffect(() => { loadFirmware(); }, [loadFirmware]);

  const handleDelete = async (id) => {
    const { status } = await httpRequest(`/api/firmware/${id}`, 'DELETE');
    if (status === 204) {
      setAlert({ severity: 'success', message: 'Firmware deleted' });
      loadFirmware();
    }
  };

  const handlePhaseChange = async (id, newPhase) => {
    const { status } = await httpRequest(`/api/firmware/${id}/phase`, 'PUT', { phase: newPhase });
    if (status === 204) loadFirmware();
  };

  return (
    <>
      <Head><title>Firmware | Oktopus</title></Head>
      <Box component="main" sx={{ flexGrow: 1, py: 4 }}>
        <Container maxWidth="xl">
          <FirmwareTable
            items={items}
            onDelete={handleDelete}
            onPhaseChange={handlePhaseChange}
            onUpload={() => setUploadOpen(true)}
          />
        </Container>
      </Box>
      <FirmwareUploadDialog
        open={uploadOpen}
        onClose={() => setUploadOpen(false)}
        onSuccess={loadFirmware}
      />
    </>
  );
};

FirmwarePage.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;
export default FirmwarePage;
```

**Step 4: Add to sidebar nav**

In `frontend/src/layouts/dashboard/config.js`, find the disabled `'File Server'` item and add a Firmware entry before it:
```js
import { ServerIcon } from '@heroicons/react/24/outline';
// In items array, add:
{
  title: 'Firmware',
  path: '/firmware',
  icon: ServerIcon,
},
```

**Step 5: Commit**
```bash
git add frontend/src/pages/firmware.js frontend/src/sections/firmware/ frontend/src/layouts/dashboard/config.js
git commit -m "feat(firmware): add Firmware management page with upload, table, phase toggle"
```

---

---

# FEATURE B: Per-Device Dashboard

---

## Task B1: Device metrics MongoDB schema

**Files:**
- Create: `backend/services/controller/internal/db/metrics.go`
- Modify: `backend/services/controller/internal/db/db.go`

**Context:** Time-series data for CPU, RAM, ROM. The collection is in the `usp` database alongside `messages`. TTL index keeps data for 7 days (configurable). Each document represents one polling sample.

**Step 1: Create metrics.go**

Create `backend/services/controller/internal/db/metrics.go`:
```go
package db

import (
    "context"
    "time"

    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

type DeviceMetrics struct {
    ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    DeviceSerial string             `bson:"device_serial" json:"device_serial"`
    Timestamp    time.Time          `bson:"timestamp"     json:"timestamp"`
    CPUUsage     float64            `bson:"cpu_usage"     json:"cpu_usage"`     // percent 0-100
    MemFree      int64              `bson:"mem_free"      json:"mem_free"`       // KB
    MemTotal     int64              `bson:"mem_total"     json:"mem_total"`      // KB
    StorageFree  int64              `bson:"storage_free"  json:"storage_free"`  // KB
    StorageUsed  int64              `bson:"storage_used"  json:"storage_used"`  // KB
    StorageTotal int64              `bson:"storage_total" json:"storage_total"` // KB
}

func (d *Database) StoreDeviceMetrics(ctx context.Context, m DeviceMetrics) error {
    m.ID = primitive.NewObjectID()
    if m.Timestamp.IsZero() {
        m.Timestamp = time.Now()
    }
    _, err := d.metrics.InsertOne(ctx, m)
    return err
}

func (d *Database) GetDeviceMetricsHistory(ctx context.Context, serial string, since time.Time) ([]DeviceMetrics, error) {
    filter := bson.M{
        "device_serial": serial,
        "timestamp":     bson.M{"$gte": since},
    }
    opts := options.Find().
        SetSort(bson.D{{Key: "timestamp", Value: 1}}).
        SetLimit(500)
    cursor, err := d.metrics.Find(ctx, filter, opts)
    if err != nil {
        return nil, err
    }
    var results []DeviceMetrics
    cursor.All(ctx, &results)
    return results, nil
}

func (d *Database) GetLatestDeviceMetrics(ctx context.Context, serial string) (DeviceMetrics, error) {
    var m DeviceMetrics
    opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}})
    err := d.metrics.FindOne(ctx, bson.M{"device_serial": serial}, opts).Decode(&m)
    return m, err
}
```

**Step 2: Register collection in db.go**

Add to `Database` struct: `metrics *mongo.Collection`

In `NewDatabase`, add in the `usp` database section:
```go
d.metrics = uspDb.Collection("device_metrics")
// TTL index: 7 days
d.metrics.Indexes().CreateOne(d.ctx, mongo.IndexModel{
    Keys:    bson.D{{Key: "timestamp", Value: 1}},
    Options: options.Index().SetExpireAfterSeconds(604800),
})
// Query index
d.metrics.Indexes().CreateOne(d.ctx, mongo.IndexModel{
    Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "timestamp", Value: -1}},
})
```

**Step 3: Build**
```bash
cd backend/services/controller && go build ./...
```

**Step 4: Commit**
```bash
git add backend/services/controller/internal/db/metrics.go backend/services/controller/internal/db/db.go
git commit -m "feat(dashboard): add DeviceMetrics MongoDB schema for time-series performance data"
```

---

## Task B2: Device info and control API handlers

**Files:**
- Create: `backend/services/controller/internal/api/deviceinfo.go`
- Modify: `backend/services/controller/internal/api/api.go`

**Context:** USP GET for device metadata uses `sendUspMsg` with `usp_utils.NewGetMsg`. The existing `fwupdate.go` is the best reference for a handler that builds and sends a USP GET, parses the response, and returns JSON. The TR-181 paths needed:

- DeviceInfo metadata: `Device.DeviceInfo.` with `MaxDepth:1` or specific paths
- WiFi: `Device.WiFi.SSID.*`, `Device.WiFi.AccessPoint.*`, `Device.WiFi.Radio.*`
- Ethernet: `Device.IP.Interface.*`
- CPU: `Device.DeviceInfo.ProcessStatus.CPUUsage`
- RAM: `Device.DeviceInfo.MemoryStatus.Free`, `Device.DeviceInfo.MemoryStatus.Total`
- Storage: `Device.StorageService.1.LogicalVolume.*`
- Reboot: `Device.Reboot()`
- Factory Reset: `Device.FactoryReset()`
- Restart Agent: `Device.USPAgent.Restart()`

**Step 1: Create deviceinfo.go**

Create `backend/services/controller/internal/api/deviceinfo.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/gorilla/mux"

    "github.com/OktopUSP/oktopus/backend/services/controller/internal/db"
    "github.com/OktopUSP/oktopus/backend/services/controller/internal/usp/usp_msg"
    "github.com/OktopUSP/oktopus/backend/services/controller/internal/usp/usp_utils"
)

// GET /api/device/{sn}/{mtp}/info
// Returns DeviceInfo metadata: model, brand, chipset, software version
func (a *Api) deviceInfoGet(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewGetMsg(usp_msg.Get{
        ParamPaths: []string{
            "Device.DeviceInfo.Manufacturer",
            "Device.DeviceInfo.ModelName",
            "Device.DeviceInfo.HardwareVersion",
            "Device.DeviceInfo.SoftwareVersion",
            "Device.DeviceInfo.SerialNumber",
            "Device.DeviceInfo.ProductClass",
            "Device.DeviceInfo.Description",
        },
        MaxDepth: 1,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// GET /api/device/{sn}/{mtp}/wifi-usp
// Returns WiFi configuration for USP devices
func (a *Api) deviceWifiUspGet(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewGetMsg(usp_msg.Get{
        ParamPaths: []string{
            "Device.WiFi.Radio.",
            "Device.WiFi.SSID.",
            "Device.WiFi.AccessPoint.",
        },
        MaxDepth: 3,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// GET /api/device/{sn}/{mtp}/interfaces
// Returns IP interface data (Ethernet IPv4/IPv6)
func (a *Api) deviceInterfacesGet(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewGetMsg(usp_msg.Get{
        ParamPaths: []string{
            "Device.IP.Interface.",
        },
        MaxDepth: 3,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// GET /api/device/{sn}/{mtp}/performance
// Returns current CPU, RAM, storage metrics and stores them in MongoDB
func (a *Api) devicePerformanceGet(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewGetMsg(usp_msg.Get{
        ParamPaths: []string{
            "Device.DeviceInfo.ProcessStatus.CPUUsage",
            "Device.DeviceInfo.MemoryStatus.Free",
            "Device.DeviceInfo.MemoryStatus.Total",
            "Device.StorageService.1.LogicalVolume.",
        },
        MaxDepth: 2,
    })
    // sendUspMsg writes to w, but we also want to store the metrics
    // Use a response recorder to capture the response
    rec := &responseRecorder{header: make(http.Header), statusCode: 200}
    sendUspMsg(msg, sn, rec, a.nc, mtp)

    // Try to parse and store metrics
    if rec.statusCode == http.StatusOK {
        go a.storePerformanceMetrics(sn, rec.body)
    }

    // Write original response to client
    for k, v := range rec.header {
        w.Header()[k] = v
    }
    w.WriteHeader(rec.statusCode)
    w.Write(rec.body)
}

// GET /api/device/{sn}/metrics?since=1h
// Returns historical metrics from MongoDB
func (a *Api) deviceMetricsHistory(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn := vars["sn"]
    sinceStr := r.URL.Query().Get("since")
    since := time.Now().Add(-24 * time.Hour) // default 24h
    if sinceStr != "" {
        if hours, err := strconv.Atoi(sinceStr); err == nil {
            since = time.Now().Add(-time.Duration(hours) * time.Hour)
        }
    }
    metrics, err := a.db.GetDeviceMetricsHistory(r.Context(), sn, since)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(metrics)
}

// PUT /api/device/{sn}/{mtp}/reboot
func (a *Api) deviceReboot(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewOperateMsg(usp_msg.Operate{
        Command:  "Device.Reboot()",
        SendResp: true,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// PUT /api/device/{sn}/{mtp}/factory-reset
func (a *Api) deviceFactoryReset(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewOperateMsg(usp_msg.Operate{
        Command:  "Device.FactoryReset()",
        SendResp: true,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// PUT /api/device/{sn}/{mtp}/restart-agent
func (a *Api) deviceRestartAgent(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewOperateMsg(usp_msg.Operate{
        Command:  "Device.USPAgent.Restart()",
        SendResp: true,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}

// responseRecorder captures the response for dual-use (store + forward)
type responseRecorder struct {
    header     http.Header
    body       []byte
    statusCode int
}
func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) WriteHeader(code int)        { r.statusCode = code }
func (r *responseRecorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }

func (a *Api) storePerformanceMetrics(sn string, data []byte) {
    // Parse the USP GetResp JSON to extract metric values
    // The GetResp structure from sendUspMsg is the protobuf-to-JSON of usp_msg.GetResp
    var resp usp_msg.GetResp
    if err := json.Unmarshal(data, &resp); err != nil {
        return
    }
    m := db.DeviceMetrics{DeviceSerial: sn, Timestamp: time.Now()}
    for _, pathResult := range resp.ReqPathResults {
        for _, resolved := range pathResult.ResolvedPathResults {
            for k, v := range resolved.ResultParams {
                switch {
                case k == "CPUUsage":
                    if f, err := strconv.ParseFloat(v, 64); err == nil { m.CPUUsage = f }
                case k == "Free" && pathResult.RequestedPath == "Device.DeviceInfo.MemoryStatus.Free":
                    if i, err := strconv.ParseInt(v, 10, 64); err == nil { m.MemFree = i }
                case k == "Total":
                    if i, err := strconv.ParseInt(v, 10, 64); err == nil { m.MemTotal = i }
                }
            }
        }
    }
    a.db.StoreDeviceMetrics(r.Context(), m)
    // Note: r.Context() is not available here — use context.Background()
}
```

Note: `storePerformanceMetrics` has a bug — `r.Context()` is not accessible in a goroutine without `r`. Fix by passing a context:
```go
// In devicePerformanceGet:
ctx := r.Context()
go a.storePerformanceMetrics(ctx, sn, rec.body)

// Change signature:
func (a *Api) storePerformanceMetrics(ctx context.Context, sn string, data []byte) {
    // ...
    a.db.StoreDeviceMetrics(ctx, m)
}
```

**Step 2: Register routes in api.go**

Add to the `iot` sub-router registrations:
```go
iot.HandleFunc("/{sn}/{mtp}/info", a.deviceInfoGet).Methods(http.MethodGet)
iot.HandleFunc("/{sn}/{mtp}/wifi-usp", a.deviceWifiUspGet).Methods(http.MethodGet)
iot.HandleFunc("/{sn}/{mtp}/interfaces", a.deviceInterfacesGet).Methods(http.MethodGet)
iot.HandleFunc("/{sn}/{mtp}/performance", a.devicePerformanceGet).Methods(http.MethodGet)
iot.HandleFunc("/{sn}/metrics", a.deviceMetricsHistory).Methods(http.MethodGet)
iot.HandleFunc("/{sn}/{mtp}/reboot", a.deviceReboot).Methods(http.MethodPut)
iot.HandleFunc("/{sn}/{mtp}/factory-reset", a.deviceFactoryReset).Methods(http.MethodPut)
iot.HandleFunc("/{sn}/{mtp}/restart-agent", a.deviceRestartAgent).Methods(http.MethodPut)
```

**Step 3: Build**
```bash
cd backend/services/controller && go build ./...
```

**Step 4: Commit**
```bash
git add backend/services/controller/internal/api/deviceinfo.go backend/services/controller/internal/api/api.go
git commit -m "feat(dashboard): add device info, WiFi USP, interfaces, performance, and control endpoints"
```

---

## Task B3: Extend USP device detail page with new tabs

**Files:**
- Modify: `frontend/src/pages/devices/usp/[...id].js`
- Create: `frontend/src/sections/devices/usp/devices-overview.js`
- Create: `frontend/src/sections/devices/usp/devices-network.js`
- Create: `frontend/src/sections/devices/usp/devices-performance.js`

**Context:** The page at `frontend/src/pages/devices/usp/[...id].js` uses a `sectionHandler()` switch based on `router.query.id[1]`. Tabs are `<Tab value="discovery">` etc. with `onClick={() => router.push(...)}`. Add three new tabs: `overview`, `network`, `performance`. The `devices-history.js` section is the best reference for a section with data fetching and table rendering.

**Step 1: Add tabs to the page**

In `frontend/src/pages/devices/usp/[...id].js`, update the `sectionHandler` switch:
```js
case "overview":     return <DevicesOverview/>
case "network":      return <DevicesNetwork/>
case "performance":  return <DevicesPerformance/>
```

Add imports at the top:
```js
import { DevicesOverview }    from '../../../sections/devices/usp/devices-overview';
import { DevicesNetwork }     from '../../../sections/devices/usp/devices-network';
import { DevicesPerformance } from '../../../sections/devices/usp/devices-performance';
```

Add the new `Tab` components in the `Tabs` group (keeping existing tabs):
```jsx
<Tab label="Overview"     value="overview"     onClick={() => router.push(`/devices/usp/${deviceID}/overview`)} />
<Tab label="Network"      value="network"      onClick={() => router.push(`/devices/usp/${deviceID}/network`)} />
<Tab label="Performance"  value="performance"  onClick={() => router.push(`/devices/usp/${deviceID}/performance`)} />
```

Also update the default redirect in `sectionHandler` to `overview` instead of `discovery`, or keep as is.

**Step 2: Create devices-overview.js**

Create `frontend/src/sections/devices/usp/devices-overview.js`:
```jsx
import { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import {
  Card, CardHeader, CardContent, Divider, Grid,
  Typography, Stack, Button, SvgIcon, Backdrop, CircularProgress, Chip
} from '@mui/material';
import {
  ArrowPathIcon, PowerIcon, TrashIcon, WrenchScrewdriverIcon
} from '@heroicons/react/24/outline';
import { useBackendContext } from '../../../contexts/backend-context';

const InfoRow = ({ label, value }) => (
  <Stack direction="row" justifyContent="space-between" sx={{ py: 0.5 }}>
    <Typography variant="body2" color="text.secondary">{label}</Typography>
    <Typography variant="body2" fontWeight={500}>{value || '—'}</Typography>
  </Stack>
);

export const DevicesOverview = () => {
  const router = useRouter();
  const deviceID = router.query.id?.[0];
  const { httpRequest, setAlert } = useBackendContext();
  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);

  const fetchInfo = async () => {
    if (!deviceID) return;
    setLoading(true);
    const { status, result } = await httpRequest(`/api/device/${deviceID}/any/info`, 'GET');
    if (status === 200) setInfo(result);
    setLoading(false);
  };

  useEffect(() => { fetchInfo(); }, [deviceID]);

  const doAction = async (endpoint, label) => {
    setActionLoading(true);
    const { status } = await httpRequest(`/api/device/${deviceID}/any/${endpoint}`, 'PUT', {});
    if (status === 200) setAlert({ severity: 'success', message: `${label} command sent` });
    setActionLoading(false);
  };

  // Parse USP GetResp to extract flat params
  const params = {};
  if (info?.req_path_results) {
    info.req_path_results.forEach(pr => {
      pr.resolved_path_results?.forEach(rr => {
        Object.entries(rr.result_params || {}).forEach(([k, v]) => { params[k] = v; });
      });
    });
  }

  return (
    <>
      <Backdrop open={loading || actionLoading} sx={{ zIndex: 9999 }}>
        <CircularProgress color="inherit" />
      </Backdrop>
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <Card>
            <CardHeader title="Device Metadata" action={
              <Button size="small" onClick={fetchInfo}
                startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}>
                Refresh
              </Button>
            }/>
            <Divider />
            <CardContent>
              <InfoRow label="Serial Number"     value={params['SerialNumber']} />
              <InfoRow label="Manufacturer"      value={params['Manufacturer']} />
              <InfoRow label="Model"             value={params['ModelName']} />
              <InfoRow label="Hardware Version"  value={params['HardwareVersion']} />
              <InfoRow label="Software Version"  value={params['SoftwareVersion']} />
              <InfoRow label="Product Class"     value={params['ProductClass']} />
              <InfoRow label="Description"       value={params['Description']} />
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={6}>
          <Card>
            <CardHeader title="Actions" />
            <Divider />
            <CardContent>
              <Stack spacing={1}>
                <Button variant="outlined" fullWidth onClick={() => doAction('reboot', 'Reboot')}
                  startIcon={<SvgIcon><PowerIcon /></SvgIcon>}>
                  Reboot
                </Button>
                <Button variant="outlined" fullWidth onClick={() => doAction('restart-agent', 'Restart Agent')}
                  startIcon={<SvgIcon><WrenchScrewdriverIcon /></SvgIcon>}>
                  Restart Agent
                </Button>
                <Button variant="outlined" color="error" fullWidth onClick={() => doAction('factory-reset', 'Factory Reset')}
                  startIcon={<SvgIcon><TrashIcon /></SvgIcon>}>
                  Factory Reset
                </Button>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </>
  );
};
```

**Step 3: Create devices-network.js**

Create `frontend/src/sections/devices/usp/devices-network.js`:
```jsx
import { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import {
  Card, CardHeader, CardContent, Divider, Tab, Tabs, Box,
  Table, TableBody, TableCell, TableHead, TableRow, Chip,
  Button, SvgIcon, Backdrop, CircularProgress, Typography
} from '@mui/material';
import { ArrowPathIcon } from '@heroicons/react/24/outline';
import { useBackendContext } from '../../../contexts/backend-context';

export const DevicesNetwork = () => {
  const router = useRouter();
  const deviceID = router.query.id?.[0];
  const { httpRequest } = useBackendContext();
  const [tab, setTab] = useState('wifi');
  const [wifiData, setWifiData] = useState(null);
  const [ethData, setEthData] = useState(null);
  const [loading, setLoading] = useState(false);

  const fetchAll = async () => {
    if (!deviceID) return;
    setLoading(true);
    const [wifiRes, ethRes] = await Promise.all([
      httpRequest(`/api/device/${deviceID}/any/wifi-usp`, 'GET'),
      httpRequest(`/api/device/${deviceID}/any/interfaces`, 'GET'),
    ]);
    if (wifiRes.status === 200) setWifiData(wifiRes.result);
    if (ethRes.status === 200) setEthData(ethRes.result);
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, [deviceID]);

  // Helper: flatten GetResp into path→params map
  const flattenGetResp = (resp) => {
    const result = {};
    resp?.req_path_results?.forEach(pr => {
      pr.resolved_path_results?.forEach(rr => {
        result[rr.resolved_path] = rr.result_params || {};
      });
    });
    return result;
  };

  const wifiFlat = flattenGetResp(wifiData);
  const ethFlat  = flattenGetResp(ethData);

  return (
    <>
      <Backdrop open={loading} sx={{ zIndex: 9999 }}><CircularProgress color="inherit" /></Backdrop>
      <Card>
        <CardHeader title="Network Interfaces" action={
          <Button size="small" onClick={fetchAll}
            startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}>Refresh</Button>
        }/>
        <Divider />
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ px: 2 }}>
          <Tab label="Wi-Fi" value="wifi" />
          <Tab label="Ethernet / IP" value="ethernet" />
        </Tabs>
        <Divider />
        <CardContent>
          {tab === 'wifi' && (
            Object.keys(wifiFlat).length === 0
              ? <Typography color="text.secondary">No Wi-Fi data. Click Refresh.</Typography>
              : <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Path</TableCell>
                      <TableCell>Parameter</TableCell>
                      <TableCell>Value</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {Object.entries(wifiFlat).map(([path, params]) =>
                      Object.entries(params).map(([k, v]) => (
                        <TableRow key={path+k}>
                          <TableCell sx={{ fontSize: 11, fontFamily: 'monospace' }}>{path}</TableCell>
                          <TableCell>{k}</TableCell>
                          <TableCell>{String(v)}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
          )}
          {tab === 'ethernet' && (
            Object.keys(ethFlat).length === 0
              ? <Typography color="text.secondary">No interface data. Click Refresh.</Typography>
              : <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Interface</TableCell>
                      <TableCell>Parameter</TableCell>
                      <TableCell>Value</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {Object.entries(ethFlat).map(([path, params]) =>
                      Object.entries(params).map(([k, v]) => (
                        <TableRow key={path+k}>
                          <TableCell sx={{ fontSize: 11, fontFamily: 'monospace' }}>{path}</TableCell>
                          <TableCell>{k}</TableCell>
                          <TableCell>{String(v)}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
          )}
        </CardContent>
      </Card>
    </>
  );
};
```

**Step 4: Create devices-performance.js**

Create `frontend/src/sections/devices/usp/devices-performance.js`:
```jsx
import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/router';
import {
  Card, CardHeader, CardContent, Divider, Grid,
  Typography, Stack, LinearProgress, Button, SvgIcon,
  Backdrop, CircularProgress, Box, Select, MenuItem, FormControl, InputLabel
} from '@mui/material';
import { ArrowPathIcon } from '@heroicons/react/24/outline';
import { Chart } from '../../../components/chart';
import { useBackendContext } from '../../../contexts/backend-context';
import { useTheme } from '@mui/material/styles';

const GaugeStat = ({ label, value, max = 100, unit = '%', color = 'primary' }) => {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  return (
    <Box>
      <Stack direction="row" justifyContent="space-between">
        <Typography variant="body2" color="text.secondary">{label}</Typography>
        <Typography variant="body2" fontWeight={600}>{value != null ? `${value}${unit}` : '—'}</Typography>
      </Stack>
      <LinearProgress
        variant="determinate"
        value={isNaN(pct) ? 0 : pct}
        color={pct > 90 ? 'error' : pct > 70 ? 'warning' : color}
        sx={{ height: 8, borderRadius: 4, mt: 0.5 }}
      />
    </Box>
  );
};

export const DevicesPerformance = () => {
  const router = useRouter();
  const deviceID = router.query.id?.[0];
  const { httpRequest } = useBackendContext();
  const theme = useTheme();
  const [current, setCurrent] = useState(null);
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(false);
  const [timeRange, setTimeRange] = useState('24');

  const fetchData = useCallback(async () => {
    if (!deviceID) return;
    setLoading(true);
    const [perfRes, histRes] = await Promise.all([
      httpRequest(`/api/device/${deviceID}/any/performance`, 'GET'),
      httpRequest(`/api/device/${deviceID}/metrics?since=${timeRange}`, 'GET'),
    ]);
    if (perfRes.status === 200) setCurrent(perfRes.result);
    if (histRes.status === 200) setHistory(histRes.result || []);
    setLoading(false);
  }, [deviceID, timeRange, httpRequest]);

  useEffect(() => { fetchData(); }, [fetchData]);

  // Parse current metrics from USP GetResp
  const params = {};
  current?.req_path_results?.forEach(pr => {
    pr.resolved_path_results?.forEach(rr => {
      Object.entries(rr.result_params || {}).forEach(([k, v]) => { params[k] = v; });
    });
  });
  const cpuUsage = parseFloat(params['CPUUsage']) || 0;
  const memFree = parseInt(params['Free']) || 0;
  const memTotal = parseInt(params['Total']) || 0;
  const memUsed = memTotal > 0 ? memTotal - memFree : 0;

  // Chart data from history
  const timestamps = history.map(m => new Date(m.timestamp).toLocaleTimeString());
  const cpuSeries = history.map(m => parseFloat(m.cpu_usage?.toFixed(1)) || 0);
  const memSeries = history.map(m => {
    const used = m.mem_total > 0 ? m.mem_total - m.mem_free : 0;
    return m.mem_total > 0 ? parseFloat(((used / m.mem_total) * 100).toFixed(1)) : 0;
  });

  const chartOptions = {
    chart: { toolbar: { show: false }, zoom: { enabled: false } },
    xaxis: { categories: timestamps, labels: { show: timestamps.length < 50 } },
    colors: [theme.palette.primary.main, theme.palette.warning.main],
    stroke: { curve: 'smooth', width: 2 },
    legend: { position: 'top' },
    tooltip: { x: { show: true } },
    yaxis: { min: 0, max: 100, labels: { formatter: v => `${v}%` } },
  };

  return (
    <>
      <Backdrop open={loading} sx={{ zIndex: 9999 }}><CircularProgress color="inherit" /></Backdrop>
      <Grid container spacing={2}>
        <Grid item xs={12} md={4}>
          <Card sx={{ height: '100%' }}>
            <CardHeader title="Current Metrics" action={
              <Button size="small" onClick={fetchData}
                startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}>Refresh</Button>
            }/>
            <Divider />
            <CardContent>
              <Stack spacing={2}>
                <GaugeStat label="CPU Usage" value={cpuUsage} unit="%" />
                <GaugeStat label="RAM Used"
                  value={memUsed > 0 ? Math.round(memUsed / 1024) : null}
                  max={memTotal > 0 ? Math.round(memTotal / 1024) : 100}
                  unit=" MB" color="warning" />
                <Typography variant="caption" color="text.secondary">
                  RAM: {memTotal > 0 ? `${Math.round(memFree/1024)} MB free / ${Math.round(memTotal/1024)} MB total` : 'N/A'}
                </Typography>
              </Stack>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={8}>
          <Card>
            <CardHeader
              title="Historical Performance"
              action={
                <FormControl size="small" sx={{ minWidth: 100 }}>
                  <Select value={timeRange} onChange={e => setTimeRange(e.target.value)}>
                    <MenuItem value="1">Last 1h</MenuItem>
                    <MenuItem value="6">Last 6h</MenuItem>
                    <MenuItem value="24">Last 24h</MenuItem>
                    <MenuItem value="168">Last 7d</MenuItem>
                  </Select>
                </FormControl>
              }
            />
            <Divider />
            <CardContent>
              {history.length === 0
                ? <Typography color="text.secondary">No historical data. Refresh device to collect metrics.</Typography>
                : <Chart
                    height={280}
                    type="line"
                    options={chartOptions}
                    series={[
                      { name: 'CPU %', data: cpuSeries },
                      { name: 'RAM %', data: memSeries },
                    ]}
                    width="100%"
                  />
              }
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </>
  );
};
```

**Step 5: Commit**
```bash
git add frontend/src/pages/devices/usp/[...id].js frontend/src/sections/devices/usp/devices-overview.js frontend/src/sections/devices/usp/devices-network.js frontend/src/sections/devices/usp/devices-performance.js
git commit -m "feat(dashboard): add Overview, Network, Performance tabs to USP device detail page"
```

---

---

# FEATURE C: Network Topology

---

## Task C1: Topology backend handler

**Files:**
- Create: `backend/services/controller/internal/api/topology.go`
- Modify: `backend/services/controller/internal/api/api.go`

**Context:** Network topology requires querying TR-181 paths for connected clients and Ethernet interface stats. The handler follows the same pattern as `deviceinfo.go`. Two USP GETs are needed:
1. `Device.WiFi.AccessPoint.*.AssociatedDevice.` — WiFi connected clients
2. `Device.Hosts.Host.` — all hosts (wired + wireless)
3. `Device.Ethernet.Interface.` — Ethernet interfaces with link status

**Step 1: Create topology.go**

Create `backend/services/controller/internal/api/topology.go`:
```go
package api

import (
    "net/http"

    "github.com/gorilla/mux"

    "github.com/OktopUSP/oktopus/backend/services/controller/internal/usp/usp_msg"
    "github.com/OktopUSP/oktopus/backend/services/controller/internal/usp/usp_utils"
)

// GET /api/device/{sn}/{mtp}/topology
// Returns associated devices (WiFi clients), hosts, and Ethernet interface status
func (a *Api) deviceTopology(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    sn, mtp := vars["sn"], vars["mtp"]
    msg := usp_utils.NewGetMsg(usp_msg.Get{
        ParamPaths: []string{
            "Device.WiFi.AccessPoint.",
            "Device.Hosts.Host.",
            "Device.Ethernet.Interface.",
        },
        MaxDepth: 3,
    })
    sendUspMsg(msg, sn, w, a.nc, mtp)
}
```

**Step 2: Register route in api.go**

Add to the `iot` sub-router:
```go
iot.HandleFunc("/{sn}/{mtp}/topology", a.deviceTopology).Methods(http.MethodGet)
```

**Step 3: Build**
```bash
cd backend/services/controller && go build ./...
```

**Step 4: Commit**
```bash
git add backend/services/controller/internal/api/topology.go backend/services/controller/internal/api/api.go
git commit -m "feat(topology): add network topology USP GET endpoint"
```

---

## Task C2: Frontend topology section

**Files:**
- Create: `frontend/src/sections/devices/usp/devices-topology.js`
- Modify: `frontend/src/pages/devices/usp/[...id].js`

**Context:** The topology view displays connected devices as a tree/table. A proper graph library (e.g. `react-flow`) would be ideal but adds a dependency. Use a simple tree-table approach with MUI components first. The data comes from the USP GetResp for `Device.Hosts.Host.*` and `Device.WiFi.AccessPoint.*.AssociatedDevice.*`.

**Step 1: Create devices-topology.js**

Create `frontend/src/sections/devices/usp/devices-topology.js`:
```jsx
import { useEffect, useState } from 'react';
import { useRouter } from 'next/router';
import {
  Card, CardHeader, CardContent, Divider, Button, SvgIcon,
  Table, TableBody, TableCell, TableHead, TableRow,
  Chip, Stack, Typography, FormControl, InputLabel, Select, MenuItem,
  Backdrop, CircularProgress, Box
} from '@mui/material';
import { ArrowPathIcon, WifiIcon, ComputerDesktopIcon } from '@heroicons/react/24/outline';
import { useBackendContext } from '../../../contexts/backend-context';

const flattenGetResp = (resp) => {
  const result = {};
  resp?.req_path_results?.forEach(pr => {
    pr.resolved_path_results?.forEach(rr => {
      result[rr.resolved_path] = rr.result_params || {};
    });
  });
  return result;
};

// Extract hosts from flat GetResp data
const extractHosts = (flat) => {
  const hosts = [];
  Object.entries(flat).forEach(([path, params]) => {
    if (path.startsWith('Device.Hosts.Host.')) {
      hosts.push({ path, ...params });
    }
  });
  return hosts;
};

const extractAssociated = (flat) => {
  const devices = [];
  Object.entries(flat).forEach(([path, params]) => {
    if (path.includes('.AssociatedDevice.')) {
      devices.push({ path, ...params });
    }
  });
  return devices;
};

const extractEthInterfaces = (flat) => {
  const ifaces = [];
  Object.entries(flat).forEach(([path, params]) => {
    if (path.startsWith('Device.Ethernet.Interface.')) {
      ifaces.push({ path, ...params });
    }
  });
  return ifaces;
};

export const DevicesTopology = () => {
  const router = useRouter();
  const deviceID = router.query.id?.[0];
  const { httpRequest } = useBackendContext();
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState('all');

  const fetchTopology = async () => {
    if (!deviceID) return;
    setLoading(true);
    const { status, result } = await httpRequest(`/api/device/${deviceID}/any/topology`, 'GET');
    if (status === 200) setData(result);
    setLoading(false);
  };

  useEffect(() => { fetchTopology(); }, [deviceID]);

  const flat = flattenGetResp(data);
  const hosts = extractHosts(flat);
  const associated = extractAssociated(flat);
  const ethIfaces = extractEthInterfaces(flat);

  const filteredHosts = hosts.filter(h => {
    if (filter === 'wired') return h.InterfaceType === 'Ethernet' || !h.InterfaceType?.includes('802.11');
    if (filter === 'wireless') return h.InterfaceType?.includes('802.11') || h.Layer1Interface?.includes('WiFi');
    return true;
  });

  return (
    <>
      <Backdrop open={loading} sx={{ zIndex: 9999 }}><CircularProgress color="inherit" /></Backdrop>
      <Stack spacing={2}>
        {/* Ethernet Interfaces */}
        <Card>
          <CardHeader title="Ethernet Interfaces" action={
            <Button size="small" onClick={fetchTopology}
              startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}>Synchronize</Button>
          }/>
          <Divider />
          <CardContent>
            {ethIfaces.length === 0
              ? <Typography color="text.secondary">No data. Click Synchronize.</Typography>
              : <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Interface</TableCell>
                      <TableCell>Status</TableCell>
                      <TableCell>Upstream</TableCell>
                      <TableCell>MaxBitRate</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {ethIfaces.map(iface => (
                      <TableRow key={iface.path}>
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: 11 }}>{iface.path}</TableCell>
                        <TableCell>
                          <Chip
                            label={iface.Status || '—'}
                            color={iface.Status === 'Up' ? 'success' : 'default'}
                            size="small"
                          />
                        </TableCell>
                        <TableCell>{iface.Upstream === 'true' ? 'Yes' : 'No'}</TableCell>
                        <TableCell>{iface.MaxBitRate ? `${iface.MaxBitRate} Mbps` : '—'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
            }
          </CardContent>
        </Card>

        {/* Connected Clients */}
        <Card>
          <CardHeader
            title="Connected Clients"
            action={
              <Stack direction="row" spacing={1} alignItems="center">
                <FormControl size="small" sx={{ minWidth: 120 }}>
                  <Select value={filter} onChange={e => setFilter(e.target.value)}>
                    <MenuItem value="all">All Devices</MenuItem>
                    <MenuItem value="wired">Wired</MenuItem>
                    <MenuItem value="wireless">Wireless</MenuItem>
                  </Select>
                </FormControl>
              </Stack>
            }
          />
          <Divider />
          <CardContent>
            {filteredHosts.length === 0
              ? <Typography color="text.secondary">No connected clients found.</Typography>
              : <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Hostname</TableCell>
                      <TableCell>IP Address</TableCell>
                      <TableCell>MAC Address</TableCell>
                      <TableCell>Interface</TableCell>
                      <TableCell>Active</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {filteredHosts.map(host => (
                      <TableRow key={host.path}>
                        <TableCell>
                          <Stack direction="row" spacing={1} alignItems="center">
                            <SvgIcon fontSize="small" color={host.Layer1Interface?.includes('WiFi') ? 'primary' : 'action'}>
                              {host.Layer1Interface?.includes('WiFi') ? <WifiIcon /> : <ComputerDesktopIcon />}
                            </SvgIcon>
                            <Typography variant="body2">{host.HostName || host.PhysAddress || '—'}</Typography>
                          </Stack>
                        </TableCell>
                        <TableCell>{host.IPAddress || '—'}</TableCell>
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: 11 }}>{host.PhysAddress || '—'}</TableCell>
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: 11 }}>{host.Layer1Interface || '—'}</TableCell>
                        <TableCell>
                          <Chip
                            label={host.Active === 'true' ? 'Active' : 'Inactive'}
                            color={host.Active === 'true' ? 'success' : 'default'}
                            size="small"
                          />
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
            }
          </CardContent>
        </Card>

        {/* WiFi Associated Devices */}
        {associated.length > 0 && (
          <Card>
            <CardHeader title="WiFi Associated Devices" />
            <Divider />
            <CardContent>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>MAC Address</TableCell>
                    <TableCell>IP Address</TableCell>
                    <TableCell>Signal Strength</TableCell>
                    <TableCell>Tx Rate</TableCell>
                    <TableCell>Rx Rate</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {associated.map(dev => (
                    <TableRow key={dev.path}>
                      <TableCell sx={{ fontFamily: 'monospace' }}>{dev.MACAddress || '—'}</TableCell>
                      <TableCell>{dev.IPAddress || '—'}</TableCell>
                      <TableCell>{dev.SignalStrength ? `${dev.SignalStrength} dBm` : '—'}</TableCell>
                      <TableCell>{dev.LastDataDownlinkRate ? `${Math.round(dev.LastDataDownlinkRate/1000)} Mbps` : '—'}</TableCell>
                      <TableCell>{dev.LastDataUplinkRate ? `${Math.round(dev.LastDataUplinkRate/1000)} Mbps` : '—'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}
      </Stack>
    </>
  );
};
```

**Step 2: Add tab to device page**

In `frontend/src/pages/devices/usp/[...id].js`, add:
```js
import { DevicesTopology } from '../../../sections/devices/usp/devices-topology';
// In sectionHandler:
case "topology": return <DevicesTopology/>
// In Tabs:
<Tab label="Topology" value="topology" onClick={() => router.push(`/devices/usp/${deviceID}/topology`)} />
```

**Step 3: Commit**
```bash
git add frontend/src/sections/devices/usp/devices-topology.js frontend/src/pages/devices/usp/[...id].js
git commit -m "feat(topology): add Network Topology tab with connected clients, WiFi devices, Ethernet interfaces"
```

---

# INFRASTRUCTURE TASKS

---

## Task I1: Nginx rate limit for new routes

**File:** `deploy/compose/nginx.conf`

**Context:** Check the existing `limit_req_zone` definition at the top of `nginx.conf`. The API limit zone is `api_limit`. New routes should inherit the same limits. Already handled in Task A4.

**Action:** Verify that `/api/firmware` uses `client_max_body_size 500m` and has no (or high) rate limit for the upload endpoint. No additional changes needed if A4 was done correctly.

---

## Task I2: Update MEMORY.md with architecture decisions

**File:** `/home/maksim/.claude/projects/-home-maksim-projects-oktopus/memory/MEMORY.md`

After completing all implementations, update the memory file with:
- New endpoints added
- New MongoDB collections: `firmware` (db: `general`), `device_metrics` (db: `usp`)
- New frontend pages: `/firmware`
- New frontend sections: `devices-overview`, `devices-network`, `devices-performance`, `devices-topology`
- New docker service: `firmware-upload` on port 8006

---

# IMPLEMENTATION SEQUENCE

Recommended order to avoid merge conflicts and ensure dependencies are met:

```
A1 → A2 → A3 → A4 → A5    (Firmware: schema → upload svc → API → infra → frontend)
B1 → B2 → B3               (Dashboard: metrics schema → API → frontend)
C1 → C2                    (Topology: API → frontend)
I1 → I2                    (Infrastructure cleanup)
```

All A tasks are independent of B/C tasks. B and C share the device detail page but touch different tabs.

---

# KNOWN LIMITATIONS AND DEFERRED WORK

1. **Performance metrics collection is on-demand only.** Background polling (cron-style) is not implemented. A periodic `time.Ticker` goroutine in the controller would push metrics to MongoDB — this can be a follow-up task.

2. **The `forwardFileUpload` function in firmware.go needs the multipart implementation.** The full implementation code is included in Task A3 — it must be used, not the placeholder.

3. **WiFi USP paths may differ per device vendor.** The TR-181 paths `Device.WiFi.Radio.*`, `Device.WiFi.SSID.*`, `Device.WiFi.AccessPoint.*` are standard, but actual devices may implement subsets. The frontend shows raw params — this is correct behavior for a first iteration.

4. **Topology graph visualization.** The current implementation uses tables. A visual graph (e.g. with `reactflow` or `vis-network`) would improve UX but requires additional package installation. Consider as a follow-up.

5. **Socket.IO real-time updates.** The socketio service currently only does WebRTC signaling. Wiring `device.v1.new` and other NATS events to frontend Socket.IO rooms for live device updates is a separate feature entirely.

6. **CWMP device detail tabs.** The new Overview/Network/Performance/Topology tabs are USP-only. CWMP equivalents would require separate handlers using `cwmpInteraction` — deferred.
