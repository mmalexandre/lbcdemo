import { useState, useRef, useEffect } from 'react'
import './App.css'

// In production (S3/CloudFront) the app is served from a different origin than
// the API (ALB).  VITE_API_URL is injected at build time by GitHub Actions.
// In dev it is empty so requests fall through to the Vite proxy.
declare const __API_URL__: string
const API = typeof __API_URL__ !== 'undefined' ? __API_URL__ : ''

interface Message {
  role: 'user' | 'assistant'
  text: string
}

function LoginPage({ onLogin }: { onLogin: (username: string) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await fetch(`${API}/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ username, password }),
      })
      if (res.ok) {
        const data = await res.json()
        onLogin(data.username)
      } else {
        setError('Invalid username or password.')
      }
    } catch {
      setError('Could not reach the server.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <h1 className="login-title">Sign in</h1>
        <div className="login-field">
          <label htmlFor="username">Username</label>
          <input
            id="username"
            type="text"
            autoComplete="username"
            value={username}
            onChange={e => setUsername(e.target.value)}
            required
          />
        </div>
        <div className="login-field">
          <label htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            required
          />
        </div>
        {error && <p className="login-error">{error}</p>}
        <button className="login-btn" type="submit" disabled={loading}>
          {loading ? 'Signing in…' : 'Login'}
        </button>
      </form>
    </div>
  )
}

function ProfileMenu({ username, onLogout }: { username: string; onLogout: () => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  return (
    <div className="profile-menu" ref={ref}>
      <button className="profile-btn" onClick={() => setOpen(o => !o)} aria-haspopup="true" aria-expanded={open}>
        <span className="profile-avatar">{username.charAt(0).toUpperCase()}</span>
        <span className="profile-username">{username}</span>
      </button>
      {open && (
        <div className="profile-dropdown">
          <button
            className="profile-dropdown-item"
            onClick={() => { setOpen(false); onLogout() }}
          >
            Log out
          </button>
        </div>
      )}
    </div>
  )
}

function ChatPage({ username, onLogout }: { username: string; onLogout: () => void }) {
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
      const res = await fetch(`${API}/prompt`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ prompt: text }),
      })
      if (res.status === 401) {
        onLogout()
        return
      }
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
        <div className="chat-header">
          <ProfileMenu username={username} onLogout={onLogout} />
        </div>

        <div className="messages">
          <div className="message-row info">
            <div className="bubble info-bubble">
              This is a magician. Ask anything and you will get the rules of a new cards game! View registered prompt in{' '}
              <a href="https://d13vttbhe09whf.cloudfront.net/mlflow" target="_blank" rel="noreferrer">
                MLFlow
              </a>
              . The chat has no memory and each reply is currently independent.
            </div>
          </div>
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
            placeholder="Type your message… (Enter to send, Shift+Enter for newline)"
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

function App() {
  const [username, setUsername] = useState<string | null>(null)
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    fetch(`${API}/me`, { credentials: 'include' })
      .then(res => res.ok ? res.json() : null)
      .then(data => {
        if (data?.username) setUsername(data.username)
      })
      .finally(() => setChecking(false))
  }, [])

  const handleLogout = async () => {
    await fetch(`${API}/logout`, { method: 'POST', credentials: 'include' })
    setUsername(null)
  }

  if (checking) return null

  if (!username) {
    return <LoginPage onLogin={setUsername} />
  }

  return <ChatPage username={username} onLogout={handleLogout} />
}

export default App

