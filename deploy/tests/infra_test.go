package infra_test

import (
	"io/fs"
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

// --- Dockerfiles: consistent FROM/AS casing ---

func TestDockerfiles_FromAsCasing(t *testing.T) {
	err := filepath.WalkDir(projectRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(entry.Name(), "Dockerfile") {
			return nil
		}

		for lineNumber, line := range strings.Split(readFile(t, path), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[0] != "FROM" {
				continue
			}
			for _, field := range fields[1:] {
				if strings.EqualFold(field, "AS") && field != "AS" {
					t.Errorf("Dockerfile %s:%d uses %q; FROM and AS keywords must use consistent uppercase casing", path, lineNumber+1, field)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Dockerfiles: %v", err)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Source deploy safety gates ---

func TestCISourceDeploy_MigratesMissingControllerLockDefaults(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "ci-source-deploy.sh"))
	want := `merge_missing_env_defaults "$SCRIPT_DIR/.env.controller.example" "$SCRIPT_DIR/.env.controller" "LOCK_"`
	if !strings.Contains(content, want) {
		t.Fatalf("ci-source-deploy.sh must merge missing LOCK_* defaults into an existing .env.controller; missing %q", want)
	}
}

func TestGenerateSecrets_ControllerLockDefaultsMatchExample(t *testing.T) {
	example := readFile(t, filepath.Join(composeDir, ".env.controller.example"))
	generator := readFile(t, filepath.Join(composeDir, "generate-secrets.sh"))
	keyPattern := regexp.MustCompile(`(?m)^(LOCK_[A-Z0-9_]+)=`)

	generatedKeys := make(map[string]bool)
	for _, match := range keyPattern.FindAllStringSubmatch(generator, -1) {
		generatedKeys[match[1]] = true
	}
	for _, match := range keyPattern.FindAllStringSubmatch(example, -1) {
		if !generatedKeys[match[1]] {
			t.Errorf("generate-secrets.sh omits controller default %s from .env.controller.example", match[1])
		}
	}
}

func TestCISourceDeploy_InfrastructureBuildFailureIsFatal(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "ci-source-deploy.sh"))
	if strings.Contains(content, "local infra build skipped") {
		t.Fatal("ci-source-deploy.sh ignores local infrastructure build failures before starting with --no-build")
	}
}

func TestCISourceDeploy_WaitsForHealthyStackBeforeCleanup(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "ci-source-deploy.sh"))
	waitIndex := strings.Index(content, `--wait --wait-timeout "${DEPLOY_HEALTH_TIMEOUT_SEC:-300}"`)
	cleanupIndex := strings.LastIndex(content, "cleanup_deploy_source")
	if waitIndex == -1 {
		t.Fatal("ci-source-deploy.sh must wait for compose services to become running/healthy")
	}
	if cleanupIndex == -1 || waitIndex > cleanupIndex {
		t.Fatal("health wait must complete before source cleanup")
	}
}

func TestOpenAPI_DocumentsEveryONTLockRoute(t *testing.T) {
	routes := readFile(t, filepath.Join(projectRoot, "backend", "services", "controller", "internal", "api", "api.go"))
	openapi := readFile(t, filepath.Join(projectRoot, "docs", "openapi.yaml"))
	routePattern := regexp.MustCompile(`(?m)lock\.Handle(?:Func)?\("([^"]+)".*?\.Methods\("([A-Z]+)"\)`)

	for _, match := range routePattern.FindAllStringSubmatch(routes, -1) {
		path := "/api/tenants/{slug}/lock" + match[1]
		pathMarker := "  " + path + ":"
		pathIndex := strings.Index(openapi, pathMarker)
		if pathIndex == -1 {
			t.Errorf("docs/openapi.yaml is missing ONT Lock route %s", path)
			continue
		}
		rest := openapi[pathIndex+len(pathMarker):]
		nextPath := strings.Index(rest, "\n  /api/")
		pathBlock := rest
		if nextPath != -1 {
			pathBlock = rest[:nextPath]
		}
		method := strings.ToLower(match[2])
		if !strings.Contains(pathBlock, "\n    "+method+":") {
			t.Errorf("docs/openapi.yaml is missing %s operation for %s", match[2], path)
		}
	}
}

func TestSnapshotProfile_IncludesRequiredDependencies(t *testing.T) {
	content := readFile(t, filepath.Join(composeDir, "docker-compose.test.yaml"))
	for _, service := range []string{"mongo_test", "nats_test"} {
		servicePattern := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(service) + `:`)
		location := servicePattern.FindStringIndex(content)
		if location == nil {
			t.Fatalf("service %s not found in docker-compose.test.yaml", service)
		}
		rest := content[location[1]:]
		nextService := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:`).FindStringIndex(rest)
		block := rest
		if nextService != nil {
			block = rest[:nextService[0]]
		}
		if !regexp.MustCompile(`profiles:\s*\[[^\]]*snapshot`).MatchString(block) {
			t.Errorf("service %s must be enabled by the snapshot profile", service)
		}
	}
}

func TestCIAndCompose_RunFullBridgeRegressionSuite(t *testing.T) {
	ci := readFile(t, filepath.Join(projectRoot, ".gitlab-ci.yml"))
	compose := readFile(t, filepath.Join(composeDir, "docker-compose.test.yaml"))
	fullCommand := "go test -v -count=1 -timeout=120s ./internal/bridge/"

	if !strings.Contains(ci, fullCommand) {
		t.Errorf("GitLab bridge job must run the full bridge package so concurrency and late-write regressions cannot be filtered out")
	}
	if !strings.Contains(compose, fullCommand) {
		t.Errorf("Docker test-bridge service must run the same full bridge package as CI")
	}
}
