import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from '../App'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function mockFetch(responses: Record<string, { ok: boolean; status?: number; json: () => Promise<unknown> }>) {
  return vi.fn((url: string) => {
    const key = Object.keys(responses).find(k => url.includes(k))
    if (!key) return Promise.reject(new Error(`Unhandled fetch: ${url}`))
    const r = responses[key]
    return Promise.resolve({
      ok: r.ok,
      status: r.status ?? (r.ok ? 200 : 400),
      json: r.json,
    })
  })
}

beforeEach(() => {
  vi.restoreAllMocks()
})

// ---------------------------------------------------------------------------
// LoginPage
// ---------------------------------------------------------------------------

describe('LoginPage', () => {
  it('renders sign-in form when not authenticated', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: false, status: 401, json: async () => ({}) },
    }))

    render(<App />)
    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
  })

  it('shows error message on invalid credentials', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: false, status: 401, json: async () => ({}) },
      '/login': { ok: false, status: 401, json: async () => ({}) },
    }))

    render(<App />)
    await screen.findByRole('heading', { name: /sign in/i })

    await userEvent.type(screen.getByLabelText(/username/i), 'baduser')
    await userEvent.type(screen.getByLabelText(/password/i), 'badpass')
    fireEvent.submit(screen.getByRole('button', { name: /login/i }))

    expect(await screen.findByText(/invalid username or password/i)).toBeInTheDocument()
  })

  it('shows network error when server is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/me')) return Promise.resolve({ ok: false, status: 500, json: async () => ({}) })
      return Promise.reject(new Error('Network error'))
    }))

    render(<App />)
    await screen.findByRole('heading', { name: /sign in/i })

    await userEvent.type(screen.getByLabelText(/username/i), 'user')
    await userEvent.type(screen.getByLabelText(/password/i), 'pass')
    fireEvent.submit(screen.getByRole('button', { name: /login/i }))

    expect(await screen.findByText(/could not reach the server/i)).toBeInTheDocument()
  })

  it('transitions to chat page on successful login', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: false, status: 401, json: async () => ({}) },
      '/login': { ok: true, json: async () => ({ username: 'alice' }) },
    }))

    render(<App />)
    await screen.findByRole('heading', { name: /sign in/i })

    await userEvent.type(screen.getByLabelText(/username/i), 'alice')
    await userEvent.type(screen.getByLabelText(/password/i), 'secret')
    fireEvent.submit(screen.getByRole('button', { name: /login/i }))

    expect(await screen.findByPlaceholderText(/type your message/i)).toBeInTheDocument()
  })

  it('disables button while signing in', async () => {
    let resolveFetch!: (value: unknown) => void
    const pending = new Promise(r => { resolveFetch = r })

    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/me')) return Promise.resolve({ ok: false, status: 401, json: async () => ({}) })
      return pending
    }))

    render(<App />)
    await screen.findByRole('heading', { name: /sign in/i })

    await userEvent.type(screen.getByLabelText(/username/i), 'alice')
    await userEvent.type(screen.getByLabelText(/password/i), 'secret')
    fireEvent.submit(screen.getByRole('button', { name: /login/i }))

    expect(await screen.findByRole('button', { name: /signing in/i })).toBeDisabled()

    resolveFetch({ ok: false, status: 401, json: async () => ({}) })
  })
})

// ---------------------------------------------------------------------------
// App – session check (/me)
// ---------------------------------------------------------------------------

describe('App session check', () => {
  it('renders nothing while checking session', () => {
    let resolve!: (value: unknown) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise(r => { resolve = r })))

    const { container } = render(<App />)
    expect(container.firstChild).toBeNull()
    resolve({ ok: false, status: 401, json: async () => ({}) })
  })

  it('skips login and shows chat when already authenticated', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: true, json: async () => ({ username: 'bob' }) },
    }))

    render(<App />)
    expect(await screen.findByPlaceholderText(/type your message/i)).toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// ChatPage
// ---------------------------------------------------------------------------

