<template>
  <el-container style="min-height: 100vh">
    <el-aside width="200px" style="border-right: 1px solid var(--el-border-color-light)">
      <div style="padding: 16px; font-size: 18px; font-weight: 600">modbus-hub</div>
      <el-menu :default-active="page" @select="(k) => (page = k)">
        <el-menu-item index="overview">总览</el-menu-item>
        <el-menu-item index="connections">连接管理</el-menu-item>
        <el-menu-item index="points">点位管理</el-menu-item>
        <el-menu-item index="manual">手动读写</el-menu-item>
        <el-menu-item index="sinks">转发设置</el-menu-item>
      </el-menu>
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

const page = ref('overview')

function reloadHint() {
  ElMessage.success('已保存')
}
</script>
