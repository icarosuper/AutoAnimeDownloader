import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  server: {
    port: parseInt(process.env.VITE_PORT || '5173', 10)
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  },
  base: './'
})

