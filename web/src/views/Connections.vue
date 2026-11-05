<template>
  <div>
    <h2>连接管理</h2>
    <el-button type="primary" @click="openNew">新建连接</el-button>
    <el-table :data="rows" size="small" border style="margin-top: 12px">
      <el-table-column prop="name" label="名称" width="140" />
      <el-table-column prop="mode" label="模式" width="80" />
      <el-table-column prop="addr" label="地址" />
      <el-table-column prop="unitId" label="从站" width="70" />
      <el-table-column label="启用" width="70">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v) => toggle(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="健康" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.status && row.status.healthy" type="success" size="small">正常</el-tag>
          <el-tag v-else-if="row.status && row.status.lastError" type="danger" size="small">异常</el-tag>
          <el-tag v-else type="info" size="small">—</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240">
        <template #default="{ row }">
          <el-button size="small" @click="test(row)">测试</el-button>
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-popconfirm title="删除该连接及其全部点位？" @confirm="remove(row)">
            <template #reference><el-button size="small" type="danger">删除</el-button></template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialog" :title="form.id ? '编辑连接' : '新建连接'" width="520">
      <el-form label-width="110px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如 plc1" />
        </el-form-item>
        <el-form-item label="模式">
          <el-radio-group v-model="form.mode">
            <el-radio value="tcp">Modbus TCP</el-radio>
            <el-radio value="rtu">串口 RTU</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="form.mode === 'tcp' ? '地址' : '串口设备'">
          <el-input v-model="form.addr" :placeholder="form.mode === 'tcp' ? '192.168.1.10:502' : '/dev/ttyUSB0 或 COM3'" />
        </el-form-item>
        <el-form-item label="从站地址">
          <el-input-number v-model="form.unitId" :min="0" :max="247" />
        </el-form-item>
        <template v-if="form.mode === 'rtu'">
          <el-form-item label="波特率">
            <el-select v-model="form.baud" style="width: 140px">
              <el-option v-for="b in [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200]" :key="b" :label="String(b)" :value="b" />
            </el-select>
          </el-form-item>
          <el-form-item label="数据位">
            <el-input-number v-model="form.dataBits" :min="5" :max="8" />
          </el-form-item>
          <el-form-item label="校验">
            <el-radio-group v-model="form.parity">
              <el-radio value="N">无</el-radio>
              <el-radio value="E">偶</el-radio>
              <el-radio value="O">奇</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="停止位">
            <el-input-number v-model="form.stopBits" :min="1" :max="2" />
          </el-form-item>
        </template>
        <el-form-item label="超时(ms)">
          <el-input-number v-model="form.timeoutMs" :min="100" :max="10000" :step="100" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api'

const emit = defineEmits(['changed'])
const rows = ref([])
const dialog = ref(false)
const emptyForm = () => ({
  id: 0, name: '', mode: 'tcp', addr: '', unitId: 1,
  baud: 9600, dataBits: 8, parity: 'N', stopBits: 1, timeoutMs: 1000, enabled: true,
})
const form = ref(emptyForm())

async function load() {
  rows.value = await api.get('/api/connections')
}
onMounted(load)

function openNew() {
  form.value = emptyForm()
  dialog.value = true
}
function openEdit(row) {
  form.value = { ...row }
  dialog.value = true
}
async function save() {
  try {
    if (form.value.id) {
      await api.put(`/api/connections/${form.value.id}`, form.value)
    } else {
      await api.post('/api/connections', form.value)
    }
    dialog.value = false
    emit('changed')
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  }
}
async function toggle(row, v) {
  row.enabled = v
  try {
    await api.put(`/api/connections/${row.id}`, row)
    emit('changed')
  } catch (e) {
    ElMessage.error(e.message)
  }
}
async function test(row) {
  const r = await api.post(`/api/connections/${row.id}/test`)
  if (r.ok) ElMessage.success('连接成功')
  else ElMessage.error(`连接失败：${r.error}`)
}
async function remove(row) {
  await api.del(`/api/connections/${row.id}`)
  emit('changed')
  await load()
}
</script>
