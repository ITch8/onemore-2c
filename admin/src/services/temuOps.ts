import { getJSON, postJSON } from '@/services/request';

export type TemuStatus = {
  mode: string;
  liveWrites: boolean;
  credentialsConfigured: boolean;
  region: string;
  routerHost: string;
  adsTransport: string;
  csSendTransport: string;
  listingCount: number;
  priceRuleCount: number;
  orderCount: number;
  adSuggestionCount: number;
  ticketCount: number;
  notes: string[];
};

export type TemuPage<T> = {
  list: T[];
  pagination: { page: number; pageSize: number; total: number };
};

export type TemuListing = {
  id: string;
  title: string;
  sourceUrl?: string;
  currency: string;
  status: string;
  mode: string;
  externalGoodsId?: string;
  errorMessage?: string;
  updatedAt: string;
};

export type TemuPriceQuote = {
  suggestedPriceMinor: number;
  floorMinor: number;
  reason: string;
};

export type TemuPriceRule = {
  id: string;
  skuCode: string;
  currency: string;
  suggestedPriceMinor: number;
  stock: number;
  status: string;
  mode: string;
  reason?: string;
  errorMessage?: string;
};

export type TemuOrder = {
  id: string;
  externalOrderId: string;
  orderNo: string;
  status: string;
  currency: string;
  totalAmountMinor: number;
  buyerMasked?: string;
  countryCode?: string;
  itemSummary?: string;
  fulfillmentStatus?: string;
  mode: string;
};

export type TemuFulfillment = {
  id: string;
  status: string;
  note?: string;
  labelRef?: string;
  trackingNo?: string;
  errorMessage?: string;
};

export type TemuAdSuggestion = {
  id: string;
  campaignName: string;
  currentBudgetMinor: number;
  suggestedBudgetMinor: number;
  reason: string;
  status: string;
  mode: string;
  note?: string;
  errorMessage?: string;
};

export type TemuTicket = {
  id: string;
  kind: string;
  subject: string;
  body?: string;
  buyerMasked?: string;
  suggestedReply?: string;
  confirmedReply?: string;
  status: string;
  mode: string;
  note?: string;
  errorMessage?: string;
};

export function createTemuIdempotencyKey(action: string) {
  const rand =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `temu-${action}-${rand}`.replace(/[^A-Za-z0-9_.:-]/g, '').slice(0, 128);
}

const pageQuery = (page = 1, pageSize = 20) =>
  `?page=${encodeURIComponent(page)}&pageSize=${encodeURIComponent(pageSize)}`;

export const getTemuStatus = () => getJSON<TemuStatus>('/api/v1/temu-ops/status');

export const listTemuListings = (page = 1, pageSize = 20) =>
  getJSON<TemuPage<TemuListing>>(`/api/v1/temu-ops/listings${pageQuery(page, pageSize)}`);

export const createTemuListing = (payload: {
  idempotencyKey: string;
  productId?: string;
  sourceUrl?: string;
  title: string;
  description?: string;
  currency?: string;
  categoryId?: string;
  skus: { skuCode: string; priceMinor: number; stock: number }[];
  submit: boolean;
  confirmLive: boolean;
}) => postJSON<{ listing: TemuListing }>('/api/v1/temu-ops/listings', payload);

export const previewTemuPrice = (payload: {
  skuCode: string;
  costMinor: number;
  marginBps: number;
  minPriceMinor: number;
  maxPriceMinor: number;
  competitorPriceMinor: number;
  followDeltaMinor: number;
  stock: number;
  currency?: string;
}) => postJSON<{ quote: TemuPriceQuote }>('/api/v1/temu-ops/price-stock/preview', payload);

export const applyTemuPrice = (
  payload: {
    idempotencyKey: string;
    confirmLive: boolean;
  } & Parameters<typeof previewTemuPrice>[0],
) => postJSON<{ rule: TemuPriceRule; quote: TemuPriceQuote }>('/api/v1/temu-ops/price-stock/apply', payload);

export const listTemuPriceRules = (page = 1, pageSize = 20) =>
  getJSON<TemuPage<TemuPriceRule>>(`/api/v1/temu-ops/price-stock/rules${pageQuery(page, pageSize)}`);

export const listTemuOrders = (page = 1, pageSize = 20) =>
  getJSON<TemuPage<TemuOrder>>(`/api/v1/temu-ops/orders${pageQuery(page, pageSize)}`);

export const pullTemuOrders = (idempotencyKey: string) =>
  postJSON<{ orders: TemuOrder[]; created: number; updated: number }>('/api/v1/temu-ops/orders/pull', {
    idempotencyKey,
    pageSize: 20,
  });

export const prepareTemuLabel = (id: string, idempotencyKey: string) =>
  postJSON<{ fulfillment: TemuFulfillment }>(`/api/v1/temu-ops/orders/${encodeURIComponent(id)}/label`, {
    idempotencyKey,
  });

export const confirmTemuShipment = (
  id: string,
  payload: { idempotencyKey: string; carrier: string; trackingNo: string; confirmLive: boolean },
) =>
  postJSON<{ fulfillment: TemuFulfillment }>(
    `/api/v1/temu-ops/orders/${encodeURIComponent(id)}/ship`,
    payload,
  );

export const listTemuAds = (page = 1, pageSize = 20) =>
  getJSON<TemuPage<TemuAdSuggestion>>(`/api/v1/temu-ops/ads/suggestions${pageQuery(page, pageSize)}`);

export const optimizeTemuAd = (payload: {
  idempotencyKey: string;
  campaignName: string;
  currentBudgetMinor: number;
  spendMinor: number;
  revenueMinor: number;
  targetRoasBps: number;
}) => postJSON<{ suggestion: TemuAdSuggestion }>('/api/v1/temu-ops/ads/optimize', payload);

export const confirmTemuAd = (id: string, idempotencyKey: string) =>
  postJSON<{ suggestion: TemuAdSuggestion }>(
    `/api/v1/temu-ops/ads/suggestions/${encodeURIComponent(id)}/confirm`,
    { idempotencyKey },
  );

export const applyTemuAd = (id: string, idempotencyKey: string, confirmLive: boolean) =>
  postJSON<{ suggestion: TemuAdSuggestion }>(
    `/api/v1/temu-ops/ads/suggestions/${encodeURIComponent(id)}/apply`,
    { idempotencyKey, confirmLive },
  );

export const listTemuTickets = (page = 1, pageSize = 20) =>
  getJSON<TemuPage<TemuTicket>>(`/api/v1/temu-ops/cs/tickets${pageQuery(page, pageSize)}`);

export const pullTemuTickets = (idempotencyKey: string) =>
  postJSON<{ tickets: TemuTicket[] }>('/api/v1/temu-ops/cs/tickets/pull', { idempotencyKey });

export const suggestTemuReply = (id: string, idempotencyKey: string) =>
  postJSON<{ ticket: TemuTicket }>(`/api/v1/temu-ops/cs/tickets/${encodeURIComponent(id)}/suggest`, {
    idempotencyKey,
  });

export const confirmTemuReply = (
  id: string,
  payload: { idempotencyKey: string; reply: string; confirmLive: boolean },
) =>
  postJSON<{ ticket: TemuTicket }>(
    `/api/v1/temu-ops/cs/tickets/${encodeURIComponent(id)}/confirm`,
    payload,
  );
