import { fileURLToPath } from 'node:url';
import { createMDX } from 'fumadocs-mdx/next';

const withMDX = createMDX();

/** @type {import('next').NextConfig} */
const config = {
  reactStrictMode: true,
  // useyuva is linked from ../../sdk/js, which Turbopack resolves only inside its root.
  turbopack: {
    root: fileURLToPath(new URL('../..', import.meta.url)),
  },
};

export default withMDX(config);
