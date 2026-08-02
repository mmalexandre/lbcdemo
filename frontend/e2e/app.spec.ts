import { test, expect, Page } from '@playwright/test'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Mock all API routes so tests run without a real backend.
 * Call this at the start of each test that needs specific behaviour.
 */
async function mockNotAuthenticated(page: Page) {
  await page.route('**/me', route =>
    route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({}) })
  )
}

async function mockAuthenticated(page: Page, username = 'alice') {
  await page.route('**/me', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ username }) })
  )
}

async function mockLogin(page: Page, username = 'alice') {
  await page.route('**/login', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ username }) })
  )
}

async function mockLoginFail(page: Page) {
  await page.route('**/login', route =>
    route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({}) })
  )
}

async function mockPrompt(page: Page, reply = 'Hello from AI!') {
  await page.route('**/prompt', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ reply }) })
  )
}

async function mockLogout(page: Page) {
  await page.route('**/logout', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) })
  )
}

// ---------------------------------------------------------------------------
// Login page
// ---------------------------------------------------------------------------

test.describe('Login page', () => {
  test('shows sign-in form when not authenticated', async ({ page }) => {
    await mockNotAuthenticated(page)
    await page.goto('/')
    await expect(page.getByRole('heading', { name: /sign in/i })).toBeVisible()
    await expect(page.getByLabel(/username/i)).toBeVisible()
    await expect(page.getByLabel(/password/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /login/i })).toBeVisible()
  })

  test('shows error on wrong credentials', async ({ page }) => {
    await mockNotAuthenticated(page)
    await mockLoginFail(page)
    await page.goto('/')

    await page.getByLabel(/username/i).fill('wrong')
    await page.getByLabel(/password/i).fill('wrong')
    await page.getByRole('button', { name: /login/i }).click()

    await expect(page.getByText(/invalid username or password/i)).toBeVisible()
  })

  test('navigates to chat on successful login', async ({ page }) => {
    await mockNotAuthenticated(page)
    await mockLogin(page, 'alice')
    await page.goto('/')

    await page.getByLabel(/username/i).fill('alice')
    await page.getByLabel(/password/i).fill('secret')
    await page.getByRole('button', { name: /login/i }).click()

    await expect(page.getByPlaceholder(/type your message/i)).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// Chat page
// ---------------------------------------------------------------------------

test.describe('Chat page', () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticated(page, 'alice')
    await mockPrompt(page)
    await mockLogout(page)
    await page.goto('/')
  })

  test('shows chat interface when authenticated', async ({ page }) => {
    await expect(page.getByPlaceholder(/type your message/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /send/i })).toBeVisible()
  })

  test('send button is disabled when input is empty', async ({ page }) => {
    await expect(page.getByRole('button', { name: /send/i })).toBeDisabled()
  })

  test('sends a message and shows AI reply', async ({ page }) => {
    await page.getByPlaceholder(/type your message/i).fill('Hello!')
    await page.getByRole('button', { name: /send/i }).click()

    await expect(page.getByText('Hello!')).toBeVisible()
    await expect(page.getByText('Hello from AI!')).toBeVisible()
  })

  test('clears input after sending', async ({ page }) => {
    const textarea = page.getByPlaceholder(/type your message/i)
    await textarea.fill('test message')
    await page.getByRole('button', { name: /send/i }).click()

    await expect(textarea).toHaveValue('')
  })

  test('Enter key sends the message', async ({ page }) => {
    await page.getByPlaceholder(/type your message/i).fill('ping')
    await page.keyboard.press('Enter')
    await expect(page.getByText('Hello from AI!')).toBeVisible()
  })

  test('Shift+Enter adds newline instead of sending', async ({ page }) => {
    const textarea = page.getByPlaceholder(/type your message/i)
    await textarea.fill('line1')
    await page.keyboard.press('Shift+Enter')
    const value = await textarea.inputValue()
    expect(value).toContain('\n')
  })
})

// ---------------------------------------------------------------------------
// Profile menu
// ---------------------------------------------------------------------------

test.describe('Profile menu', () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthenticated(page, 'alice')
    await mockLogout(page)
    await page.goto('/')
  })

  test('displays username and avatar initial', async ({ page }) => {
    await expect(page.getByText('alice')).toBeVisible()
    await expect(page.getByText('A')).toBeVisible()
  })

  test('opens dropdown on click', async ({ page }) => {
    await page.getByText('alice').click()
    await expect(page.getByRole('button', { name: /log out/i })).toBeVisible()
  })

  test('closes dropdown when clicking outside', async ({ page }) => {
    await page.getByText('alice').click()
    await expect(page.getByRole('button', { name: /log out/i })).toBeVisible()

    await page.click('body', { position: { x: 10, y: 10 } })
    await expect(page.getByRole('button', { name: /log out/i })).not.toBeVisible()
  })

  test('logs out and returns to login page', async ({ page }) => {
    await page.getByText('alice').click()
    await page.getByRole('button', { name: /log out/i }).click()

    // After logout, /me should return 401
    await page.route('**/me', route =>
      route.fulfill({ status: 401, body: JSON.stringify({}) })
    )

    await expect(page.getByRole('heading', { name: /sign in/i })).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

test.describe('Error handling', () => {
  test('shows error bubble when prompt API is unreachable', async ({ page }) => {
    await mockAuthenticated(page, 'alice')
    await page.route('**/prompt', route => route.abort())
    await page.goto('/')

    await page.getByPlaceholder(/type your message/i).fill('test')
    await page.getByRole('button', { name: /send/i }).click()

    await expect(page.getByText(/could not reach the api/i)).toBeVisible()
  })

  test('redirects to login when prompt returns 401', async ({ page }) => {
    await mockAuthenticated(page, 'alice')
    await page.route('**/prompt', route =>
      route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({}) })
    )
    await page.goto('/')

    await page.getByPlaceholder(/type your message/i).fill('test')
    await page.getByRole('button', { name: /send/i }).click()

    await expect(page.getByRole('heading', { name: /sign in/i })).toBeVisible()
  })
})
