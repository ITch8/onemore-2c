import { expect, test } from '../fixtures/admin.fixture';
import { ok } from '../mocks/envelope';
import { expectNoRootOverflow } from '../utils/assertions';

const viewports = [
  { width: 1440, height: 900 },
  { width: 1280, height: 800 },
  { width: 1024, height: 768 },
  { width: 768, height: 900 },
  { width: 375, height: 812 },
];

const status = {
  mode: 'mock',
  liveWrites: false,
  credentialsConfigured: false,
  region: 'us',
  routerHost: 'openapi-b-us.temu.com',
  adsTransport: 'api_gap',
  csSendTransport: 'local_only',
  listingCount: 0,
  priceRuleCount: 0,
  orderCount: 0,
  adSuggestionCount: 0,
  ticketCount: 0,
  notes: ['默认只保存本地演示记录，不访问 Temu。'],
};

const listing = {
  id: 'temu-listing-1',
  title: '演示水杯',
  sourceUrl: 'https://detail.1688.com/offer/1.html',
  currency: 'USD',
  status: 'demo_saved',
  mode: 'mock',
  externalGoodsId: 'demo-cup',
  updatedAt: '2026-10-21T00:00:00Z',
};

test.describe('Temu seller operations', () => {
  test('overview stays within five viewports', async ({ admin, page }) => {
    await page.route('**/api/v1/temu-ops/status', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ok(status)),
      });
    });
    for (const viewport of viewports) {
      await page.setViewportSize(viewport);
      await admin.goto('/temu-ops/overview');
      await expect(page.getByText('Temu 运营总览', { exact: true })).toBeVisible();
      await expect(page.getByText('商品上架').first()).toBeVisible();
      await expect(page.getByText('客服与售后').first()).toBeVisible();
      await expectNoRootOverflow(page);
    }
  });

  test('creates a listing record only after confirmation', async ({ admin, page }) => {
    await page.route('**/api/v1/temu-ops/listings**', async (route) => {
      if (route.request().method() !== 'GET') {
        await route.fallback();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ok({ list: [], pagination: { page: 1, pageSize: 20, total: 0 } })),
      });
    });
    admin.writeGuard.allow({
      operation: 'create-temu-listing',
      method: 'POST',
      path: /^\/api\/v1\/temu-ops\/listings$/,
      response: ok({ listing }),
    });

    await admin.goto('/temu-ops/listings');
    await expect(page.getByText('还没有 Temu 上架记录')).toBeVisible();
    await page.getByRole('button', { name: '新建上架记录' }).first().click();
    await page.getByRole('button', { name: '取 消' }).click();
    await admin.writeGuard.expectRequestCount('create-temu-listing', 0);

    await page.getByRole('button', { name: '新建上架记录' }).first().click();
    await page.getByLabel('标题').fill('演示水杯');
    await page.getByLabel('规格编码').fill('CUP');
    await page.locator('.ant-modal-content').getByRole('button', { name: '保 存' }).click();
    await admin.writeGuard.expectRequestCount('create-temu-listing', 1);
    const payload = admin.writeGuard.calls('create-temu-listing')[0]?.postDataJSON as {
      title?: string;
      confirmLive?: boolean;
      skus?: { skuCode?: string }[];
    };
    expect(payload.title).toBe('演示水杯');
    expect(payload.confirmLive).toBe(false);
    expect(payload.skus?.[0]?.skuCode).toBe('CUP');
  });
});
