/**
 * Tests for SocketIOContext.
 *
 * Verifies:
 * 1. Socket is created only once, not on every render -- WILL FAIL
 * 2. Disconnect handler is not empty
 */

jest.mock('socket.io-client', () => {
  const mockSocket = {
    on: jest.fn(),
    emit: jest.fn(),
    disconnect: jest.fn(),
    connected: true,
  };
  const io = jest.fn(() => mockSocket);
  io._mockSocket = mockSocket;
  return { __esModule: true, default: io };
});

jest.mock('next/router', () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

jest.mock('src/contexts/error-context', () => ({
  useAlertContext: () => ({ setAlert: jest.fn() }),
}));

test('socket.io-client should be called only once across re-renders', () => {
  const io = require('socket.io-client').default;
  const React = require('react');
  const { renderHook } = require('@testing-library/react');

  // Clear any prior calls
  io.mockClear();

  // We need to read the source to check if io() is called outside useEffect/useMemo
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'socketio-context.js'),
    'utf8'
  );

  // Check if io() is called outside of useEffect/useMemo/useRef
  // A simple heuristic: if "const socket = io(" appears outside of a useEffect/useMemo callback,
  // it runs on every render.
  const lines = source.split('\n');
  let insideUseEffect = false;
  let ioCalledOutsideEffect = false;

  for (const line of lines) {
    if (line.includes('useEffect') || line.includes('useMemo') || line.includes('useRef')) {
      insideUseEffect = true;
    }
    if (line.includes('io(') && !insideUseEffect && !line.trim().startsWith('//') && !line.trim().startsWith('*')) {
      ioCalledOutsideEffect = true;
    }
  }

  if (ioCalledOutsideEffect) {
    throw new Error(
      'BUG: socket.io-client io() is called outside useEffect/useMemo/useRef -- ' +
      'creates a new connection on every render of WsProvider'
    );
  }
});

test('disconnect handler should not be empty', () => {
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'socketio-context.js'),
    'utf8'
  );

  // Find the disconnect handler
  const disconnectMatch = source.match(/on\s*\(\s*['"]disconnect['"]\s*,\s*function\s*\(\s*\)\s*\{\s*\}/);
  if (disconnectMatch) {
    throw new Error(
      'BUG: Socket.IO disconnect handler is empty (no-op) -- ' +
      'user gets no notification when real-time connection drops'
    );
  }
});
