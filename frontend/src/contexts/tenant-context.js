import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useAuth } from 'src/hooks/use-auth';

const TenantContext = createContext({
  tenantSlug: '',
  level: 0,
  isSuperAdmin: false,
  apiPrefix: '',
  setActiveTenant: () => {},
});

export function TenantProvider({ children }) {
  const { user } = useAuth();
  const level = user?.level ?? 0;
  const isSuperAdmin = level === 0;

  const [activeTenantSlug, setActiveTenantSlug] = useState(
    user?.tenantSlug || ''
  );

  const setActiveTenant = useCallback((slug) => {
    if (isSuperAdmin) {
      setActiveTenantSlug(slug);
      window.sessionStorage.setItem('activeTenantSlug', slug);
    }
  }, [isSuperAdmin]);

  useEffect(() => {
    if (isSuperAdmin) {
      const stored = window.sessionStorage.getItem('activeTenantSlug');
      if (stored) setActiveTenantSlug(stored);
    }
  }, [isSuperAdmin]);

  // For tenant users, always use their JWT slug
  const tenantSlug = isSuperAdmin ? activeTenantSlug : (user?.tenantSlug || '');
  const apiPrefix = tenantSlug ? `/api/tenants/${tenantSlug}` : '/api';

  const hasTenant = !!tenantSlug;

  const value = useMemo(() => ({
    tenantSlug,
    level,
    isSuperAdmin,
    hasTenant,
    apiPrefix,
    setActiveTenant,
  }), [tenantSlug, level, isSuperAdmin, hasTenant, apiPrefix, setActiveTenant]);

  return (
    <TenantContext.Provider value={value}>
      {children}
    </TenantContext.Provider>
  );
}

export const useTenant = () => useContext(TenantContext);
