<template>
  <div>
    <h2>总览</h2>
    <h3 style="margin: 12px 0 8px">连接状态</h3>
    <el-table :data="connections" size="small" border>
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="mode" label="模式" width="80" />
      <el-table-column prop="addr" label="地址" />
      <el-table-column label="状态" width="120">
        <template #default="{ row }">
          <el-tag v-if="!row.enabled" type="info" size="small">已停用</el-tag>
          <el-tag v-else-if="row.healthy === null" type="warning" size="small">等待数据</el-tag>
          <el-tag v-else-if="row.healthy" type="success" size="small">正常</el-tag>
          <el-tag v-else type="danger" size="small">异常</el-tag>
        </template>
      </el-table-column>
    </el-table>

    <h3 style="margin: 20px 0 8px">最新读数（{{ readings.length }} 个点位，3 秒自动刷新）</h3>
    <el-table :data="readings" size="small" border>
      <el-table-column prop="connection" label="连接" width="140" />
      <el-table-column prop="pointName" label="点位" width="160" />
      <el-table-column label="值" width="140">
        <template #default="{ row }">
          <span v-if="row.ok" style="font-family: monospace">{{ row.value }} {{ row.unit }}</span>
          <el-text v-else type="danger" size="small">读取失败</el-text>
        </template>
      </el-table-column>
      <el-table-column label="时间" width="180">
        <template #default="{ row }">{{ fmtTime(row.timestamp) }}</template>
      </el-table-column>
      <el-table-column prop="error" label="错误" show-overflow-tooltip />
    </el-table>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { api, fmtTime } from '../api'

const connections = ref([])
const readings = ref([])
let timer = null

async function load() {
  try {
    const data = await api.get('/api/values')
    connections.value = data.connections.map((c) => ({ ...c, healthy: c.healthy === undefined ? null : c.healthy }))
    readings.value = data.readings
  } catch (e) {
    /* 网络闪断不打断页面，下一轮再试 */
  }
}
onMounted(() => {
  load()
  timer = setInterval(load, 3000)
})
onUnmounted(() => clearInterval(timer))
</script>
