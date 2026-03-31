import PropTypes from 'prop-types';
import Link from 'next/link'
import { Box, Typography, Stack } from '@mui/material';

export const Layout = (props) => {
  const { children } = props;

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

      {/* Footer */}
      <Stack sx={{ position: 'absolute', bottom: 2, left: 2 }} direction="row" spacing={1}>
        <Typography
          align="center"
          color="text.secondary"
          component="footer"
          variant="body2"
          sx={{ p: 2 }}
        >
          Powered by
        </Typography>
      </Stack>
      <a href='https://oktopus.app.br' style={{ position: 'absolute', bottom: 10, left: 100 }} target='_blank'>
        <img
          src="/assets/logo.png"
          alt="Oktopus logo image"
          width={80}
        />
      </a>
    </Box>
  );
};

Layout.prototypes = {
  children: PropTypes.node
};