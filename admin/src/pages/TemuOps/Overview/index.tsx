import { history } from '@umijs/max';
import { Alert, Button, Space, Spin, Typography } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import PermissionGuard from '@/components/PermissionGuard';
import { ErrorAlert, SectionCard, TmPageContainer } from '@/components/ui';
import { getTemuStatus, type TemuStatus } from '@/services/temuOps';
import { PERMISSIONS } from '@/utils/permission';
import { temuErrorText, temuModeLabel, temuTransportLabel } from '../labels';
import '../temu-ops.less';

const { Paragraph, Text } = Typography;

const modules = [
  { title: '商品上架', path: '/temu-ops/listings', field: 'listingCount', text: '把采集草稿或手工来源保存为 Temu 上架记录。' },
  { title: '价格与库存', path: '/temu-ops/price-stock', field: 'priceRuleCount', text: '按成本和竞品价计算跟价，确认后才记录执行。' },
  { title: '订单履约', path: '/temu-ops/orders', field: 'orderCount', text: '拉取订单、准备面单并回传发货。不写入现有订单账。' },
  { title: '广告优化', path: '/temu-ops/ads', field: 'adSuggestionCount', text: '生成预算建议。默认不改广告，也不使用浏览器自动操作。' },
  { title: '客服与售后', path: '/temu-ops/customer-service', field: 'ticketCount', text: '生成建议回复，人工确认后默认只保存在本地。' },
] as const;

export default function TemuOpsOverviewPage() {
  const [status, setStatus] = useState<TemuStatus>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setStatus(await getTemuStatus());
    } catch (nextError) {
      setError(temuErrorText(nextError, 'Temu 运营状态加载失败，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <PermissionGuard require={PERMISSIONS.TEMU_VIEW} showForbiddenPage>
      <TmPageContainer className="temu-ops-page" title="Temu 运营总览" subTitle="五个店铺动作共用同一套凭证、演示模式和人工确认。">
        {loading ? <Spin /> : null}
        {error ? <ErrorAlert title="状态加载失败" actionHint={error} action={<Button onClick={() => void load()}>重试</Button>} /> : null}
        {status ? (
          <>
            <Alert
              type={status.liveWrites ? 'warning' : 'info'}
              showIcon
              message={`${temuModeLabel(status.mode)} · ${status.liveWrites ? '正式写入已打开' : '正式写入关闭'} · ${status.credentialsConfigured ? '凭证已配置' : '凭证未配置'}`}
              description={`广告：${temuTransportLabel(status.adsTransport)}。客服发送：${temuTransportLabel(status.csSendTransport)}。${status.routerHost ? `接口主机：${status.routerHost}。` : ''}`}
            />
            <div className="temu-ops-grid" style={{ marginTop: 16 }}>
              {modules.map((item) => (
                <SectionCard key={item.path} title={item.title} description={item.text}>
                  <Space direction="vertical">
                    <Text>本地记录 {status[item.field]}</Text>
                    <Button onClick={() => history.push(item.path)}>进入</Button>
                  </Space>
                </SectionCard>
              ))}
            </div>
            <div style={{ marginTop: 16 }}>
              {status.notes?.map((note) => (
                <Paragraph key={note} className="temu-ops-note">
                  {note}
                </Paragraph>
              ))}
            </div>
          </>
        ) : null}
      </TmPageContainer>
    </PermissionGuard>
  );
}
