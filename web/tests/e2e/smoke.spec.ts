import { expect, test } from '@playwright/test'

test('renders login screen', async ({ page }) => {
  await page.goto('/console')
  await expect(page.getByRole('heading', { name: 'Web Operator Console' })).toBeVisible()
})
