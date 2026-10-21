import { describe, expect, it } from 'vitest';
import contract from './temu-ops.json';

describe('Temu ops API contract', () => {
  it('keeps the shared envelope and the five module routes', () => {
    expect(contract.envelope).toEqual(['code', 'message', 'data']);
    const paths = contract.endpoints.map((item) => item.path);
    expect(paths).toEqual(
      expect.arrayContaining([
        '/api/v1/temu-ops/listings',
        '/api/v1/temu-ops/price-stock/apply',
        '/api/v1/temu-ops/orders/pull',
        '/api/v1/temu-ops/ads/optimize',
        '/api/v1/temu-ops/cs/tickets',
      ]),
    );
    for (const endpoint of contract.endpoints) {
      expect(endpoint.method).toMatch(/^(GET|POST)$/);
      expect(endpoint.permission === 'temu.view' || endpoint.permission === 'temu.operate').toBe(true);
      if (endpoint.method === 'GET') {
        expect(endpoint.permission).toBe('temu.view');
      }
    }
  });
});
