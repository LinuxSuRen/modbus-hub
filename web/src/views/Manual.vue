<template>
  <div>
    <h2>手动读写</h2>
    <el-card shadow="never">
      <template #header>写入</template>
      <el-form label-width="110px" style="max-width: 460px">
        <el-form-item label="连接">
          <el-select v-model="writeForm.connectionId">
            <el-option v-for="c in connections" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="功能码">
          <el-radio-group v-model="writeForm.functionCode">
            <el-radio :value="5">FC5 线圈</el-radio>
            <el-radio :value="6">FC6 单寄存器</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="地址">
          <el-input-number v-model="writeForm.address" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item :label="writeForm.functionCode === 5 ? '值（开关）' : '值（0-65535）'">
          <el-switch v-if="writeForm.functionCode === 5" v-model="writeForm.boolValue" />
          <el-input-number v-else v-model="writeForm.value" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="doWrite">写入</el-button>
        </el-form-item>
      </el-form>
    </el-card>
    <el-card shadow="never" style="margin-top: 16px">
      <template #header>读取（走已配置的点位，立即读一次）</template>
      <el-form label-width="110px" style="max-width: 460px" inline>
        <el-form-item label="点位">
          <el-select v-model="readPointId" filterable style="width: 240px">
            <el-option v-for="p in points" :key="p.id" :label="`${p.name}（${connName(p.connectionId)}）`" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button @click="doRead">读取</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="readResult" :type="readResult.ok ? 'success' : 'error'" :closable="false"
        :title="readResult.ok ? `${readResult.pointName} = ${readResult.value} ${readResult.unit}` : `读取失败：${readResult.error}`" />
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const connections = ref([])
const points = ref([])
const writeForm = ref({ connectionId: null, functionCode: 6, address: 0, value: 0, boolValue: false })
const readPointId = ref(null)
const readResult = ref(null)

onMounted(async () => {
  connections.value = await api.get('/api/connections')
  points.value = await api.get('/api/points')
  if (connections.value.length && writeForm.value.connectionId === null) {
    writeForm.value.connectionId = connections.value[0].id
  }
})

function connName(id) {
  const c = connections.value.find((c) => c.id === id)
  return c ? c.name : id
}

async function doWrite() {
  const f = writeForm.value
  const value = f.functionCode === 5 ? (f.boolValue ? 1 : 0) : f.value
  const r = await api.post('/api/write', {
    connectionId: f.connectionId, functionCode: f.functionCode, address: f.address, value,
  })
  if (r.ok) ElMessage.success('写入成功')
  else ElMessage.error(`写入失败：${r.error}`)
}
async function doRead() {
  if (!readPointId.value) return
  readResult.value = await api.post(`/api/points/${readPointId.value}/read`)
}
</script>
