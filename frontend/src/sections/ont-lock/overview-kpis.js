import { useEffect, useRef } from 'react';
import PropTypes from 'prop-types';
import { Box, Card, CardContent, Stack, Typography } from '@mui/material';
import gsap from 'gsap';

const KPI_ITEMS = [
  { key: 'unauthorized', label: 'Unauthorized', color: 'warning.main' },
  { key: 'unsupported', label: 'Unsupported', color: 'info.main' },
  { key: 'success', label: 'Commands success', color: 'success.main' },
  { key: 'failed', label: 'Commands failed', color: 'error.main' },
];

export const OverviewKpis = ({ counts }) => {
  const rootRef = useRef(null);
  const valueRefs = useRef([]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return undefined;

    const cards = root.querySelectorAll('[data-kpi-card]');
    const mm = gsap.matchMedia();

    mm.add(
      {
        reduceMotion: '(prefers-reduced-motion: reduce)',
        allowMotion: '(prefers-reduced-motion: no-preference)',
      },
      (context) => {
        const { reduceMotion } = context.conditions;
        if (reduceMotion) {
          gsap.set(cards, { autoAlpha: 1, y: 0 });
          valueRefs.current.forEach((el, idx) => {
            if (!el) return;
            const key = KPI_ITEMS[idx].key;
            el.textContent = String(counts[key] ?? 0);
          });
          return undefined;
        }

        gsap.from(cards, {
          y: 16,
          autoAlpha: 0,
          duration: 0.45,
          stagger: 0.08,
          ease: 'power2.out',
          clearProps: 'transform',
        });

        valueRefs.current.forEach((el, idx) => {
          if (!el) return;
          const key = KPI_ITEMS[idx].key;
          const target = { value: 0 };
          const end = Number(counts[key] ?? 0);
          gsap.to(target, {
            value: end,
            duration: 0.7,
            delay: 0.08 * idx,
            ease: 'power2.out',
            onUpdate: () => {
              el.textContent = String(Math.round(target.value));
            },
          });
        });

        return undefined;
      }
    );

    return () => mm.revert();
  }, [counts]);

  return (
    <Stack
      ref={rootRef}
      direction={{ xs: 'column', sm: 'row' }}
      spacing={2}
      useFlexGap
      flexWrap="wrap"
    >
      {KPI_ITEMS.map((item, idx) => (
        <Card
          key={item.key}
          data-kpi-card
          sx={{ flex: '1 1 160px', minWidth: 160 }}
        >
          <CardContent>
            <Typography color="text.secondary" variant="overline">
              {item.label}
            </Typography>
            <Box
              ref={(el) => {
                valueRefs.current[idx] = el;
              }}
              sx={{
                mt: 1,
                typography: 'h4',
                color: item.color,
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {counts[item.key] ?? 0}
            </Box>
          </CardContent>
        </Card>
      ))}
    </Stack>
  );
};

OverviewKpis.propTypes = {
  counts: PropTypes.shape({
    unauthorized: PropTypes.number,
    unsupported: PropTypes.number,
    success: PropTypes.number,
    failed: PropTypes.number,
  }).isRequired,
};
