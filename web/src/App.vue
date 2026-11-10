<template>
  <el-container style="min-height: 100vh">
    <el-aside width="200px" style="border-right: 1px solid var(--el-border-color-light); position: relative">
      <div style="padding: 16px; display: flex; align-items: center; gap: 10px">
        <img src="/logo.svg" width="32" height="32" alt="modbus-hub logo" />
        <span style="font-size: 18px; font-weight: 600">modbus-hub</span>
      </div>
      <el-menu :default-active="page" @select="selectPage">
        <el-menu-item index="overview">总览</el-menu-item>
        <el-menu-item index="connections">连接管理</el-menu-item>
        <el-menu-item index="points">点位管理</el-menu-item>
        <el-menu-item index="manual">手动读写</el-menu-item>
        <el-menu-item index="sinks">转发设置</el-menu-item>
      </el-menu>
      <a href="https://github.com/linuxsuren/modbus-hub" target="_blank" rel="noreferrer"
        style="position: absolute; bottom: 16px; left: 0; right: 0; display: flex; align-items: center; justify-content: center; gap: 6px; color: inherit; text-decoration: none; font-size: 14px">
        <svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true">
          <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/>
        </svg>
        GitHub
      </a>
    </el-aside>
    <el-main>
      <Overview v-if="page === 'overview'" />
      <Connections v-else-if="page === 'connections'" @changed="reloadHint" />
      <Points v-else-if="page === 'points'" />
      <Manual v-else-if="page === 'manual'" />
      <Sinks v-else-if="page === 'sinks'" />
    </el-main>
  </el-container>
</template>

<script setup>
import { ref, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import Overview from './views/Overview.vue'
import Connections from './views/Connections.vue'
import Points from './views/Points.vue'
import Manual from './views/Manual.vue'
import Sinks from './views/Sinks.vue'

// 菜单选中状态落在 URL hash（#/points）：刷新页面、浏览器前进/后退都保持当前页
const pages = ['overview', 'connections', 'points', 'manual', 'sinks']

function pageFromHash() {
  const h = location.hash.replace(/^#\/?/, '')
  return pages.includes(h) ? h : 'overview'
}

const page = ref(pageFromHash())

function selectPage(k) {
  page.value = k
  location.hash = '#/' + k
}

window.addEventListener('hashchange', () => {
  page.value = pageFromHash()
})

function reloadHint() {
  ElMessage.success('已保存')
}
</script>
