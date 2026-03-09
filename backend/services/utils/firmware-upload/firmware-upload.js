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
        const form = new IncomingForm({ maxFileSize: 500 * 1024 * 1024, uploadDir: FIRMWARE_DIR }); // 500MB
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
