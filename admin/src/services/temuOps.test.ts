import { describe, expect, it } from 'vitest';
import { createTemuIdempotencyKey } from './temuOps';

describe('temu ops service helpers', () => {
  it('creates distinct bounded idempotency keys', () => {
    const first = createTemuIdempotencyKey('listing');
    const second = createTemuIdempotencyKey('listing');
    expect(first).toMatch(/^temu-listing-/);
    expect(first.length).toBeLessThanOrEqual(128);
    expect(first).not.toBe(second);
  });
});
