import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, Form, Input, InputNumber, Popconfirm, Space, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { EmptyState, ErrorAlert, SectionCard, TmPageContainer, TmProTable } from '@/components/ui';
import { usePermission } from '@/hooks/usePermission';
import {
  applyTemuPrice,
  createTemuIdempotencyKey,
  listTemuPriceRules,
  previewTemuPrice,
  type TemuPriceQuote,
  type TemuPriceRule,
} from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { minorMoney, temuErrorText, temuModeLabel, temuStatusLabel } from '../labels';
import '../temu-ops.less';

type RuleRow = TemuPriceRule & Record<string, unknown>;
type PriceForm = {
  skuCode: string;
  currency?: string;
  costMinor: number;
  marginBps: number;
  minPriceMinor: number;
  maxPriceMinor: number;
  competitorPriceMinor: number;
  followDeltaMinor: number;
  stock: number;
};

export default function TemuPriceStockPage() {
  const { can, readonly } = usePermission();
  const canOperate = !readonly && can(PERMISSIONS.TEMU_OPERATE);
  const [form] = Form.useForm<PriceForm>();
  const [rows, setRows] = useState<RuleRow[]>([]);
  const [quote, setQuote] = useState<TemuPriceQuote>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listTemuPriceRules();
      setRows((result.list ?? []) as RuleRow[]);
    } catch (nextError) {
      setError(temuErrorText(nextError, '价格记录加载失败，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const valuesOrNull = async () => form.validateFields();

  const preview = async () => {
    const values = await valuesOrNull();
    setSubmitting(true);
    try {
      const result = await previewTemuPrice(values);
      setQuote(result.quote);
    } catch (nextError) {
      message.error(temuErrorText(nextError, '价格试算失败，请检查输入。'));
    } finally {
      setSubmitting(false);
    }
  };

  const apply = async () => {
    const values = await valuesOrNull();
    setSubmitting(true);
    try {
      const result = await applyTemuPrice({
        ...values,
        idempotencyKey: createTemuIdempotencyKey('price'),
        confirmLive: false,
      });
      message.success(`已记录：${temuStatusLabel(result.rule.status)}`);
      setQuote(result.quote);
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '价格记录保存失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ProColumns<RuleRow>[] = [
    { title: '规格编码', dataIndex: 'skuCode' },
    { title: '建议价', dataIndex: 'suggestedPriceMinor', render: (_, row) => minorMoney(Number(row.suggestedPriceMinor), String(row.currency ?? '')) },
    { title: '库存', dataIndex: 'stock', width: 80 },
    { title: '状态', dataIndex: 'status', width: 140, render: (_, row) => temuStatusLabel(String(row.status ?? '')) },
    { title: '运行方式', dataIndex: 'mode', width: 120, render: (_, row) => temuModeLabel(String(row.mode ?? '')) },
    { title: '说明', dataIndex: 'reason', ellipsis: true },
  ];

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 价格与库存" subTitle="先试算跟价，再明确保存。金额使用货币最小单位，例如 1999 表示 19.99。">
        <SectionCard title="跟价试算">
          <Form
            form={form}
            layout="vertical"
            initialValues={{ currency: 'USD', costMinor: 1000, marginBps: 1500, minPriceMinor: 1200, maxPriceMinor: 5000, competitorPriceMinor: 1800, followDeltaMinor: 50, stock: 10 }}
          >
            <Space wrap>
              <Form.Item name="skuCode" label="规格编码" rules={[{ required: true, message: '请填写规格编码' }]}>
                <Input />
              </Form.Item>
              <Form.Item name="currency" label="币种">
                <Input maxLength={3} />
              </Form.Item>
              <Form.Item name="costMinor" label="成本（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="marginBps" label="毛利（万分比）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="minPriceMinor" label="最低价（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="maxPriceMinor" label="最高价（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="competitorPriceMinor" label="竞品价（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="followDeltaMinor" label="跟价差额（分）" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="stock" label="库存" rules={[{ required: true }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
            </Space>
            <Space wrap>
              <Button onClick={() => void preview()} loading={submitting}>试算</Button>
              {canOperate ? (
                <Popconfirm title="保存这条价格与库存记录？" okText="保存" cancelText="取消" onConfirm={() => void apply()}>
                  <Button type="primary" loading={submitting}>保存记录</Button>
                </Popconfirm>
              ) : null}
            </Space>
          </Form>
          {quote ? (
            <Alert
              style={{ marginTop: 16 }}
              type="info"
              showIcon
              message={`建议价 ${minorMoney(quote.suggestedPriceMinor)}，底价 ${minorMoney(quote.floorMinor)}`}
              description={quote.reason}
            />
          ) : null}
        </SectionCard>
        {error ? <ErrorAlert title="价格记录加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {!loading && !error && rows.length === 0 ? (
          <EmptyState title="还没有价格与库存记录" description="先试算，再保存。保存不会自动改动 Temu 在售价格。" />
        ) : (
          <TmProTable<RuleRow> rowKey="id" search={false} options={false} loading={loading} columns={columns} dataSource={rows} pagination={false} scroll={{ x: 880 }} />
        )}
      </TmPageContainer>
    </PermissionGuard>
  );
}
