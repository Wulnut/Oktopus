/**
 * Tests for devices.js error handling.
 *
 * Verifies that API failures show user-visible errors, not just console.error.
 * These tests will FAIL -- current code uses raw fetch() with console.error only.
 */

test('devices.js should use httpRequest or show user-visible errors, not console.error', () => {
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'devices.js'),
    'utf8'
  );

  // Count occurrences of console.error used as the sole error handler in catch blocks
  const consoleErrorCatches = (source.match(/\.catch\s*\(\s*\w*\s*=>\s*\{?\s*(?:return\s+)?console\.error/g) || []);

  if (consoleErrorCatches.length > 0) {
    throw new Error(
      `BUG: devices.js has ${consoleErrorCatches.length} .catch blocks that only call console.error -- ` +
      'user sees no error feedback when API calls fail. Should use setAlert or httpRequest from BackendContext.'
    );
  }
});

test('devices.js should not use raw fetch() -- should use httpRequest from BackendContext', () => {
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'devices.js'),
    'utf8'
  );

  // Count raw fetch() calls (not httpRequest)
  // Exclude comments and imports
  const lines = source.split('\n').filter(l => !l.trim().startsWith('//') && !l.trim().startsWith('*'));
  const rawFetchCalls = lines.filter(l => /\bfetch\s*\(/.test(l) && !l.includes('httpRequest'));

  if (rawFetchCalls.length > 0) {
    throw new Error(
      `BUG: devices.js has ${rawFetchCalls.length} raw fetch() calls -- ` +
      'bypasses centralized error handling, auth token refresh, and 401 redirect. ' +
      'Should use httpRequest from BackendContext.'
    );
  }
});

test('devices.js removeDevice should show error on failure, not just console.log', () => {
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'devices.js'),
    'utf8'
  );

  // Find the removeDevice function and check if it uses setAlert on error
  const removeDeviceBlock = source.substring(
    source.indexOf('removeDevice'),
    source.indexOf('removeDevice') + 500
  );

  if (removeDeviceBlock.includes('console.log') && !removeDeviceBlock.includes('setAlert')) {
    throw new Error(
      'BUG: removeDevice logs errors to console.log but does not call setAlert -- ' +
      'user sees dialog close with no indication that deletion failed'
    );
  }
});
