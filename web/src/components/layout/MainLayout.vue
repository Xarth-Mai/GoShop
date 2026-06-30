<script setup lang="ts">
import TopNav from './TopNav.vue'
import Footer from './Footer.vue'
</script>

<template>
  <div class="layout-wrapper">
    <TopNav />
    <main class="main-content">
      <router-view></router-view>
    </main>
    <Footer />
  </div>
</template>

<style scoped>
.layout-wrapper {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.main-content {
  flex: 1 0 auto;
  display: flex;
  flex-direction: column;
  background-color: var(--colors-canvas);
}
</style>
