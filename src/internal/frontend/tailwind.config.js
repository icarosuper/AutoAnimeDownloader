import daisyui from 'daisyui'

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  darkMode: ['class', '[data-theme="night"]'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', 'sans-serif'],
      },
    },
  },
  plugins: [daisyui],
  daisyui: {
    themes: ['light', 'night'],
    darkTheme: 'night',
    base: true,
    styled: true,
    utils: true,
    logs: false,
  },
}

