package infra_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot resolves via PROJECT_ROOT env var (for Docker) or relative path (for host).
var projectRoot = getProjectRoot()
var composeDir = filepath.Join(projectRoot, "deploy", "compose")

func getProjectRoot() string {
	if env := os.Getenv("PROJECT_ROOT"); env != "" {
		return env
	}
	return filepath.Join("..", "..")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", path, err)
	}
	return string(data)
}

// --- Docker Compose: Restart Policies ---

func TestDockerCompose_AllServicesHaveRestartPolicy(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "docker-compose.yaml"))

	// Simple YAML parsing: find service blocks and check for restart:
	// Services are at top-level indentation under "services:"
	inServices := false
	currentService := ""
	hasRestart := map[string]bool{}
	services := []string{}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		// Top-level key outside services (networks:, volumes:, etc.)
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' && strings.Contains(line, ":") {
			inServices = false
			continue
		}
		// Service name: exactly 2 or 4 spaces indent + name + colon
		if matched, _ := regexp.MatchString(`^  \w`, line); matched && strings.Contains(line, ":") {
			currentService = strings.TrimSpace(strings.Split(line, ":")[0])
			services = append(services, currentService)
		}
		if currentService != "" && strings.Contains(trimmed, "restart:") {
			hasRestart[currentService] = true
		}
	}

	for _, svc := range services {
		if svc == "registry-certs-generator" {
			continue // restart: "no" is valid for init containers
		}
		if !hasRestart[svc] {
			t.Errorf("Service %q has no restart policy", svc)
		}
	}
}

// --- Docker Compose: Internal Services Not Exposed to Host ---

func TestDockerCompose_InternalServicesNotExposedToHost(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "docker-compose.yaml"))

	// Services that should NOT have ports: mapped to host
	internalOnly := []string{"controller", "container-upload", "file-server", "socketio"}

	for _, svc := range internalOnly {
		// Find service block and check for "ports:" (not "expose:")
		svcPattern := regexp.MustCompile(`(?m)^\s{2}` + regexp.QuoteMeta(svc) + `:`)
		loc := svcPattern.FindStringIndex(content)
		if loc == nil {
			continue // service not found
		}

		// Find the next service or end of file
		nextSvc := regexp.MustCompile(`(?m)^\s{2}\w+:`)
		rest := content[loc[1]:]
		nextLoc := nextSvc.FindStringIndex(rest)
		var block string
		if nextLoc != nil {
			block = rest[:nextLoc[0]]
		} else {
			block = rest
		}

		if strings.Contains(block, "ports:") {
			t.Errorf("Service %q has ports: mapped to host -- should use expose: instead", svc)
		}
	}
}

// --- Docker Compose: Pinned Images ---

func TestDockerCompose_AllImagesPinned(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "docker-compose.yaml"))

	imageRe := regexp.MustCompile(`(?m)^\s+image:\s*(.+)`)
	matches := imageRe.FindAllStringSubmatch(content, -1)

	for _, m := range matches {
		img := strings.TrimSpace(m[1])
		img = strings.Trim(img, `"'`)
		// Skip locally-built images (oktopusp/* are built by run.sh, not pulled)
		if strings.HasPrefix(img, "oktopusp/") {
			continue
		}
		if strings.HasSuffix(img, ":latest") {
			t.Errorf("Image %q uses :latest tag -- should be pinned to a specific version", img)
		}
		// No tag at all (e.g., "mongo" without ":X")
		if !strings.Contains(img, ":") && !strings.Contains(img, "@") {
			t.Errorf("Image %q has no version tag -- should be pinned (e.g., %s:7.0)", img, img)
		}
	}
}

// --- Docker Compose: Health Checks ---

func TestDockerCompose_CriticalServicesHaveHealthcheck(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "docker-compose.yaml"))

	critical := []string{"mongo_usp", "msg_broker", "controller", "nginx"}

	for _, svc := range critical {
		svcPattern := regexp.MustCompile(`(?m)^\s{2}` + regexp.QuoteMeta(svc) + `:`)
		loc := svcPattern.FindStringIndex(content)
		if loc == nil {
			t.Errorf("Service %q not found in docker-compose.yaml", svc)
			continue
		}

		nextSvc := regexp.MustCompile(`(?m)^\s{2}\w+:`)
		rest := content[loc[1]:]
		nextLoc := nextSvc.FindStringIndex(rest)
		var block string
		if nextLoc != nil {
			block = rest[:nextLoc[0]]
		} else {
			block = rest
		}

		if !strings.Contains(block, "healthcheck:") {
			t.Errorf("Critical service %q has no healthcheck defined", svc)
		}
	}
}

// --- Docker Compose: No Hardcoded Secrets ---

func TestDockerCompose_NoHardcodedSecrets(t *testing.T) {
	knownDefaults := []string{"oktopususer", "oktopuspw", "supersecretkey"}

	// Only check deploy/compose/.env.* files (generated by generate-secrets.sh)
	envFiles, _ := filepath.Glob(filepath.Join(composeDir, ".env.*"))

	for _, envFile := range envFiles {
		// Skip .env.example
		if strings.HasSuffix(envFile, ".example") {
			continue
		}
		content := readFile(t, envFile)
		for _, secret := range knownDefaults {
			if strings.Contains(content, secret) {
				t.Errorf("File %s contains known default secret %q -- run generate-secrets.sh to regenerate", filepath.Base(envFile), secret)
			}
		}
	}
}

