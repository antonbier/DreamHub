/** @type {import('tailwindcss').Config} */
// Tokens, scale, radii and breakpoints mirror ../DESIGN.md exactly — that
// file is the source of truth; keep the two in sync when either changes.
export default {
  // Appearance (dark/light/system) is applied as `data-theme` on <html> by
  // src/stores/preferences.ts, not the OS-only `prefers-color-scheme`.
  darkMode: ['selector', '[data-theme="dark"]'],
  content: ['./index.html', './src/**/*.{vue,ts}'],
  theme: {
    // DESIGN.md, "Layout": the three breakpoints the shell responds to —
    // named for what happens at them, not by size, so usage stays legible.
    screens: {
      'below-lg': { max: '1180px' }, // dashboard grid: 4 -> 2 columns
      'below-md': { max: '900px' }, // sidebar becomes a drawer
      'below-sm': { max: '640px' }, // dashboard grid: -> 1 column
    },
    extend: {
      colors: {
        bg: 'var(--dh-bg)',
        side: 'var(--dh-side)',
        tile: 'var(--dh-tile)',
        'tile-2': 'var(--dh-tile-2)',
        field: 'var(--dh-field)',
        line: 'var(--dh-line)',
        'line-soft': 'var(--dh-line-soft)',
        text: 'var(--dh-text)',
        muted: 'var(--dh-muted)',
        faint: 'var(--dh-faint)',
        accent: 'var(--dh-accent)',
        onair: 'var(--dh-onair)',
        warn: 'var(--dh-warn)',
        ok: 'var(--dh-ok)',
      },
      fontFamily: {
        sans: ['"Source Sans 3"', 'system-ui', 'sans-serif'],
        mono: ['"Source Code Pro"', 'ui-monospace', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '6px', // tiles, buttons
        sm: '4px', // picons, inputs
        lg: '10px', // dialogs
      },
      maxWidth: {
        content: '1480px',
      },
      spacing: {
        sidebar: '232px',
        'sidebar-collapsed': '60px',
      },
    },
  },
  plugins: [],
}
