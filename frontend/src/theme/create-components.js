import {
  createTheme,
  filledInputClasses,
  inputLabelClasses,
  outlinedInputClasses,
  paperClasses,
  tableCellClasses
} from '@mui/material';

// Used only to create transitions
const muiTheme = createTheme();

export function createComponents(config) {
  const { palette } = config;

  return {
    MuiAvatar: {
      styleOverrides: {
        root: {
          fontSize: 14,
          fontWeight: 600,
          letterSpacing: 0
        }
      }
    },
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: '12px',
          textTransform: 'none',
          '&:hover': {
            backgroundColor: palette.mode === 'dark' ? '#4da3b8' : palette.primary.main,
            color: palette.primary.contrastText
          },
        },
        sizeSmall: {
          padding: '6px 16px'
        },
        sizeMedium: {
          padding: '8px 20px'
        },
        sizeLarge: {
          padding: '11px 24px'
        },
        textSizeSmall: {
          padding: '7px 12px'
        },
        textSizeMedium: {
          padding: '9px 16px'
        },
        textSizeLarge: {
          padding: '12px 16px'
        }
      }
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 20,
          [`&.${paperClasses.elevation1}`]: {
            boxShadow: palette.mode === 'dark'
              ? '0px 5px 22px rgba(0, 0, 0, 0.4), 0px 0px 0px 0.5px rgba(0, 0, 0, 0.12)'
              : '0px 5px 22px rgba(0, 0, 0, 0.1), 0px 0px 0px 0.5px rgba(0, 0, 0, 0.03)'
          }
        }
      }
    },
    MuiCardContent: {
      styleOverrides: {
        root: {
          padding: '32px 24px',
          '&:last-child': {
            paddingBottom: '32px'
          }
        }
      }
    },
    MuiCardHeader: {
      defaultProps: {
        titleTypographyProps: {
          variant: 'h6'
        },
        subheaderTypographyProps: {
          variant: 'body2'
        }
      },
      styleOverrides: {
        root: {
          padding: '32px 24px 16px'
        }
      }
    },
    MuiCssBaseline: {
      styleOverrides: {
        '*': {
          boxSizing: 'border-box'
        },
        html: {
          MozOsxFontSmoothing: 'grayscale',
          WebkitFontSmoothing: 'antialiased',
          display: 'flex',
          flexDirection: 'column',
          minHeight: '100%',
          width: '100%'
        },
        body: {
          display: 'flex',
          flex: '1 1 auto',
          flexDirection: 'column',
          minHeight: '100%',
          width: '100%'
        },
        '#__next': {
          display: 'flex',
          flex: '1 1 auto',
          flexDirection: 'column',
          height: '100%',
          width: '100%'
        },
        '#nprogress': {
          pointerEvents: 'none'
        },
        '#nprogress .bar': {
          backgroundColor: palette.primary.main,
          height: 3,
          left: 0,
          position: 'fixed',
          top: 0,
          width: '100%',
          zIndex: 2000
        }
      }
    },
    MuiInputBase: {
      styleOverrides: {
        input: {
          '&::placeholder': {
            opacity: 1
          }
        }
      }
    },
    MuiInput: {
      styleOverrides: {
        input: {
          fontSize: 14,
          fontWeight: 500,
          lineHeight: '24px',
          '&::placeholder': {
            color: palette.text.secondary
          }
        }
      }
    },
    MuiFilledInput: {
      styleOverrides: {
        root: {
          backgroundColor: palette.mode === 'dark' ? 'rgba(255,255,255,0.04)' : 'transparent',
          borderRadius: 8,
          borderStyle: 'solid',
          borderWidth: 1,
          overflow: 'hidden',
          borderColor: palette.mode === 'dark' ? palette.divider : palette.neutral[200],
          transition: muiTheme.transitions.create([
            'border-color',
            'box-shadow'
          ]),
          '&:hover': {
            backgroundColor: palette.action.hover
          },
          '&:before': {
            display: 'none'
          },
          '&:after': {
            display: 'none'
          },
          [`&.${filledInputClasses.disabled}`]: {
            backgroundColor: 'transparent'
          },
          [`&.${filledInputClasses.focused}`]: {
            backgroundColor: 'transparent',
            borderColor: palette.primary.main,
            boxShadow: `${palette.primary.main} 0 0 0 2px`
          },
          [`&.${filledInputClasses.error}`]: {
            borderColor: palette.error.main,
            boxShadow: `${palette.error.main} 0 0 0 2px`
          }
        },
        input: {
          fontSize: 14,
          fontWeight: 500,
          lineHeight: '24px'
        }
      }
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          backgroundColor: palette.mode === 'dark' ? 'rgba(255,255,255,0.04)' : 'transparent',
          '&:hover': {
            backgroundColor: palette.action.hover,
            [`& .${outlinedInputClasses.notchedOutline}`]: {
              borderColor: palette.mode === 'dark' ? '#4A5568' : palette.neutral[200]
            }
          },
          [`&.${outlinedInputClasses.focused}`]: {
            backgroundColor: 'transparent',
            [`& .${outlinedInputClasses.notchedOutline}`]: {
              borderColor: palette.primary.main,
              borderWidth: 2,
            }
          },
          [`&.${filledInputClasses.error}`]: {
            [`& .${outlinedInputClasses.notchedOutline}`]: {
              borderColor: palette.error.main,
              borderWidth: 2,
            }
          }
        },
        input: {
          fontSize: 14,
          fontWeight: 500,
          lineHeight: '24px',
        },
        notchedOutline: {
          borderColor: palette.mode === 'dark' ? palette.divider : palette.neutral[200],
          transition: muiTheme.transitions.create([
            'border-color',
            'box-shadow'
          ])
        }
      }
    },
    MuiFab: {
      styleOverrides: {
        root: {
          '&:hover': {
            backgroundColor: palette.primary.main,
          },
        }
      }
    },
    MuiFormLabel: {
      styleOverrides: {
        root: {
          fontSize: 14,
          fontWeight: 500,
          [`&.${inputLabelClasses.filled}`]: {
            transform: 'translate(12px, 18px) scale(1)'
          },
          [`&.${inputLabelClasses.shrink}`]: {
            [`&.${inputLabelClasses.standard}`]: {
              transform: 'translate(0, -1.5px) scale(0.85)'
            },
            [`&.${inputLabelClasses.filled}`]: {
              transform: 'translate(12px, 6px) scale(0.85)'
            },
            [`&.${inputLabelClasses.outlined}`]: {
              transform: 'translate(14px, -9px) scale(0.85)'
            }
          }
        }
      }
    },
    MuiTab: {
      styleOverrides: {
        root: {
          fontSize: 14,
          fontWeight: 500,
          lineHeight: 1.71,
          minWidth: 'auto',
          paddingLeft: 0,
          paddingRight: 0,
          textTransform: 'none',
          '& + &': {
            marginLeft: 24
          }
        }
      }
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottomColor: palette.divider,
          padding: '15px 16px'
        }
      }
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          borderBottom: 'none',
          [`& .${tableCellClasses.root}`]: {
            borderBottom: 'none',
            // Light: soft gray header + dark text. Dark: keep brand table color + white text.
            // (neutral[50]/[700] are brand-mapped and unsuitable for light table heads.)
            backgroundColor: palette.mode === 'dark' ? palette.neutral[50] : palette.neutral[100],
            color: palette.mode === 'dark' ? palette.neutral[700] : palette.neutral[600],
            fontSize: 12,
            fontWeight: 600,
            lineHeight: 1,
            letterSpacing: 0.5,
            textTransform: 'uppercase'
          },
          [`& .${tableCellClasses.paddingCheckbox}`]: {
            paddingTop: 4,
            paddingBottom: 4,
            color: palette.mode === 'dark' ? palette.neutral[700] : palette.neutral[600],
          }
        }
      }
    },
    MuiTextField: {
      defaultProps: {
        variant: 'filled'
      }
    }
  };
}