// --- Nginx: Body Size ---

func TestNginx_ApiBodySizeReasonable(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "nginx.conf"))

	// Check server-level (default) client_max_body_size -- indented with 8 spaces
	// Location-level overrides (e.g., container upload at 10G) are intentional
	serverRe := regexp.MustCompile(`(?m)^        client_max_body_size\s+(\S+);`)
	matches := serverRe.FindAllStringSubmatch(content, -1)

	for _, m := range matches {
		size := strings.TrimSpace(m[1])
		if size == "10G" || size == "10g" || size == "5G" || size == "5g" || size == "1G" || size == "1g" {
			t.Errorf("Server-level client_max_body_size %s is too large -- should be <=10M for API endpoints (per-location overrides are fine)", size)
		}
	}
}

// --- Nginx: Rate Limiting ---

func TestNginx_AllEndpointsHaveRateLimiting(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "nginx.conf"))

	// Endpoints that should have limit_req
	endpoints := []string{"/firmwares", "/images"}

	for _, ep := range endpoints {
		// Find location block for this endpoint
		locRe := regexp.MustCompile(`location\s+` + regexp.QuoteMeta(ep) + `\s*\{([^}]+)\}`)
		match := locRe.FindStringSubmatch(content)
		if match == nil {
			// Try simpler match
			idx := strings.Index(content, "location "+ep)
			if idx == -1 {
				continue
			}
			block := content[idx:min(idx+500, len(content))]
			if !strings.Contains(block, "limit_req") {
				t.Errorf("Location %s has no rate limiting (limit_req)", ep)
			}
			continue
		}
		if !strings.Contains(match[1], "limit_req") {
			t.Errorf("Location %s has no rate limiting (limit_req)", ep)
		}
	}
}

// --- Dockerfiles: USER directive ---

func TestDockerfiles_HaveUserDirective(t *testing.T) {
	// container-upload excluded: requires root for Docker socket access
	dockerfiles := []string{
		filepath.Join(projectRoot, "backend", "services", "controller", "build", "Dockerfile"),
		filepath.Join(projectRoot, "frontend", "build", "Dockerfile"),
		filepath.Join(projectRoot, "backend", "services", "utils", "firmware-upload", "build", "Dockerfile"),
	}

	for _, df := range dockerfiles {
		content := readFile(t, df)
		if !strings.Contains(content, "USER ") {
			t.Errorf("Dockerfile %s runs as root -- no USER directive found", df)
		}
	}
}

// --- Dockerfiles: No EOL Base Images ---

func TestDockerfiles_NoEOLAlpine(t *testing.T) {
	// Find all Dockerfiles and check for EOL Alpine versions
	eolAlpine := []string{"alpine:3.14", "alpine:3.15", "alpine:3.16", "alpine:3.17"}

	dockerfiles, _ := filepath.Glob(filepath.Join(projectRoot, "backend", "services", "*", "build", "Dockerfile"))
	mtpDockerfiles, _ := filepath.Glob(filepath.Join(projectRoot, "backend", "services", "mtp", "*", "build", "Dockerfile"))
	dockerfiles = append(dockerfiles, mtpDockerfiles...)

	for _, df := range dockerfiles {
		content := readFile(t, df)
		for _, eol := range eolAlpine {
			if strings.Contains(content, eol) {
				t.Errorf("Dockerfile %s uses EOL %s -- should use alpine:3.21 or later", df, eol)
			}
		}
	}
}

func TestDockerfiles_NoEOLNode(t *testing.T) {
	eolNode := []string{"node:14", "node:16", "node:18"}

	dockerfiles := []string{
		filepath.Join(projectRoot, "frontend", "build", "Dockerfile"),
		filepath.Join(projectRoot, "frontend", "build", "Dockerfile.dev"),
		filepath.Join(projectRoot, "backend", "services", "utils", "socketio", "build", "Dockerfile"),
		filepath.Join(projectRoot, "backend", "services", "utils", "firmware-upload", "build", "Dockerfile"),
		filepath.Join(composeDir, "container-upload-service", "Dockerfile"),
		filepath.Join(composeDir, "registry-certs-generator", "Dockerfile"),
	}

	for _, df := range dockerfiles {
		if _, err := os.Stat(df); err != nil {
			continue
		}
		content := readFile(t, df)
		for _, eol := range eolNode {
			// Match "node:16" or "node:16." or "node:18.18"
			if strings.Contains(content, "FROM "+eol) {
				t.Errorf("Dockerfile %s uses EOL %s -- should use node:22-alpine or later", df, eol)
			}
		}
	}
}

// --- CI: Has Test Step ---

func TestCircleCI_HasTestStep(t *testing.T) {
	content := readFile(t, filepath.Join(projectRoot, ".circleci", "config.yml"))

	if !strings.Contains(content, "go test") && !strings.Contains(content, "npm test") {
		t.Error(".circleci/config.yml has no test step -- CI only builds/pushes without running tests")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
