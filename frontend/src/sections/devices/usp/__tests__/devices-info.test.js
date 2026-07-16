import React from 'react';
import { render, waitFor } from '@testing-library/react';
import { DevicesInfo } from '../devices-info';

const mockHttpRequest = jest.fn();
const mockSetAlert = jest.fn();

jest.mock('src/contexts/backend-context', () => ({
  useBackendContext: () => ({
    httpRequest: mockHttpRequest,
    apiPrefix: '/api/tenants/test',
  }),
}));

jest.mock('src/contexts/error-context', () => ({
  useAlertContext: () => ({ setAlert: mockSetAlert }),
}));

beforeEach(() => {
  jest.clearAllMocks();
  mockHttpRequest.mockImplementation(async (path) => {
    if (path.endsWith('/cached-info')) {
      return {
        status: 200,
        result: {
          info: {
            req_path_results: [{
              resolved_path_results: [{
                result_params: { 'Device.DeviceInfo.SerialNumber': 'SN-1' },
              }],
            }],
          },
          updated_at: '2026-07-16T00:00:00Z',
        },
      };
    }
    if (path.endsWith('/any/info')) {
      return { status: 504, result: null };
    }
    if (path.endsWith('/firmware')) {
      return { status: 200, result: [] };
    }
    if (path.endsWith('/upgrade-logs')) {
      return { status: 200, result: [] };
    }
    return { status: 404, result: null };
  });
});

test('waits for status and loads cached info when device becomes offline', async () => {
  const { rerender } = render(
    <DevicesInfo sn="SN-1" mtp="any" deviceOnline={null} />
  );

  expect(mockHttpRequest).not.toHaveBeenCalledWith(
    '/api/tenants/test/device/SN-1/any/info',
    'GET',
    null,
    null
  );

  rerender(<DevicesInfo sn="SN-1" mtp="any" deviceOnline={false} />);

  await waitFor(() => {
    expect(mockHttpRequest).toHaveBeenCalledWith(
      '/api/tenants/test/device/SN-1/cached-info',
      'GET'
    );
  });
});

test('falls back to cached info when a live request times out', async () => {
  render(<DevicesInfo sn="SN-1" mtp="any" deviceOnline={true} />);

  await waitFor(() => {
    expect(mockHttpRequest).toHaveBeenCalledWith(
      '/api/tenants/test/device/SN-1/cached-info',
      'GET'
    );
  });
});
