// fetch 封装：统一 JSON、错误提示与 query 拼接
async function request(method, path, body) {
  const opts = { method, headers: {} }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const resp = await fetch(path, opts)
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    throw new Error(data.error || `请求失败（${resp.status}）`)
  }
  return data
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body),
  put: (path, body) => request('PUT', path, body),
  del: (path) => request('DELETE', path),
}

// 展示与数据分离：后端只回 code，前端负责文案
export const fcText = { 1: '线圈（FC1）', 2: '离散输入（FC2）', 3: '保持寄存器（FC3）', 4: '输入寄存器（FC4）' }
export const dataTypeText = {
  uint16: 'uint16', int16: 'int16', uint32: 'uint32', int32: 'int32',
  float32: 'float32', float32sw: 'float32（字交换）', bool: 'bool', raw: 'raw',
}

export function fmtTime(v) {
  if (!v) return '—'
  return new Date(v).toLocaleString('zh-CN', { hour12: false })
}
