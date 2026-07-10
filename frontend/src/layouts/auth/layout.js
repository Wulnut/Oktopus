import PropTypes from 'prop-types';
import Link from 'next/link'
import { Box, Typography } from '@mui/material';
import { useAppVersion } from 'src/hooks/use-app-version';

export const Layout = (props) => {
  const { children } = props;
  const appVersion = useAppVersion();

  return (
    <Box
      component="main"
      sx={{
        display: 'flex',
        flex: '1 1 auto',
        minHeight: '100vh',
        backgroundColor: 'background.paper',
        position: 'relative',
      }}
    >
      {/* Logo top-right */}
      <Box
        sx={{
          position: 'absolute',
          top: 24,
          right: 24,
        }}
      >
        <Link href={typeof window !== 'undefined' ? `${window.location.origin}/devices` : '/devices'}>
          <img
            alt=""
            src={`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}/images/logo.png`}
            style={{ maxWidth: 200 }}
          />
        </Link>
      </Box>

      {/* Login form centered */}
      <Box
        sx={{
          display: 'flex',
          flex: '1 1 auto',
          flexDirection: 'column',
        }}
      >
        {children}
      </Box>

      <Typography
        component="footer"
        color="text.secondary"
        variant="caption"
        title={appVersion.built_at ? `Built ${appVersion.built_at}` : undefined}
        sx={{ position: 'absolute', bottom: 16, left: 16 }}
      >
        {appVersion.label}
      </Typography>
    </Box>
  );
};

Layout.prototypes = {
  children: PropTypes.node
};