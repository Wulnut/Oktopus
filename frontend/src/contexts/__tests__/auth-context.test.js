/**
 * Tests for AuthContext.
 *
 * Verifies:
 * 1. signIn does NOT log token to console -- WILL FAIL
 * 2. User object does not contain hardcoded Devias template data -- WILL FAIL
 */

// Mock next/router
jest.mock('next/router', () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

// Mock error context
jest.mock('src/contexts/error-context', () => ({
  useAlertContext: () => ({ setAlert: jest.fn() }),
}));

// Mock backend context
jest.mock('src/contexts/backend-context', () => ({
  useBackendContext: () => ({
    httpRequest: jest.fn(),
  }),
}));

import React from 'react';

beforeEach(() => {
  jest.clearAllMocks();
  global.localStorage = {
    getItem: jest.fn(() => null),
    setItem: jest.fn(),
    removeItem: jest.fn(),
  };
  global.fetch = jest.fn();
});

test('signIn should not log token to console', async () => {
  // This test verifies that auth-context.js:100 does not call
  // console.log with the token value.
  const consoleSpy = jest.spyOn(console, 'log').mockImplementation(() => {});

  global.fetch.mockResolvedValue({
    status: 200,
    json: () => Promise.resolve({ token: 'secret-jwt-token-123' }),
  });

  // Dynamically import to capture console.log calls during module init
  const { AuthProvider, AuthContext } = require('../auth-context');
  const { renderHook, act } = require('@testing-library/react');

  const wrapper = ({ children }) => <AuthProvider>{children}</AuthProvider>;
  const { result } = renderHook(() => React.useContext(AuthContext), { wrapper });

  // Wait for initialization
  await act(async () => {
    await new Promise(resolve => setTimeout(resolve, 100));
  });

  // Check if any console.log call contained the token
  const tokenLogged = consoleSpy.mock.calls.some(call =>
    call.some(arg => typeof arg === 'string' && arg.includes('token'))
  );

  consoleSpy.mockRestore();

  if (tokenLogged) {
    throw new Error(
      'BUG: Token is logged to browser console via console.log -- ' +
      'this exposes credentials in DevTools (auth-context.js:100)'
    );
  }
});

test('user object should not contain hardcoded Devias template data', () => {
  // The auth-context.js contains hardcoded values from the Devias template:
  // id: '5e86809283e28b96d2d38537'
  // email: 'anika.visser@devias.io'
  // These should be removed or replaced with actual user data.

  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'auth-context.js'),
    'utf8'
  );

  if (source.includes('5e86809283e28b96d2d38537')) {
    throw new Error(
      'BUG: auth-context.js contains hardcoded Devias user ID "5e86809283e28b96d2d38537"'
    );
  }
  if (source.includes('anika.visser@devias.io')) {
    throw new Error(
      'BUG: auth-context.js contains hardcoded Devias email "anika.visser@devias.io"'
    );
  }
});
