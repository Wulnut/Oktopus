import { act, render, waitFor } from '@testing-library/react';
import CredentialsPage from 'src/pages/credentials';
import DevicesPage from 'src/pages/devices';

const push = jest.fn();
const router = { push };
const httpRequest = jest.fn();
const setAlert = jest.fn();

jest.mock('next/router', () => ({
  useRouter: () => router,
}));

jest.mock('src/hooks/use-auth', () => ({
  useAuth: () => ({ user: { token: 'test-token' } }),
}));

jest.mock('src/contexts/tenant-context', () => ({
  useTenant: () => ({ apiPrefix: '/api/tenants/test' }),
}));

jest.mock('src/contexts/backend-context', () => ({
  useBackendContext: () => ({
    apiPrefix: '/api/tenants/test',
    httpRequest,
    setAlert,
  }),
}));

jest.mock('@emotion/react', () => ({
  ...jest.requireActual('@emotion/react'),
  useTheme: () => ({ palette: { primary: { lightest: '#fff' } } }),
}));

jest.mock('src/layouts/dashboard/layout', () => ({
  Layout: ({ children }) => children,
}));

jest.mock('src/sections/credentials/credentials-table', () => ({
  CredentialsTable: () => null,
}));

describe('page request effects', () => {
  beforeEach(() => {
    push.mockClear();
    httpRequest.mockReset();
    setAlert.mockClear();
  });

  afterEach(() => {
    delete global.fetch;
  });

  test('credentials 404 performs one request instead of retriggering on local state', async () => {
    global.fetch = jest.fn(async () => ({ status: 404 }));

    render(<CredentialsPage />);

    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(global.fetch).toHaveBeenCalledTimes(1);
  });

  test('devices initial load does not retrigger when response page is stored for the UI', async () => {
    httpRequest.mockImplementation(async (url) => {
      if (url.endsWith('/device/filterOptions')) {
        return {
          status: 200,
          result: { productClasses: [], vendors: [], versions: [], models: [] },
        };
      }

      return {
        status: 200,
        result: { devices: [], pages: 0, page: 0, total: 0 },
      };
    });

    render(<DevicesPage />);

    await waitFor(() => {
      expect(httpRequest.mock.calls.some(([url]) => url.includes('/device?'))).toBe(true);
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const deviceListCalls = httpRequest.mock.calls.filter(([url]) => url.includes('/device?'));
    expect(deviceListCalls).toHaveLength(1);
  });
});
