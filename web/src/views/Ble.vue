<template>
  <div>
    <h2>BLE 模块</h2>
    <el-text type="info">扫描周边低功耗蓝牙透传模块（如 E104-BT12NSP），并直接下发 AT 指令完成参数设置，无需拆线接 USB。</el-text>

    <el-card style="margin-top: 12px">
      <template #header>
        <div style="display: flex; align-items: center; gap: 12px">
          <span>设备扫描</span>
          <el-input v-model="filter" placeholder="按名称/地址过滤，可留空" style="width: 240px" size="small" />
          <el-button type="primary" size="small" :loading="scanning" @click="scan">扫描</el-button>
        </div>
      </template>
      <el-table :data="devices" size="small" border highlight-current-row @current-change="pick">
        <el-table-column prop="address" label="地址" width="200" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="rssi" label="信号 (dBm)" width="110" />
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button size="small" @click="pick(row)">选用</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card style="margin-top: 12px">
      <template #header>AT 指令设置</template>
      <el-form label-width="110px">
        <el-form-item label="设备地址" required>
          <el-input v-model="form.addr" placeholder="AA:BB:CC:DD:EE:FF" style="width: 240px" />
        </el-form-item>
        <el-form-item label="服务 UUID">
          <el-input v-model="form.serviceUUID" placeholder="默认 FFE0" style="width: 220px" />
        </el-form-item>
        <el-form-item label="写/通知 UUID">
          <el-input v-model="form.writeUUID" placeholder="默认 FFE1" style="width: 220px" />
          <el-text size="small" type="info" style="margin-left: 8px">写与通知同为 FFE1 时通知 UUID 留空即可</el-text>
        </el-form-item>
        <el-form-item label="AT 指令">
          <el-input v-model="form.command" placeholder="如 AT+NAME" style="width: 260px" />
          <el-select v-model="form.terminator" style="width: 120px; margin-left: 8px">
            <el-option value="" label="\\r\\n（默认）" />
            <el-option value="\\r" label="\\r" />
            <el-option value="\\n" label="\\n" />
          </el-select>
        </el-form-item>
        <el-form-item label=" ">
          <el-space wrap>
            <el-button v-for="p in presets" :key="p" size="small" @click="form.command = p">{{ p }}</el-button>
          </el-space>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="sending" :disabled="!form.addr || !form.command" @click="send">发送</el-button>
          <el-text size="small" type="info" style="margin-left: 8px">
            预设指令请以所用模块的用户手册为准；发送即临时建链，发送完成后自动断开
          </el-text>
        </el-form-item>
        <el-form-item v-if="response" label="模块应答">
          <el-input type="textarea" :model-value="response" readonly :rows="4" style="width: 420px; font-family: monospace" />
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const devices = ref([])
const filter = ref('')
const scanning = ref(false)
const sending = ref(false)
const response = ref('')
const form = ref({ addr: '', serviceUUID: '', writeUUID: '', notifyUUID: '', command: '', terminator: '', timeoutMs: 3000 })

// 预设指令：以 E104-BT12 系列常见 AT 指令为准，手册不同时可自行输入
const presets = ['AT+VER?', 'AT+NAME?', 'AT+BAUD?', 'AT+RESET']

async function scan() {
  scanning.value = true
  try {
    devices.value = await api.get(`/api/ble/scan?duration=4&filter=${encodeURIComponent(filter.value)}`)
    if (!devices.value.length) ElMessage.warning('未扫描到 BLE 设备，请确认模块已上电并在广播')
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    scanning.value = false
  }
}

function pick(row) {
  if (!row) return
  form.value.addr = row.address
  ElMessage.success(`已选用 ${row.address}`)
}

async function send() {
  sending.value = true
  response.value = ''
  try {
    const r = await api.post('/api/ble/at', {
      addr: form.value.addr,
      serviceUUID: form.value.serviceUUID,
      writeUUID: form.value.writeUUID,
      notifyUUID: form.value.notifyUUID,
      command: form.value.command,
      terminator: form.value.terminator,
      timeoutMs: form.value.timeoutMs,
    })
    response.value = r.response
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    sending.value = false
  }
}
</script>
