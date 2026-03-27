/**
 * Tests for BackendContext / httpRequest.
 *
 * These tests verify:
 * 1. Token is read fresh per request (not cached at init) -- WILL FAIL
 * 2. Error alert shown on non-200 responses
 * 3. Redirect on 401
 */

// Mock next/router
const mockPush = jest.fn();
jest.mock('next/router', () => ({
  useRouter: () => ({ push: mockPush }),
}));

// Mock error context
const mockSetAlert = jest.fn();
jest.mock('src/contexts/error-context', () => ({
  useAlertContext: () => ({ setAlert: mockSetAlert }),
}));

import React from 'react';
import { renderHook, act } from '@testing-library/react';
import { BackendProvider, useBackendContext } from '../backend-context';

const wrapper = ({ children }) => <BackendProvider>{children}</BackendProvider>;

beforeEach(() => {
  jest.clearAllMocks();
  global.fetch = jest.fn();
  global.localStorage = {
    getItem: jest.fn(() => 'test-token'),
    setItem: jest.fn(),
    removeItem: jest.fn(),
  };
});

test('httpRequest sends GET with auth header and returns parsed JSON', async () => {
  global.fetch.mockResolvedValue({
    status: 200,
    json: () => Promise.resolve({ data: 'test-data' }),
  });

  const { result } = renderHook(() => useBackendContext(), { wrapper });

  let response;
  await act(async () => {
    response = await result.current.httpRequest('/api/test', 'GET');
  });

  expect(response.status).toBe(200);
  expect(response.result).toEqual({ data: 'test-data' });
  expect(global.fetch).toHaveBeenCalledTimes(1);
});

test('httpRequest shows error alert on 500 response', async () => {
  global.fetch.mockResolvedValue({
    status: 500,
    text: () => Promise.resolve('Internal Server Error'),
  });

  const { result } = renderHook(() => useBackendContext(), { wrapper });

  await act(async () => {
    await result.current.httpRequest('/api/test', 'GET');
  });

  expect(mockSetAlert).toHaveBeenCalledWith(
    expect.objectContaining({ severity: 'error' })
  );
});

test('httpRequest redirects to login on 401', async () => {
  global.fetch.mockResolvedValue({
    status: 401,
    text: () => Promise.resolve('Unauthorized'),
  });

  const { result } = renderHook(() => useBackendContext(), { wrapper });

  await act(async () => {
    await result.current.httpRequest('/api/test', 'GET');
  });

  expect(mockPush).toHaveBeenCalledWith('/auth/login');
});

test('httpRequest reads token fresh per request, not cached at init', () => {
  // Verify the source code reads localStorage.getItem('token') inside httpRequest,
  // not at module/component init level.
  const fs = require('fs');
  const path = require('path');
  const source = fs.readFileSync(
    path.join(__dirname, '..', 'backend-context.js'),
    'utf8'
  );

  // Check that localStorage.getItem('token') is NOT called at the top level
  // (outside of httpRequest). It should be inside the useCallback/httpRequest function.
  const lines = source.split('\n');
  let inHttpRequest = false;
  let tokenReadOutsideFunction = false;

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed.includes('httpRequest') && (trimmed.includes('const') || trimmed.includes('async'))) {
      inHttpRequest = true;
    }
    if (!inHttpRequest && trimmed.includes("localStorage.getItem") && trimmed.includes("token")) {
      tokenReadOutsideFunction = true;
    }
  }

  if (tokenReadOutsideFunction) {
    throw new Error(
      'BUG: BackendContext reads localStorage token outside httpRequest -- ' +
      'token is cached at init and never refreshed'
    );
  }
});
