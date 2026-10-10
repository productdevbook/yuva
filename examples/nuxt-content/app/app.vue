<script setup lang="ts">
const { data: navigation } = await useAsyncData("navigation", () => queryCollectionNavigation("content"));
</script>

<template>
  <div class="site">
    <nav>
      <strong>Acme Docs</strong>
      <NuxtLink v-for="item in navigation" :key="item.path" :to="item.path">{{ item.title }}</NuxtLink>
    </nav>
    <main>
      <NuxtPage />
    </main>
  </div>
</template>

<style>
:root {
  color-scheme: light dark;
  font-family: system-ui, sans-serif;
  line-height: 1.6;
}
body {
  margin: 0;
}
.site {
  display: flex;
  flex-wrap: wrap;
  gap: 2rem;
  max-width: 64rem;
  margin: 0 auto;
  padding: 2rem 1rem;
}
nav {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 10rem;
}
nav a.router-link-exact-active {
  font-weight: 600;
}
main {
  flex: 1;
  min-width: 0;
}
pre {
  overflow-x: auto;
  padding: 1rem;
  border-radius: 0.5rem;
  background: color-mix(in srgb, currentColor 6%, transparent);
}
table {
  border-collapse: collapse;
}
th,
td {
  padding: 0.25rem 0.75rem;
  border: 1px solid color-mix(in srgb, currentColor 20%, transparent);
}
yuva-page-feedback,
yuva-page-questions {
  margin-block: 2rem;
}
</style>
