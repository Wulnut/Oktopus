import { render } from '@testing-library/react';
import UsersPage from 'src/pages/users';
import AccessControlUsersPage from 'src/pages/access-control/users';

const router = { push: jest.fn() };

jest.mock('next/router', () => ({
  useRouter: () => router,
}));

jest.mock('src/hooks/use-auth', () => ({
  useAuth: () => ({ user: null }),
}));

jest.mock('src/contexts/tenant-context', () => ({
  useTenant: () => ({ apiPrefix: '/api/tenants/test' }),
}));

jest.mock('src/layouts/dashboard/layout', () => ({
  Layout: ({ children }) => children,
}));

jest.mock('src/sections/customer/customers-table', () => ({
  CustomersTable: () => null,
}));

jest.mock('src/sections/customer/customers-search', () => ({
  CustomersSearch: () => null,
}));

describe('user pages during auth restoration', () => {
  test.each([
    ['users', UsersPage],
    ['access control users', AccessControlUsersPage],
  ])('%s page does not dereference a missing user during render', (_name, Page) => {
    expect(() => render(<Page />)).not.toThrow();
  });
});
