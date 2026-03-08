import type { ClientEvent, ServerEvent } from '../types/events'

type Callbacks = {
  onOpen: () => void
  onClose: () => void
  onEvent: (event: ServerEvent) => void
  onError: (message: string) => void
}

export class ConsoleSocket {
  private ws: WebSocket | null = null

  connect(sessionId: string, callbacks: Callbacks): void {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${protocol}//${window.location.host}/api/web-console/ws?session_id=${encodeURIComponent(sessionId)}`
    this.ws = new WebSocket(url)

    this.ws.onopen = () => callbacks.onOpen()
    this.ws.onclose = () => callbacks.onClose()
    this.ws.onerror = () => callbacks.onError('websocket error')
    this.ws.onmessage = (message) => {
      try {
        const event = JSON.parse(message.data) as ServerEvent
        callbacks.onEvent(event)
      } catch {
        callbacks.onError('invalid websocket payload')
      }
    }
  }

  send(event: ClientEvent): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(event))
    }
  }

  isOpen(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  disconnect(): void {
    this.ws?.close()
    this.ws = null
  }
}
