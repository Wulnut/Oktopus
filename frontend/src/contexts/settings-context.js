import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

const SettingsContext = createContext({
  themeMode: 'light',
  setThemeMode: () => {},
});

export function SettingsProvider({ children }) {
  const [themeMode, setThemeModeState] = useState('light');

  useEffect(() => {
    const stored = window.localStorage.getItem('themeMode');
    if (stored === 'dark' || stored === 'light') {
      setThemeModeState(stored);
    }
  }, []);

  const setThemeMode = useCallback((mode) => {
    setThemeModeState(mode);
    window.localStorage.setItem('themeMode', mode);
  }, []);

  const value = useMemo(() => ({
    themeMode,
    setThemeMode,
  }), [themeMode, setThemeMode]);

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  );
}

export const useSettings = () => useContext(SettingsContext);
