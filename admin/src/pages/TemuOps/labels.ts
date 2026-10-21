import { commonStatusLabel } from '@/constants/copywriting';

export function temuStatusLabel(status?: string | null) {
  const raw = (status ?? '').trim();
  if (!raw) return '—';
  const label = commonStatusLabel(raw);
  if (label === raw) return '未知状态';
  return label;
}

export function temuModeLabel(mode?: string | null) {
  if ((mode ?? '').trim().toLowerCase() === 'live') return '正式接口';
  return '演示模式';
}

export function temuTransportLabel(value?: string | null) {
  switch ((value ?? '').trim()) {
    case 'official_api':
      return '官方接口';
    case 'local_only':
      return '仅本地保存';
    case 'api_gap':
      return '官方接口未覆盖';
    default:
      return '未知';
  }
}

export function minorMoney(minor?: number, currency?: string) {
  const amount = ((minor ?? 0) / 100).toFixed(2);
  return `${currency || 'USD'} ${amount}`;
}

export function temuErrorText(error: unknown, fallback: string) {
  const value = (error as Error)?.message?.trim();
  return value && /[\u3400-\u9fff]/.test(value) ? value : fallback;
}
