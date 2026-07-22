export const palette = {
  light: {
    surface: '#f9f9f7',
    card: '#fcfcfb',
    ink: '#0b0b0b',
    inkSecondary: '#52514e',
    inkMuted: '#898781',
    border: '#e1e0d9',
    hairline: '#c3c2b7',
    good: '#0ca30c',
    warning: '#fab219',
    serious: '#ec835a',
    critical: '#d03b3b',
    series: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#4a3aa7', '#e34948'],
  },
  dark: {
    surface: '#0d0d0d',
    card: '#1a1a19',
    ink: '#ffffff',
    inkSecondary: '#c3c2b7',
    inkMuted: '#898781',
    border: '#2c2c2a',
    hairline: '#383835',
    good: '#0ca30c',
    warning: '#fab219',
    serious: '#ec835a',
    critical: '#d03b3b',
    series: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'],
  },
} as const

export type ThemeMode = keyof typeof palette
export type Palette = (typeof palette)['light']
