import { useState, useRef, useEffect } from 'react'
import './App.css'

interface Message {
  role: 'user' | 'assistant'
  text: string
}

function App() {
  const [messages, setMessages] = useState<Message[]>([])
  const [prompt, setPrompt] = useState('')
  const [loading, setLoading] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const sendPrompt = async () => {
    const text = prompt.trim()
    if (!text || loading) return

    setMessages(prev => [...prev, { role: 'user', text }])
    setPrompt('')
    setLoading(true)

    try {
      const res = await fetch('http://localhost:8080/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: text }),
      })
      const data = await res.json()
      setMessages(prev => [...prev, { role: 'assistant', text: data.reply }])
    } catch {
      setMessages(prev => [...prev, { role: 'assistant', text: 'Error: could not reach the API.' }])
    } finally {
      setLoading(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendPrompt()
    }
  }

  return (
    <div className="app">
      <div className="chat-container">
        <div className="messages">
          {messages.map((msg, i) => (
            <div key={i} className={`message-row ${msg.role}`}>
              <div className="bubble">{msg.text}</div>
            </div>
          ))}
          {loading && (
            <div className="message-row assistant">
              <div className="bubble thinking">...</div>
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        <div className="input-area">
          <textarea
            value={prompt}
            onChange={e => setPrompt(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type your message... (Enter to send, Shift+Enter for newline)"
            rows={4}
          />
          <button onClick={sendPrompt} disabled={loading || !prompt.trim()}>
            Send
          </button>
        </div>
      </div>
    </div>
  )
}

export default App
