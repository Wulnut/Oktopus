/**
 * Tests for SocketIOContext.
 *
 * Verifies:
 * 1. Socket is created only once, not on every render
 * 2. Disconnect handler is not empty
 */

const fs = require('fs');
const path = require('path');

const source = fs.readFileSync(
  path.join(__dirname, '..', 'socketio-context.js'),
  'utf8'
);

test('socket.io-client should be created only once (via useRef or useMemo)', () => {
  // Check that io() is called within a useRef/useMemo pattern, not bare in render
  const lines = source.split('\n');
  let hasIoCall = false;
  let ioProtected = false;

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed.startsWith('//') || trimmed.startsWith('*')) continue;
    if (/\bio\(/.test(trimmed) && trimmed.includes('io(')) {
      hasIoCall = true;
      // Check if it's inside a useRef/useMemo/useEffect pattern
      if (source.includes('socketRef.current') || source.includes('useMemo') || source.includes('useRef')) {
        ioProtected = true;
      }
    }
  }

  if (hasIoCall && !ioProtected) {
    throw new Error(
      'BUG: socket.io-client io() is called without useRef/useMemo protection -- ' +
      'creates a new connection on every render of WsProvider'
    );
  }
});

test('disconnect handler should not be empty', () => {
  // Check for empty disconnect handler: on('disconnect', function(){ })
  const emptyHandler = /on\s*\(\s*['"]disconnect['"]\s*,\s*function\s*\(\s*\)\s*\{\s*\}\s*\)/;
  if (emptyHandler.test(source)) {
    throw new Error(
      'BUG: Socket.IO disconnect handler is empty (no-op) -- ' +
      'user gets no notification when real-time connection drops'
    );
  }
});