describe('ChatPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: true, json: async () => ({ username: 'alice' }) },
    }))
  })

  it('displays username in profile button', async () => {
    render(<App />)
    expect(await screen.findByText('alice')).toBeInTheDocument()
  })

  it('sends a message and renders the assistant reply', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: true, json: async () => ({ username: 'alice' }) },
      '/prompt': { ok: true, json: async () => ({ reply: 'Hello from AI!' }) },
    }))

    render(<App />)
    const textarea = await screen.findByPlaceholderText(/type your message/i)

    await userEvent.type(textarea, 'Hi there')
    await userEvent.click(screen.getByRole('button', { name: /send/i }))

    expect(await screen.findByText('Hi there')).toBeInTheDocument()
    expect(await screen.findByText('Hello from AI!')).toBeInTheDocument()
  })

  it('shows error bubble when /prompt returns network error', async () => {
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/me')) return Promise.resolve({ ok: true, status: 200, json: async () => ({ username: 'alice' }) })
      return Promise.reject(new Error('Network error'))
    }))

    render(<App />)
    const textarea = await screen.findByPlaceholderText(/type your message/i)

    await userEvent.type(textarea, 'test')
    await userEvent.click(screen.getByRole('button', { name: /send/i }))

    expect(await screen.findByText(/could not reach the api/i)).toBeInTheDocument()
  })

  it('logs out when /prompt returns 401', async () => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: true, json: async () => ({ username: 'alice' }) },
      '/prompt': { ok: false, status: 401, json: async () => ({}) },
      '/logout': { ok: true, json: async () => ({}) },
    }))

    render(<App />)
    const textarea = await screen.findByPlaceholderText(/type your message/i)

    await userEvent.type(textarea, 'test')
    await userEvent.click(screen.getByRole('button', { name: /send/i }))

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  })

  it('send button is disabled when input is empty', async () => {
    render(<App />)
    await screen.findByPlaceholderText(/type your message/i)
    expect(screen.getByRole('button', { name: /send/i })).toBeDisabled()
  })

  it('sends message on Enter keypress', async () => {
    const promptFetch = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ reply: 'pong' }) })
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/me')) return Promise.resolve({ ok: true, status: 200, json: async () => ({ username: 'alice' }) })
      return promptFetch(url)
    }))

    render(<App />)
    const textarea = await screen.findByPlaceholderText(/type your message/i)

    await userEvent.type(textarea, 'ping{Enter}')
    expect(await screen.findByText('pong')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// ProfileMenu
// ---------------------------------------------------------------------------

describe('ProfileMenu', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch({
      '/me': { ok: true, json: async () => ({ username: 'alice' }) },
    }))
  })

  it('shows avatar with first letter of username', async () => {
    render(<App />)
    await screen.findByPlaceholderText(/type your message/i)
    expect(screen.getByText('A')).toBeInTheDocument()
  })

  it('opens dropdown and shows Logout option on click', async () => {
    render(<App />)
    await screen.findByPlaceholderText(/type your message/i)

    await userEvent.click(screen.getByText('alice'))
    expect(screen.getByRole('button', { name: /log out/i })).toBeInTheDocument()
  })

  it('logs out and returns to login page', async () => {
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/me')) return Promise.resolve({ ok: true, status: 200, json: async () => ({ username: 'alice' }) })
      if (url.includes('/logout')) return Promise.resolve({ ok: true, status: 200, json: async () => ({}) })
      return Promise.reject(new Error('Unhandled'))
    }))

    render(<App />)
    await screen.findByPlaceholderText(/type your message/i)

    await userEvent.click(screen.getByText('alice'))
    await userEvent.click(screen.getByRole('button', { name: /log out/i }))

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  })

  it('closes dropdown when clicking outside', async () => {
    render(<App />)
    await screen.findByPlaceholderText(/type your message/i)

    await userEvent.click(screen.getByText('alice'))
    expect(screen.getByRole('button', { name: /log out/i })).toBeInTheDocument()

    await userEvent.click(document.body)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /log out/i })).not.toBeInTheDocument()
    })
  })
})
