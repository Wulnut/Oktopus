import { useEffect, useState } from 'react';

const FALLBACK = {
  version: '0.0.0-dev',
  commit: 'unknown',
  built_at: '',
  label: 'v0.0.0-dev',
};

export const useAppVersion = () => {
  const [info, setInfo] = useState(FALLBACK);

  useEffect(() => {
    let cancelled = false;
    fetch('/version.json', { cache: 'no-store' })
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (cancelled || !data) return;
        const version = data.version || FALLBACK.version;
        const commit = data.commit || FALLBACK.commit;
        setInfo({
          version,
          commit,
          built_at: data.built_at || '',
          label: data.label || `v${version} (${commit})`,
        });
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return info;
};
