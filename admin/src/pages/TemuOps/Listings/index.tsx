import type { ProColumns } from '@ant-design/pro-components';
import { Button, Form, Input, InputNumber, Modal, Space, Switch, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { EmptyState, ErrorAlert, TmPageContainer, TmProTable } from '@/components/ui';
import { usePermission } from '@/hooks/usePermission';
import { createTemuIdempotencyKey, createTemuListing, listTemuListings, type TemuListing } from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { minorMoney, temuErrorText, temuModeLabel, temuStatusLabel } from '../labels';
import '../temu-ops.less';

type ListingRow = TemuListing & Record<string, unknown>;
type ListingForm = {
  sourceUrl?: string;
  productId?: string;
  title: string;
  description?: string;
  currency?: string;
  categoryId?: string;
  skuCode: string;
  priceMinor: number;
  stock: number;
  submit: boolean;
};

export default function TemuListingsPage() {
  const { can, readonly } = usePermission();
  const canOperate = !readonly && can(PERMISSIONS.TEMU_OPERATE);
  const [form] = Form.useForm<ListingForm>();
  const [rows, setRows] = useState<ListingRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listTemuListings();
      setRows((result.list ?? []) as ListingRow[]);
    } catch (nextError) {
      setError(temuErrorText(nextError, '上架记录加载失败，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const columns: ProColumns<ListingRow>[] = [
    { title: '标题', dataIndex: 'title', ellipsis: true },
    { title: '状态', dataIndex: 'status', width: 140, render: (_, row) => temuStatusLabel(String(row.status ?? '')) },
    { title: '运行方式', dataIndex: 'mode', width: 120, render: (_, row) => temuModeLabel(String(row.mode ?? '')) },
    { title: '外部编号', dataIndex: 'externalGoodsId', ellipsis: true, render: (_, row) => row.externalGoodsId || '—' },
    { title: '失败原因', dataIndex: 'errorMessage', ellipsis: true, render: (_, row) => row.errorMessage || '—' },
  ];

  const submit = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const result = await createTemuListing({
        idempotencyKey: createTemuIdempotencyKey('listing'),
        productId: values.productId?.trim() || undefined,
        sourceUrl: values.sourceUrl?.trim() || undefined,
        title: values.title.trim(),
        description: values.description?.trim(),
        currency: values.currency?.trim() || 'USD',
        categoryId: values.categoryId?.trim(),
        skus: [{ skuCode: values.skuCode.trim(), priceMinor: values.priceMinor, stock: values.stock }],
        submit: values.submit,
        confirmLive: false,
      });
      message.success(values.submit ? `已记录：${temuStatusLabel(result.listing.status)}` : '草稿已保存');
      setOpen(false);
      form.resetFields();
      await load();
    } catch (nextError) {
      message.error(temuErrorText(nextError, '上架记录保存失败，请稍后重试。'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 商品上架" subTitle="来源可以是已有商品草稿编号，或手工填写的货源链接。提交默认不调用 Temu。">
        <div className="temu-ops-toolbar">
          <Button onClick={() => void load()}>刷新</Button>
          {canOperate ? (
            <Button
              type="primary"
              onClick={() => {
                form.setFieldsValue({ currency: 'USD', priceMinor: 1999, stock: 1, submit: false });
                setOpen(true);
              }}
            >
              新建上架记录
            </Button>
          ) : null}
        </div>
        {error ? <ErrorAlert title="上架记录加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {!loading && !error && rows.length === 0 ? (
          <EmptyState title="还没有 Temu 上架记录" description="可以从采集草稿编号带入，或手工填写标题和规格。" actionLabel={canOperate ? '新建上架记录' : undefined} onAction={canOperate ? () => setOpen(true) : undefined} />
        ) : (
          <TmProTable<ListingRow>
            rowKey="id"
            search={false}
            options={false}
            loading={loading}
            columns={columns}
            dataSource={rows}
            pagination={false}
            scroll={{ x: 880 }}
          />
        )}
        <Modal
          title="新建上架记录"
          open={open}
          confirmLoading={submitting}
          okText="保存"
          cancelText="取消"
          onCancel={() => setOpen(false)}
          onOk={() => void submit()}
          destroyOnHidden
          width="min(560px, 92vw)"
        >
          <Form form={form} layout="vertical">
            <Form.Item name="productId" label="商品草稿编号">
              <Input placeholder="可留空" />
            </Form.Item>
            <Form.Item name="sourceUrl" label="来源链接">
              <Input placeholder="https://" />
            </Form.Item>
            <Form.Item name="title" label="标题" rules={[{ required: true, message: '请填写标题' }]}>
              <Input maxLength={500} />
            </Form.Item>
            <Form.Item name="description" label="描述">
              <Input.TextArea rows={3} maxLength={4000} />
            </Form.Item>
            <Space wrap>
              <Form.Item name="currency" label="币种">
                <Input maxLength={3} />
              </Form.Item>
              <Form.Item name="categoryId" label="类目编号">
                <Input />
              </Form.Item>
            </Space>
            <Form.Item name="skuCode" label="规格编码" rules={[{ required: true, message: '请填写规格编码' }]}>
              <Input />
            </Form.Item>
            <Space wrap>
              <Form.Item name="priceMinor" label="价格（分）" rules={[{ required: true, message: '请填写价格' }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
              <Form.Item name="stock" label="库存" rules={[{ required: true, message: '请填写库存' }]}>
                <InputNumber min={0} precision={0} />
              </Form.Item>
            </Space>
            <Form.Item name="submit" label="立即提交记录" valuePropName="checked">
              <Switch />
            </Form.Item>
            <div>{`示例价格 ${minorMoney(1999, 'USD')}`}</div>
          </Form>
        </Modal>
      </TmPageContainer>
    </PermissionGuard>
  );
}
