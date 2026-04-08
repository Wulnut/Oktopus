#!/usr/bin/env node
// Minimal Node.js service for container uploads
// Runs docker import, tag, and push commands
//
// SECURITY NOTE: This service prefixes all container images with the tenant slug
// from the JWT (e.g., "prpl-test/my-container:v1.0"). This provides namespace
// isolation at the naming level. However, the Docker Registry itself does NOT
// enforce download authentication — any client that knows an image name can pull it.
// This is acceptable because:
// 1. Devices only pull URLs constructed by the controller
// 2. The controller only constructs URLs with the correct tenant prefix
// 3. The /_catalog endpoint requires auth (enforced by nginx)
// See docs/SECURITY.md for full threat model.

const http = require('http');
const { formidable } = require('formidable');
const { execFile } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');
const url = require('url');

const PORT = process.env.UPLOAD_SERVICE_PORT || 8005;
const REGISTRY = process.env.REGISTRY || '127.0.0.1:443';

// Decode JWT payload (base64url) to extract tenant_slug.
// Token was already verified by the controller/nginx — we just need the claims.
function decodeTenantFromToken(authHeader) {
  if (!authHeader) return null;
  const token = authHeader.replace(/^Bearer\s+/i, '');
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = Buffer.from(parts[1], 'base64url').toString('utf8');
    const claims = JSON.parse(payload);
    return claims.tenant_slug || null;
  } catch {
    return null;
  }
}

// Validate Docker image name (lowercase, digits, underscores, periods, hyphens)
function validateImageName(name) {
  if (!name || typeof name !== 'string') return false;
  // Docker image name rules: lowercase letters, digits, underscores, periods, hyphens
  // Cannot start with period or hyphen
  return /^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$/.test(name) && name.length <= 255;
}

// Validate Docker tag (similar to name but can include uppercase)
function validateImageTag(tag) {
  if (!tag || typeof tag !== 'string') return false;
  // Docker tag rules: alphanumeric, underscores, periods, hyphens
  // Cannot start with period or hyphen
  return /^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$/.test(tag) && tag.length <= 128;
}

// Secure command execution using execFile to prevent command injection
function execCommand(command, args) {
  return new Promise((resolve, reject) => {
    execFile(command, args, (error, stdout, stderr) => {
      if (error) {
        reject({ error, stderr, stdout });
      } else {
        resolve({ stdout, stderr });
      }
    });
  });
}

const https = require('https');

// Helper to make HTTPS requests ignoring certificate errors
function httpsRequest(options, data = null) {
  return new Promise((resolve, reject) => {
    const req = https.request({
      ...options,
      rejectUnauthorized: false, // Ignore certificate errors
    }, (res) => {
      let body = '';
      res.on('data', (chunk) => { body += chunk; });
      res.on('end', () => {
        resolve({ statusCode: res.statusCode, headers: res.headers, body });
      });
    });
    req.on('error', reject);
    if (data) {
      req.write(data);
    }
    req.end();
  });
}

