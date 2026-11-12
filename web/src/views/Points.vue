<template>
  <div>
    <h2>点位管理</h2>
    <el-select v-model="connFilter" clearable placeholder="按连接筛选" style="width: 200px" @change="load">
      <el-option v-for="c in connections" :key="c.id" :label="c.name" :value="c.id" />
    </el-select>
    <el-button type="primary" style="margin-left: 8px" @click="openNew">新建点位</el-button>
    <el-table :data="rows" size="small" border style="margin-top: 12px">
      <el-table-column prop="name" label="点位名" width="140" />
      <el-table-column label="连接" width="120">
        <template #default="{ row }">{{ connName(row.connectionId) }}</template>
      </el-table-column>
      <el-table-column label="功能码" width="120">
        <template #default="{ row }">{{ fcText[row.functionCode] || row.functionCode }}</template>
      </el-table-column>
      <el-table-column prop="address" label="地址" width="70" />
      <el-table-column prop="quantity" label="数量" width="70" />
      <el-table-column label="类型" width="110">
        <template #default="{ row }">{{ dataTypeText[row.dataType] || row.dataType }}</template>
      </el-table-column>
      <el-table-column label="变换" width="150">
        <template #default="{ row }">
          ×{{ row.scale }} +{{ row.offset }}<template v-if="row.precision">（{{ row.precision }} 位小数）</template>
        </template>
      </el-table-column>
      <el-table-column prop="unit" label="单位" width="70" />
      <el-table-column label="实时值" min-width="150">
        <template #default="{ row }">
          <template v-if="live[row.id]">
            <el-text v-if="live[row.id].ok" type="success">
              {{ live[row.id].value }} {{ row.unit }}
            </el-text>
            <el-tooltip v-else :content="live[row.id].error" placement="top">
              <el-text type="danger">读取失败</el-text>
            </el-tooltip>
            <div>
              <el-text size="small" type="info">{{ fmtTime(live[row.id].timestamp) }}</el-text>
            </div>
          </template>
          <el-text v-else size="small" type="info">—</el-text>
        </template>
      </el-table-column>
      <el-table-column prop="pollMs" label="周期(ms)" width="90" />
      <el-table-column label="启用" width="70">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v) => toggle(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button size="small" @click="readNow(row)">读取</el-button>
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-popconfirm title="删除该点位？" @confirm="remove(row)">
            <template #reference><el-button size="small" type="danger">删除</el-button></template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialog" :title="form.id ? '编辑点位' : '新建点位'" width="520">
      <el-form label-width="110px">
        <el-form-item label="点位名" required>
          <el-input v-model="form.name" placeholder="如 温度1" />
        </el-form-item>
        <el-form-item label="连接" required>
          <el-select v-model="form.connectionId">
            <el-option v-for="c in connections" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="功能码">
          <el-select v-model="form.functionCode" style="width: 200px">
            <el-option v-for="(v, k) in fcText" :key="k" :label="v" :value="Number(k)" />
          </el-select>
        </el-form-item>
        <el-form-item label="起始地址">
          <el-input-number v-model="form.address" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item label="数量">
          <el-input-number v-model="form.quantity" :min="1" :max="125" />
        </el-form-item>
        <el-form-item label="数据类型">
          <el-select v-model="form.dataType" style="width: 200px">
            <el-option v-for="(v, k) in dataTypeText" :key="k" :label="v" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="倍率">
          <el-input-number v-model="form.scale" :step="0.1" />
        </el-form-item>
        <el-form-item label="偏移">
          <el-input-number v-model="form.offset" :step="1" />
        </el-form-item>
        <el-form-item label="小数位">
          <el-input-number v-model="form.precision" :min="0" :max="9" />
          <el-text size="small" type="info" style="margin-left: 8px">0 = 不处理</el-text>
        </el-form-item>
        <el-form-item label="单位">
          <el-input v-model="form.unit" placeholder="℃ / kPa / ..." style="width: 120px" />
        </el-form-item>
        <el-form-item label="轮询周期(ms)">
          <el-input-number v-model="form.pollMs" :min="100" :max="600000" :step="100" />
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
import { onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, fcText, dataTypeText, fmtTime } from '../api'
import { onReading } from '../events'

const rows = ref([])
const connections = ref([])
const connFilter = ref(null)
const dialog = ref(false)

// 实时读数（SSE）：pointId -> 最新 Reading
const live = ref({})
const offReading = onReading((rd) => {
  live.value = { ...live.value, [rd.pointId]: rd }
})
onUnmounted(offReading)
const emptyForm = () => ({
  id: 0, name: '', connectionId: null, functionCode: 3, address: 0, quantity: 1,
  dataType: 'uint16', scale: 1, offset: 0, precision: 0, unit: '', pollMs: 1000, enabled: true,
})
const form = ref(emptyForm())

async function load() {
  const q = connFilter.value ? `?connection=${connFilter.value}` : ''
  rows.value = await api.get(`/api/points${q}`)
}
onMounted(async () => {
  connections.value = await api.get('/api/connections')
  await load()
})

function connName(id) {
  const c = connections.value.find((c) => c.id === id)
  return c ? c.name : id
}
function openNew() {
  form.value = emptyForm()
  if (connFilter.value) form.value.connectionId = connFilter.value
  else if (connections.value.length) form.value.connectionId = connections.value[0].id
  dialog.value = true
}
function openEdit(row) {
  form.value = { ...row }
  dialog.value = true
}
async function save() {
  try {
    if (form.value.id) await api.put(`/api/points/${form.value.id}`, form.value)
    else await api.post('/api/points', form.value)
    dialog.value = false
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  }
}
async function toggle(row, v) {
  row.enabled = v
  try {
    await api.put(`/api/points/${row.id}`, row)
  } catch (e) {
    ElMessage.error(e.message)
  }
}
async function readNow(row) {
  const r = await api.post(`/api/points/${row.id}/read`)
  if (r.ok) ElMessage.success(`${row.name} = ${r.value} ${row.unit}`)
  else ElMessage.error(`读取失败：${r.error}`)
}
async function remove(row) {
  await api.del(`/api/points/${row.id}`)
  await load()
}
</script>
