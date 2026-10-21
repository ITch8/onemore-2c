import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, Form, Input, InputNumber, Popconfirm, Space, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { EmptyState, ErrorAlert, SectionCard, TmPageContainer, TmProTable } from '@/components/ui';
import { usePermission } from '@/hooks/usePermission';
import {
  applyTemuAd,
  confirmTemuAd,
  createTemuIdempotencyKey,
  listTemuAds,
  optimizeTemuAd,
  type TemuAdSuggestion,
} from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { minorMoney, temuErrorText, temuStatusLabel } from '../labels';
import '../temu-ops.less';

type AdRow = TemuAdSuggestion & Record<string, unknown>;
type AdForm = {
  campaignName: string;
  currentBudgetMinor: number;
  spendMinor: number;
  revenueMinor: number;
  targetRoasBps: number;
};

export default function TemuAdsPage() {
  const { can, readonly } = usePermission();
  const canOperate = !readonly && can(PERMISSIONS.TEMU_OPERATE);
  const [form] = Form.useForm<AdForm>();
  const [rows, setRows] = useState<AdRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listTemuAds();
      setRows((result.list ?? []) as AdRow[]);
    } catch (nextError) {
      setError(temuErrorText(nextError, '广告建议加载失败，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const optimize = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const result = await optimizeTemuAd({ ...values, idempotencyKey: createTemuIdempotencyKey('ads') });
      message.success(result.suggestion.reason);
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '广告建议生成失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const confirm = async (row: AdRow) => {
    setSubmitting(true);
    try {
      await confirmTemuAd(String(row.id), createTemuIdempotencyKey('ads-confirm'));
      message.success('已确认，预算尚未改动');
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '确认失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const apply = async (row: AdRow) => {
    setSubmitting(true);
    try {
      const result = await applyTemuAd(String(row.id), createTemuIdempotencyKey('ads-apply'), false);
      message.success(result.suggestion.note || temuStatusLabel(result.suggestion.status));
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '执行记录失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ProColumns<AdRow>[] = [
    { title: '计划', dataIndex: 'campaignName', ellipsis: true },
    { title: '当前预算', dataIndex: 'currentBudgetMinor', render: (_, row) => minorMoney(Number(row.currentBudgetMinor)) },
    { title: '建议预算', dataIndex: 'suggestedBudgetMinor', render: (_, row) => minorMoney(Number(row.suggestedBudgetMinor)) },
    { title: '状态', dataIndex: 'status', width: 140, render: (_, row) => temuStatusLabel(String(row.status ?? '')) },
    { title: '说明', dataIndex: 'note', ellipsis: true, render: (_, row) => row.note || row.reason },
    {
      title: '操作',
      valueType: 'option',
      width: 180,
      render: (_, row) =>
        canOperate ? (
          <Space wrap>
            <Button size="small" disabled={submitting || row.status !== 'suggested'} onClick={() => void confirm(row)}>确认</Button>
            <Popconfirm title="记录执行？默认不会改动 Temu 广告。" okText="记录" cancelText="取消" onConfirm={() => void apply(row)}>
              <Button size="small" disabled={submitting || row.status !== 'confirmed'}>记录执行</Button>
            </Popconfirm>
          </Space>
        ) : '—',
    },
  ];

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 广告优化" subTitle="根据花费和成交额给出预算建议。官方广告接口未配置时，执行只会留下未覆盖记录。">
        <Alert type="info" showIcon message="不会打开浏览器自动修改广告。" style={{ marginBottom: 16 }} />
        <SectionCard title="生成建议">
          <Form form={form} layout="vertical" initialValues={{ currentBudgetMinor: 10000, spendMinor: 8000, revenueMinor: 9000, targetRoasBps: 20000 }}>
            <Space wrap>
              <Form.Item name="campaignName" label="计划名称" rules={[{ required: true, message: '请填写计划名称' }]}>
                <Input />
              </Form.Item>
              <Form.Item name="currentBudgetMinor" label="当前预算（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="spendMinor" label="花费（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="revenueMinor" label="成交额（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="targetRoasBps" label="目标回报（万分比）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
            </Space>
            {canOperate ? <Button type="primary" loading={submitting} onClick={() => void optimize()}>生成建议</Button> : null}
          </Form>
        </SectionCard>
        {error ? <ErrorAlert title="广告建议加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {!loading && !error && rows.length === 0 ? (
          <EmptyState title="还没有广告建议" description="填写计划花费和成交额后生成建议。确认前不会记录执行。" />
        ) : (
          <TmProTable<AdRow> rowKey="id" search={false} options={false} loading={loading} columns={columns} dataSource={rows} pagination={false} scroll={{ x: 960 }} />
        )}
      </TmPageContainer>
    </PermissionGuard>
  );
}
