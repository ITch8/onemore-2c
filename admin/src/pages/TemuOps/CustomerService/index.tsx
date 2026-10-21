import type { ProColumns } from '@ant-design/pro-components';
import { Alert, Button, Form, Input, Modal, Popconfirm, Space, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { EmptyState, ErrorAlert, TmPageContainer, TmProTable } from '@/components/ui';
import { usePermission } from '@/hooks/usePermission';
import {
  confirmTemuReply,
  createTemuIdempotencyKey,
  listTemuTickets,
  pullTemuTickets,
  suggestTemuReply,
  type TemuTicket,
} from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { temuErrorText, temuStatusLabel } from '../labels';
import '../temu-ops.less';

type TicketRow = TemuTicket & Record<string, unknown>;

const kindLabel = (kind?: string) => (kind === 'after_sales' ? '售后' : kind === 'message' ? '咨询' : '其他');

export default function TemuCustomerServicePage() {
  const { can, readonly } = usePermission();
  const canOperate = !readonly && can(PERMISSIONS.TEMU_OPERATE);
  const [form] = Form.useForm<{ reply: string }>();
  const [rows, setRows] = useState<TicketRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [current, setCurrent] = useState<TicketRow>();

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listTemuTickets();
      setRows((result.list ?? []) as TicketRow[]);
    } catch (nextError) {
      setError(temuErrorText(nextError, '客服记录加载失败，请稍后重试。'));
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
      await pullTemuTickets(createTemuIdempotencyKey('cs'));
      message.success('已拉取客服与售后记录');
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '拉取失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const suggest = async (row: TicketRow) => {
    setSubmitting(true);
    try {
      const result = await suggestTemuReply(String(row.id), createTemuIdempotencyKey('cs-suggest'));
      message.success('已生成建议回复，尚未发送');
      setCurrent(result.ticket as TicketRow);
      form.setFieldsValue({ reply: result.ticket.suggestedReply });
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '建议回复生成失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const confirm = async () => {
    if (!current) return;
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const result = await confirmTemuReply(String(current.id), {
        idempotencyKey: createTemuIdempotencyKey('cs-confirm'),
        reply: values.reply.trim(),
        confirmLive: false,
      });
      message.success(result.ticket.note || temuStatusLabel(result.ticket.status));
      setCurrent(undefined);
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '回复保存失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ProColumns<TicketRow>[] = [
    { title: '类型', dataIndex: 'kind', width: 80, render: (_, row) => kindLabel(String(row.kind ?? '')) },
    { title: '主题', dataIndex: 'subject', ellipsis: true },
    { title: '买家', dataIndex: 'buyerMasked', width: 120, render: (_, row) => row.buyerMasked || '—' },
    { title: '状态', dataIndex: 'status', width: 140, render: (_, row) => temuStatusLabel(String(row.status ?? '')) },
    { title: '说明', dataIndex: 'note', ellipsis: true, render: (_, row) => row.note || '—' },
    {
      title: '操作',
      valueType: 'option',
      width: 180,
      render: (_, row) =>
        canOperate ? (
          <Space wrap>
            <Button size="small" disabled={submitting} onClick={() => void suggest(row)}>生成回复</Button>
            <Button
              size="small"
              disabled={submitting}
              onClick={() => {
                setCurrent(row);
                form.setFieldsValue({ reply: String(row.confirmedReply || row.suggestedReply || '') });
              }}
            >
              确认回复
            </Button>
          </Space>
        ) : '—',
    },
  ];

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 客服与售后" subTitle="建议回复必须人工确认。默认只保存在本地，不会自动发送给买家。">
        <Alert type="info" showIcon message="确认回复不会自动外发。" style={{ marginBottom: 16 }} />
        <div className="temu-ops-toolbar">
          <Button onClick={() => void load()}>刷新</Button>
          {canOperate ? (
            <Popconfirm title="拉取客服与售后记录？" okText="拉取" cancelText="取消" onConfirm={() => void pull()}>
              <Button type="primary" loading={submitting}>拉取记录</Button>
            </Popconfirm>
          ) : null}
        </div>
        {error ? <ErrorAlert title="客服记录加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {!loading && !error && rows.length === 0 ? (
          <EmptyState title="还没有客服或售后记录" description="拉取后可以生成建议回复，并在确认后保存。" actionLabel={canOperate ? '拉取记录' : undefined} onAction={canOperate ? () => void pull() : undefined} />
        ) : (
          <TmProTable<TicketRow> rowKey="id" search={false} options={false} loading={loading} columns={columns} dataSource={rows} pagination={false} scroll={{ x: 960 }} />
        )}
        <Modal title="确认回复" open={Boolean(current)} confirmLoading={submitting} okText="保存回复" cancelText="取消" onCancel={() => setCurrent(undefined)} onOk={() => void confirm()} destroyOnHidden width="min(560px, 92vw)">
          <Form form={form} layout="vertical">
            <Form.Item name="reply" label="回复内容" rules={[{ required: true, message: '请填写回复' }]}>
              <Input.TextArea rows={5} maxLength={4000} />
            </Form.Item>
          </Form>
        </Modal>
      </TmPageContainer>
    </PermissionGuard>
  );
}
