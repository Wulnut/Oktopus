import { useState, useEffect } from 'react';
import Head from 'next/head';
import PlusIcon from '@heroicons/react/24/solid/PlusIcon';
import {
  Box,
  Button,
  CircularProgress,
  Container,
  Stack,
  SvgIcon,
  Typography,
} from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { TenantsTable } from 'src/sections/tenants/tenants-table';
import { TenantForm } from 'src/sections/tenants/tenant-form';
import { useAuth } from 'src/hooks/use-auth';
import { useTenant } from 'src/contexts/tenant-context';
import { useRouter } from 'next/router';

const Page = () => {
  const auth = useAuth();
  const router = useRouter();
  const { isSuperAdmin } = useTenant();

  const [tenants, setTenants] = useState([]);
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);

  useEffect(() => {
    if (!isSuperAdmin) {
      router.push('/');
      return;
    }
    fetchTenants();
  }, [isSuperAdmin]);

  const getHeaders = () => {
    const h = new Headers();
    h.append('Content-Type', 'application/json');
    h.append('Authorization', auth.user.token);
    return h;
  };

  const fetchTenants = async () => {
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants`,
        { method: 'GET', headers: getHeaders(), redirect: 'follow' }
      );
      if (res.status === 401) return router.push('/auth/login');
      if (res.status === 403) return router.push('/403');
      // Always clone first — body can only be read once
      const cloned = res.clone();
      let data;
      try {
        data = await cloned.json();
      } catch {
        const text = await res.text();
        console.error('Non-JSON response from /api/tenants:', text);
        data = [];
      }
      setTenants(data || []);
    } catch (err) {
      console.error('Error fetching tenants:', err);
    } finally {
      setLoading(false);
    }
  };

  const createTenant = async (formData) => {
    const res = await fetch(
      `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants`,
      {
        method: 'POST',
        headers: getHeaders(),
        body: JSON.stringify(formData),
        redirect: 'follow',
      }
    );
    if (res.status === 401) return router.push('/auth/login');
    if (res.status === 403) return router.push('/403');
    if (!res.ok) {
      const text = await res.text();
      throw new Error(text);
    }
    const createdClone = res.clone();
    let created;
    try {
      created = await createdClone.json();
    } catch {
      const text = await res.text();
      console.error('Non-JSON response from POST /api/tenants:', text);
      created = null;
    }
    if (created) setTenants([...tenants, created]);
  };

  const deleteTenant = async (slug) => {
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants/${slug}`,
        { method: 'DELETE', headers: getHeaders(), redirect: 'follow' }
      );
      if (res.status === 401) return router.push('/auth/login');
      if (res.status === 403) return router.push('/403');
      setTenants(tenants.filter((t) => t.slug !== slug));
    } catch (err) {
      console.error('Error deleting tenant:', err);
    }
  };

  const toggleTenantStatus = async (slug, newStatus) => {
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants/${slug}`,
        {
          method: 'PUT',
          headers: getHeaders(),
          body: JSON.stringify({ status: newStatus }),
          redirect: 'follow',
        }
      );
      if (res.status === 401) return router.push('/auth/login');
      if (res.status === 403) return router.push('/403');
      if (res.ok) {
        setTenants(
          tenants.map((t) =>
            t.slug === slug ? { ...t, status: newStatus } : t
          )
        );
      }
    } catch (err) {
      console.error('Error updating tenant status:', err);
    }
  };

  if (!isSuperAdmin) {
    return null;
  }

  return (
    <>
      <Head>
        <title>Tenants</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Stack
              direction="row"
              justifyContent="space-between"
              spacing={4}
            >
              <Stack spacing={1}>
                <Typography variant="h4">Tenants</Typography>
              </Stack>
              <div>
                <Button
                  startIcon={
                    <SvgIcon fontSize="small">
                      <PlusIcon />
                    </SvgIcon>
                  }
                  variant="contained"
                  onClick={() => setFormOpen(true)}
                >
                  Create Tenant
                </Button>
              </div>
            </Stack>
            {loading ? (
              <CircularProgress />
            ) : (
              <TenantsTable
                items={tenants}
                onDelete={deleteTenant}
                onToggleStatus={toggleTenantStatus}
              />
            )}
          </Stack>
        </Container>
      </Box>
      <TenantForm
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSubmit={createTenant}
      />
    </>
  );
};

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
