import type { ProColumns } from '@ant-design/pro-components';
import { Button, Form, Input, Modal, Popconfirm, Space, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { EmptyState, ErrorAlert, TmPageContainer, TmProTable } from '@/components/ui';
import { usePermission } from '@/hooks/usePermission';
import {
  confirmTemuShipment,
  createTemuIdempotencyKey,
  listTemuOrders,
  prepareTemuLabel,
  pullTemuOrders,
  type TemuOrder,
} from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { minorMoney, temuErrorText, temuModeLabel, temuStatusLabel } from '../labels';
import '../temu-ops.less';

type OrderRow = TemuOrder & Record<string, unknown>;

export default function TemuOrdersPage() {
  const { can, readonly } = usePermission();
  const canOperate = !readonly && can(PERMISSIONS.TEMU_OPERATE);
  const [form] = Form.useForm<{ carrier: string; trackingNo: string }>();
  const [rows, setRows] = useState<OrderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [shipOrder, setShipOrder] = useState<OrderRow>();

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listTemuOrders();
      setRows((result.list ?? []) as OrderRow[]);
    } catch (nextError) {
      setError(temuErrorText(nextError, '订单加载失败，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const pull = async () => {
    setSubmitting(true);
    try {
      const result = await pullTemuOrders(createTemuIdempotencyKey('orders'));
      message.success(`已拉取 ${result.orders?.length ?? 0} 笔`);
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '订单拉取失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const label = async (row: OrderRow) => {
    setSubmitting(true);
    try {
      const result = await prepareTemuLabel(String(row.id), createTemuIdempotencyKey('label'));
      message.success(result.fulfillment.note || temuStatusLabel(result.fulfillment.status));
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '面单准备失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const ship = async () => {
    if (!shipOrder) return;
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const result = await confirmTemuShipment(String(shipOrder.id), {
        idempotencyKey: createTemuIdempotencyKey('ship'),
        carrier: values.carrier.trim(),
        trackingNo: values.trackingNo.trim(),
        confirmLive: false,
      });
      message.success(result.fulfillment.note || temuStatusLabel(result.fulfillment.status));
      setShipOrder(undefined);
      form.resetFields();
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '发货回传失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ProColumns<OrderRow>[] = [
    { title: '订单号', dataIndex: 'orderNo', ellipsis: true },
    { title: '金额', dataIndex: 'totalAmountMinor', width: 120, render: (_, row) => minorMoney(Number(row.totalAmountMinor), String(row.currency ?? '')) },
    { title: '买家', dataIndex: 'buyerMasked', width: 120, render: (_, row) => row.buyerMasked || '—' },
    { title: '履约', dataIndex: 'fulfillmentStatus', width: 140, render: (_, row) => temuStatusLabel(String(row.fulfillmentStatus || row.status || '')) },
    { title: '运行方式', dataIndex: 'mode', width: 120, render: (_, row) => temuModeLabel(String(row.mode ?? '')) },
    {
      title: '操作',
      valueType: 'option',
      width: 180,
      render: (_, row) =>
        canOperate ? (
          <Space wrap>
            <Button size="small" disabled={submitting} onClick={() => void label(row)}>准备面单</Button>
            <Button size="small" disabled={submitting} onClick={() => setShipOrder(row)}>回传发货</Button>
          </Space>
        ) : '—',
    },
  ];

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 订单履约" subTitle="订单保存在 Temu 运营记录中，不进入现有订单、库存或仓库账。面单默认只生成演示引用。">
        <div className="temu-ops-toolbar">
          <Button onClick={() => void load()}>刷新</Button>
          {canOperate ? (
            <Popconfirm title="拉取订单到本地记录？" okText="拉取" cancelText="取消" onConfirm={() => void pull()}>
              <Button type="primary" loading={submitting}>拉取订单</Button>
            </Popconfirm>
          ) : null}
        </div>
        {error ? <ErrorAlert title="订单加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {!loading && !error && rows.length === 0 ? (
          <EmptyState title="还没有 Temu 订单" description="拉取后可以准备面单并回传发货。演示模式不会通知 Temu。" actionLabel={canOperate ? '拉取订单' : undefined} onAction={canOperate ? () => void pull() : undefined} />
        ) : (
          <TmProTable<OrderRow> rowKey="id" search={false} options={false} loading={loading} columns={columns} dataSource={rows} pagination={false} scroll={{ x: 960 }} />
        )}
        <Modal title="回传发货" open={Boolean(shipOrder)} confirmLoading={submitting} okText="保存" cancelText="取消" onCancel={() => setShipOrder(undefined)} onOk={() => void ship()} destroyOnHidden width="min(480px, 92vw)">
          <Form form={form} layout="vertical">
            <Form.Item name="carrier" label="承运商" rules={[{ required: true, message: '请填写承运商' }]}>
              <Input />
            </Form.Item>
            <Form.Item name="trackingNo" label="运单号" rules={[{ required: true, message: '请填写运单号' }]}>
              <Input />
            </Form.Item>
          </Form>
        </Modal>
      </TmPageContainer>
    </PermissionGuard>
  );
}
