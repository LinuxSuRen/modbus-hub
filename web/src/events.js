// 常驻 SSE 读数流：连接按监听者计数复用，浏览器自动重连，最后一个监听者离开才关闭。
// 常驻流清理语义：页面卸载时调用返回的取消函数即可，onerror 空处理交给浏览器重连。
let es = null
let refs = 0
const listeners = new Set()

function ensure() {
  if (es) return
  es = new EventSource('/api/events')
  es.addEventListener('reading', (e) => {
    try {
      const rd = JSON.parse(e.data)
      listeners.forEach((fn) => fn(rd))
    } catch (_) {
      // 单条坏消息不能杀整条流，静默忽略
    }
  })
  es.onerror = () => {} // 常驻流：交给浏览器自动重连
}

export function onReading(fn) {
  listeners.add(fn)
  refs++
  ensure()
  return () => {
    listeners.delete(fn)
    refs--
    if (refs <= 0 && es) {
      es.close()
      es = null
    }
  }
}
