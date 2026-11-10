<template>
  <div>
    <h2>转发设置</h2>

    <el-card shadow="never">
      <template #header>
        MQTT 转发
        <el-tag v-if="status.mqtt && status.mqtt.running" type="success" size="small" style="margin-left: 8px">运行中</el-tag>
        <el-tag v-else type="info" size="small" style="margin-left: 8px">未运行</el-tag>
      </template>
      <el-form label-width="120px" style="max-width: 520px">
        <el-form-item label="Broker">
          <el-input v-model="mqtt.broker" placeholder="tcp://127.0.0.1:1883" />
        </el-form-item>
        <el-form-item label="Client ID">
          <el-input v-model="mqtt.clientId" placeholder="modbus-hub" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="mqtt.username" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="mqtt.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="Topic 前缀">
          <el-input v-model="mqtt.topicPrefix" placeholder="modbus" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="mqtt.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveMQTT">保存并应用</el-button>
        </el-form-item>
      </el-form>
      <el-text type="info" size="small">
        读数发布到 {前缀}/{连接名}/{点位名}，QoS1、不 retain；payload 为 JSON（value/unit/timestamp）。
      </el-text>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        TCP 转发（JSON 行流）
        <el-tag v-if="status.tcp && status.tcp.running" type="success" size="small" style="margin-left: 8px">
          运行中 · {{ status.tcp.clients }} 个客户端
        </el-tag>
        <el-tag v-else type="info" size="small" style="margin-left: 8px">未运行</el-tag>
      </template>
      <el-form label-width="120px" style="max-width: 520px">
        <el-form-item label="监听地址">
          <el-input v-model="tcp.listen" placeholder=":9000" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="tcp.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveTCP">保存并应用</el-button>
        </el-form-item>
      </el-form>
      <el-text type="info" size="small">
        每条读数一行 JSON 推给所有已连接客户端；nc &lt;ip&gt; &lt;port&gt; 即可订阅。
      </el-text>
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const mqtt = ref({})
const tcp = ref({})
const status = ref({})

async function load() {
  mqtt.value = await api.get('/api/sinks/mqtt')
  tcp.value = await api.get('/api/sinks/tcp')
  status.value = await api.get('/api/sinks/status')
}
onMounted(load)

async function saveMQTT() {
  const r = await api.put('/api/sinks/mqtt', mqtt.value)
  if (r.error) ElMessage.error(`已保存但启动失败：${r.error}`)
  else ElMessage.success('已应用')
  await load()
}
async function saveTCP() {
  const r = await api.put('/api/sinks/tcp', tcp.value)
  if (r.error) ElMessage.error(`已保存但启动失败：${r.error}`)
  else ElMessage.success('已应用')
  await load()
}
</script>
