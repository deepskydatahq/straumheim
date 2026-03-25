import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://straumheim.com',
  integrations: [
    starlight({
      title: 'Straumheim',
      favicon: '/favicon.svg',
      customCss: ['./src/styles/custom.css'],
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/deepskydatahq/straumheim' },
      ],
    }),
  ],
});