const server = http.createServer(async (req, res) => {
  // CORS headers
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'POST, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');

  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

  // Handle DELETE request for container deletion
  if (req.method === 'DELETE' && req.url.startsWith('/delete')) {
    // Check authorization
    const authHeader = req.headers.authorization;
    if (!authHeader) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Unauthorized' }));
      return;
    }

    try {
      const tenantSlug = decodeTenantFromToken(authHeader);
      if (!tenantSlug) {
        res.writeHead(403, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ message: 'Forbidden: tenant context required. SuperAdmin must select a tenant.' }));
        return;
      }

      const parsedUrl = url.parse(req.url, true);
      const name = parsedUrl.query.name;
      const tag = parsedUrl.query.tag;

      if (!name || !tag) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'Missing required parameters: name and tag' }));
        return;
      }

      // Validate input to prevent command injection
      if (!validateImageName(name) || !validateImageTag(tag)) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'Invalid image name or tag format' }));
        return;
      }

      // SECURITY: Tenant prefix provides namespace isolation in the shared registry.
      // See docs/SECURITY.md for known limitations on download-level enforcement.
      const prefixedName = `${tenantSlug}/${name}`;

      console.log(`Deleting ${prefixedName}:${tag} from registry ${REGISTRY}`);

      // TODO: Fix it in future - Properly handle host.docker.internal resolution
      // Current workaround: Using hardcoded gateway IP (172.17.0.1) because host.docker.internal
      // doesn't resolve on Linux. This is a terrible workaround that should be fixed by:
      // 1. Properly configuring host.docker.internal via extra_hosts in docker-compose
      // 2. Or dynamically detecting the gateway IP from the container's network
      // 3. Or using a proper service discovery mechanism
      let registryHost = REGISTRY.split(':')[0];
      let registryPort = REGISTRY.split(':')[1] || 443;
      if (registryHost === '127.0.0.1') {
        // Terrible workaround: hardcoded gateway IP
        registryHost = '172.17.0.1';
      }

      // Step 1: Get manifest digest for the tag (using HEAD to get only headers)
      let manifestResponse;
      try {
        manifestResponse = await httpsRequest({
          hostname: registryHost,
          port: parseInt(registryPort),
          path: `/v2/${prefixedName}/manifests/${tag}`,
          method: 'HEAD',
          headers: {
            'Accept': 'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.v1+json',
          },
        });
      } catch (err) {
        // If host.docker.internal doesn't work, try gateway IP
        if (registryHost === 'host.docker.internal') {
          registryHost = '172.17.0.1';
          manifestResponse = await httpsRequest({
            hostname: registryHost,
            port: parseInt(registryPort),
            path: `/v2/${prefixedName}/manifests/${tag}`,
            method: 'HEAD',
            headers: {
              'Accept': 'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.v1+json',
            },
          });
        } else {
          throw err;
        }
      }

      if (manifestResponse.statusCode !== 200) {
        // Log more details for debugging
        console.error(`Manifest request failed: ${manifestResponse.statusCode}`, {
          path: `/v2/${prefixedName}/manifests/${tag}`,
          hostname: registryHost,
          port: registryPort,
          body: manifestResponse.body?.substring(0, 200),
        });
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ 
          error: `Manifest not found: ${manifestResponse.statusCode}`,
          details: manifestResponse.body?.substring(0, 200) || 'No response body'
        }));
        return;
      }

      const digest = manifestResponse.headers['docker-content-digest'];
      if (!digest) {
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'Could not get manifest digest' }));
        return;
      }

      // Step 2: Delete manifest using digest
      const deleteResponse = await httpsRequest({
        hostname: registryHost,
        port: parseInt(registryPort),
        path: `/v2/${prefixedName}/manifests/${digest}`,
        method: 'DELETE',
        headers: {
          'Accept': 'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.v1+json',
        },
      });

      if (deleteResponse.statusCode !== 202 && deleteResponse.statusCode !== 404) {
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: `Delete failed: ${deleteResponse.statusCode} ${deleteResponse.body}` }));
        return;
      }

      // Also try to delete local Docker image if it exists
      const imageName = `${REGISTRY}/${prefixedName}:${tag}`;
      execCommand('docker', ['rmi', imageName]).catch(() => {});

      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        message: 'Container deleted successfully',
        name: prefixedName,
        tag,
      }));
    } catch (err) {
      console.error('Delete error:', err);
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: err.message || 'Internal server error' }));
    }
    return;
  }

  if (req.method !== 'POST' || req.url !== '/upload') {
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Not found' }));
    return;
  }

  // Check authorization
  const authHeader = req.headers.authorization;
  if (!authHeader) {
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Unauthorized' }));
    return;
  }

  const tenantSlug = decodeTenantFromToken(authHeader);
  if (!tenantSlug) {
    res.writeHead(403, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Forbidden: tenant context required. SuperAdmin must select a tenant.' }));
    return;
  }

  const form = formidable({
    uploadDir: os.tmpdir(),
    keepExtensions: true,
    maxFileSize: 1024 * 1024 * 1024, // 1GB
  });

  try {
    const [fields, files] = await form.parse(req);
    const name = fields.name?.[0]?.trim();
    const tag = fields.tag?.[0]?.trim();
    const file = files.file?.[0];

    if (!name || !tag || !file) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Missing required fields: name, tag, or file' }));
      return;
    }

    // Validate input to prevent command injection
    if (!validateImageName(name) || !validateImageTag(tag)) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Invalid image name or tag format' }));
      if (file) fs.unlinkSync(file.filepath);
      return;
    }

    // Validate file extension
    if (!file.originalFilename.endsWith('.tar') && !file.originalFilename.endsWith('.tar.gz')) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'File must be .tar or .tar.gz' }));
      fs.unlinkSync(file.filepath);
      return;
    }

    // Validate file path to prevent path traversal
    if (!path.isAbsolute(file.filepath) || !file.filepath.startsWith(os.tmpdir())) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Invalid file path' }));
      fs.unlinkSync(file.filepath);
      return;
    }

    // SECURITY: Tenant prefix provides namespace isolation in the shared registry.
    // See docs/SECURITY.md for known limitations on download-level enforcement.
    const prefixedName = `${tenantSlug}/${name}`;
    const imageName = `${prefixedName}:${tag}`;
    const registryImage = `${REGISTRY}/${imageName}`;

    console.log(`Importing ${imageName} from ${file.filepath}`);

    // Step 1: docker import
    try {
      await execCommand('docker', ['import', file.filepath, imageName]);
    } catch (err) {
      fs.unlinkSync(file.filepath);
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: `Docker import failed: ${err.stderr || err.message}` }));
      return;
    }

    // Step 2: docker tag
    try {
      await execCommand('docker', ['tag', imageName, registryImage]);
    } catch (err) {
      execCommand('docker', ['rmi', imageName]).catch(() => {});
      fs.unlinkSync(file.filepath);
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: `Docker tag failed: ${err.stderr || err.message}` }));
      return;
    }

    // Step 3: docker push
    try {
      await execCommand('docker', ['push', registryImage]);
    } catch (err) {
      execCommand('docker', ['rmi', registryImage]).catch(() => {});
      execCommand('docker', ['rmi', imageName]).catch(() => {});
      fs.unlinkSync(file.filepath);
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: `Docker push failed: ${err.stderr || err.message}` }));
      return;
    }

    // Clean up
    execCommand('docker', ['rmi', registryImage]).catch(() => {});
    execCommand('docker', ['rmi', imageName]).catch(() => {});
    fs.unlinkSync(file.filepath);

    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      message: 'Container uploaded and pushed successfully',
      name: prefixedName,
      tag,
      image: registryImage,
    }));
  } catch (err) {
    console.error('Upload error:', err);
    res.writeHead(500, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: err.message || 'Internal server error' }));
  }
});

server.listen(PORT, () => {
  console.log(`Container upload service listening on port ${PORT}`);
});

