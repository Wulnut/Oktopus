import Head from 'next/head';
import { CacheProvider } from '@emotion/react';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFnsV3';
import { CssBaseline } from '@mui/material';
import { ThemeProvider } from '@mui/material/styles';
import { AuthConsumer, AuthProvider } from 'src/contexts/auth-context';
import { SettingsProvider, useSettings } from 'src/contexts/settings-context';
import { useNProgress } from 'src/hooks/use-nprogress';
import { createTheme } from 'src/theme';
import { createEmotionCache } from 'src/utils/create-emotion-cache';
import 'simplebar-react/dist/simplebar.min.css';
import '../utils/map.css';
import { useMemo } from 'react';
import { BackendProvider } from 'src/contexts/backend-context';
import { AlertProvider } from 'src/contexts/error-context';
import { TenantProvider } from 'src/contexts/tenant-context';

const clientSideEmotionCache = createEmotionCache();

const SplashScreen = () => null;

function ThemedApp(props) {
  const { Component, emotionCache = clientSideEmotionCache, pageProps } = props;
  const { themeMode } = useSettings();

  useNProgress();

  const theme = useMemo(() => createTheme(themeMode), [themeMode]);

  const getLayout = Component.getLayout ?? ((page) => page);

  return (
    <CacheProvider value={emotionCache}>
      <Head>
        <title>
          Oktopus | Controller
        </title>
        <meta
          name="viewport"
          content="initial-scale=1, width=device-width"
        />
      </Head>
      <LocalizationProvider dateAdapter={AdapterDateFns}>
        <AuthProvider>
          <TenantProvider>
          {/* <WsProvider> */}
          <AlertProvider>
            <BackendProvider>
                <ThemeProvider theme={theme}>
                  <CssBaseline />
                  <AuthConsumer>
                    {
                      (auth) => auth.isLoading
                        ? <SplashScreen />
                        : getLayout(<Component {...pageProps} />)
                    }
                  </AuthConsumer>
                </ThemeProvider>
            </BackendProvider>
          </AlertProvider>
          {/* </WsProvider> */}
          </TenantProvider>
        </AuthProvider>
      </LocalizationProvider>
    </CacheProvider>
  );
}

const App = (props) => (
  <SettingsProvider>
    <ThemedApp {...props} />
  </SettingsProvider>
);

export default App;
