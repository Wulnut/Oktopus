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

test('httpRequest reads token fresh per request, not cached at init', async () => {
  // First call with token-1
  let callCount = 0;
  global.localStorage.getItem = jest.fn(() => {
    callCount++;
    return callCount <= 1 ? 'token-1' : 'token-2';
  });

  global.fetch.mockResolvedValue({
    status: 200,
    json: () => Promise.resolve({}),
  });

  const { result } = renderHook(() => useBackendContext(), { wrapper });

  // First request
  await act(async () => {
    await result.current.httpRequest('/api/test', 'GET');
  });

  const firstCallHeaders = global.fetch.mock.calls[0][1].headers;

  // Change token
  global.localStorage.getItem = jest.fn(() => 'token-2');

  // Second request
  await act(async () => {
    await result.current.httpRequest('/api/test2', 'GET');
  });

  const secondCallHeaders = global.fetch.mock.calls[1][1].headers;

  // The second call should use token-2, not the cached token-1
  // BUG: BackendContext caches myHeaders at init, so both calls use token-1
  const getAuth = (headers) => {
    if (headers instanceof Headers) return headers.get('Authorization');
    if (headers && headers.Authorization) return headers.Authorization;
    return null;
  };

  const firstAuth = getAuth(firstCallHeaders);
  const secondAuth = getAuth(secondCallHeaders);

  if (firstAuth === secondAuth) {
    throw new Error(
      `BUG: Both requests used the same token "${firstAuth}" -- ` +
      'BackendContext caches myHeaders at init instead of reading token per request'
    );
  }
});
