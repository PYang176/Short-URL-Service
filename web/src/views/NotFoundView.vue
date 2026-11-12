<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <main class="not-found-page">
    <p class="section-kicker">404</p>
    <h1>这个地址没有对应的页面。</h1>
    <p>短链接跳转使用 `/r/{slug}`，管理页面则从工作台进入。</p>
    <RouterLink class="primary-button" to="/dashboard">返回工作台</RouterLink>
  </main>
</template>
