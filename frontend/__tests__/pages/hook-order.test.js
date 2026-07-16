import { render, waitFor } from '@testing-library/react';
import LoginPage from 'src/pages/auth/login';
import TenantsPage from 'src/pages/tenants';

const push = jest.fn();
const auth = {
  user: { token: 'test-token' },
  signIn: jest.fn(),
  skip: jest.fn(),
};

jest.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}));

jest.mock('next/router', () => ({
  useRouter: () => ({ push }),
}));

jest.mock('src/hooks/use-auth', () => ({
  useAuth: () => auth,
}));

jest.mock('src/contexts/tenant-context', () => ({
  useTenant: () => ({ isSuperAdmin: true }),
}));

jest.mock('src/layouts/auth/layout', () => ({
  Layout: ({ children }) => children,
}));

jest.mock('src/layouts/dashboard/layout', () => ({
  Layout: ({ children }) => children,
}));

jest.mock('src/sections/tenants/tenants-table', () => ({
  TenantsTable: () => null,
}));

jest.mock('src/sections/tenants/tenant-form', () => ({
  TenantForm: () => null,
}));

describe('page hook declaration order', () => {
  beforeEach(() => {
    push.mockClear();
    global.fetch = jest.fn(async (url) => {
      if (url.endsWith('/api/auth/admin/exists')) {
        const response = {
          status: 200,
          json: async () => true,
          text: async () => 'true',
        };
        response.clone = () => response;
        return response;
      }

      const response = {
        status: 200,
        ok: true,
        json: async () => [],
        text: async () => '[]',
      };
      response.clone = () => response;
      return response;
    });
  });

  afterEach(() => {
    delete global.fetch;
  });

  test('login page renders without referencing a callback before declaration', async () => {
    expect(() => render(<LoginPage />)).not.toThrow();
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
  });

  test('tenants page renders without referencing a callback before declaration', async () => {
    expect(() => render(<TenantsPage />)).not.toThrow();
    await waitFor(() => expect(global.fetch).toHaveBeenCalled());
  });
});
