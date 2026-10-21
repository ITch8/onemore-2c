# Temu 店铺运营

本文说明 TradeMind 上的 Temu 卖家运营层：商品上架、价格与库存、订单履约、广告建议、客服与售后。它复用现有 Go 平台接入、管理端页面和统一响应，不另起一套服务。

公开商品页采集仍走采集器里的 `shein_temu` 预留入口，当前未实现。卖家侧动作不使用浏览器自动登录或自动点击。

## 五个模块

| 模块 | 管理端 | 做什么 | 默认行为 |
| --- | --- | --- | --- |
| 商品上架 | `/temu-ops/listings` | 用商品草稿编号或手工来源链接生成 Temu 上架记录 | 演示模式只保存本地记录 |
| 价格与库存 | `/temu-ops/price-stock` | 按成本、毛利、最低价/最高价和竞品价计算跟价，并可带上库存 | 试算不写库；保存只写本地记录 |
| 订单履约 | `/temu-ops/orders` | 拉取订单、准备面单、填写承运商和运单号后回传发货 | 不写入现有订单、库存或仓库账；面单默认是演示引用 |
| 广告优化 | `/temu-ops/ads` | 按花费和成交额给出预算建议，需人工确认后再记录执行 | 未配置广告接口时记为未覆盖，不改预算 |
| 客服与售后 | `/temu-ops/customer-service` | 拉取咨询/售后，生成建议回复，人工确认后保存 | 默认不发送给买家 |

总览页是 `/temu-ops/overview`。

## 凭证与运行模式

敏感字段放在后台「平台开放配置」的 `platform_temu`：应用标识、应用密钥、访问令牌、站点区域、接口地址。应用密钥和访问令牌按现有设置加密保存，页面只展示脱敏值。

也可以用环境变量提供部署级占位。进程环境优先于 `.env`。后台 `platform_temu` 里已保存的非空应用标识、密钥、访问令牌、区域和接口地址会覆盖同名环境变量。运行模式和正式写入开关只看环境变量，不能在页面里打开。不要把真实密钥写进仓库。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `TEMU_API_MODE` | `mock` | 只有精确等于 `live` 才访问 Temu。其他值都当作演示模式。 |
| `TEMU_LIVE_WRITES` | `false` | 正式写入总开关。演示模式忽略它。 |
| `TEMU_APP_KEY` | 空 | 应用标识。可被 `platform_temu.app_key` 覆盖。 |
| `TEMU_APP_SECRET` | 空 | 应用密钥。 |
| `TEMU_ACCESS_TOKEN` | 空 | 访问令牌。 |
| `TEMU_REGION` | `us` | `us`、`eu` 或 `global`。决定默认路由主机。 |
| `TEMU_API_BASE_URL` | 空 | 留空时使用 `openapi-b-us.temu.com`、`openapi-b-eu.temu.com` 或 `openapi-b-global.temu.com`。 |
| `TEMU_HTTP_TIMEOUT_SECONDS` | `20` | 外部请求超时，允许 5 到 60 秒。 |
| `TEMU_API_ALLOW_UNLISTED_HOST` | `false` | 仅开发或测试可放宽主机校验。预发和生产会忽略。 |
| `TEMU_API_TYPE_*` | 见 `.env.example` | Partner 路由方法名。广告和客服发送默认为空，表示接口未覆盖。 |

正式调用还要在当次请求里带 `confirmLive=true`。三个条件缺一个，都不会向 Temu 写入。

店铺连接测试在演示模式下直接返回「未访问 Temu 开放接口」，不会把店铺标成已通过正式校验。

## 官方接口与浏览器

卖家动作走 Temu Partner 开放路由：`POST` JSON，参数含 `type`、`app_key`、`timestamp`、`access_token`、`data_type` 和签名。签名为参数名排序后，用应用密钥包裹再做 MD5 大写。

默认方法名来自公开文档中的常见路由名，必须用店铺实际开通的目录核对后才能当作生产方法：

- 连接测试：`bg.open.accesstoken.info.get`
- 上架：`bg.local.goods.add`
- 价格：`bg.local.goods.price.edit`
- 库存：`bg.local.goods.stock.edit`
- 订单：`bg.order.list.v2.get`
- 面单：`bg.logistics.shipment.document.get`
- 发货回传：`bg.logistics.shipment.confirm`
- 售后拉取：`bg.aftersales.refund.list.get`

广告和客服发送没有默认方法。未配置 `TEMU_API_TYPE_ADS` 或 `TEMU_API_TYPE_CS_SEND` 时，系统明确记为未覆盖或仅本地保存，不启动 Playwright，不登录卖家中心。

现有刊登、订单同步和库存同步工作流不会因为这次接入就自动调用 Temu。Temu 接入登记为测试版，但不实现那些工作流要求的发布、订单和库存接口，因此旧链路保持关闭。

## 现在能不能上生产

| 能力 | 演示模式 | 正式接口 |
| --- | --- | --- |
| 五个模块的本地记录、权限、幂等和总览 | 可评审 | 同一套代码 |
| 上架、跟价、库存、拉单、面单、发货回传 | 本地样例 | 有凭证、打开两个开关并核对方法名后才会请求 |
| 广告自动改预算 | 只生成建议 | 默认未覆盖 |
| 客服自动发送 | 禁止 | 默认禁止；即便配置了发送方法，也要人工确认 |
| 写入现有 ERP 订单和库存 | 不做 | 不做 |
| 1688、拼多多、采集器 | 不改行为 | 不改行为 |

没有店铺真实授权和方法目录核对之前，应保持 `TEMU_API_MODE=mock` 与 `TEMU_LIVE_WRITES=false`。

## 权限

- `temu.view`：查看五个模块。管理员、运营、复核和只读都有。
- `temu.operate`：创建和确认记录。只读和复核没有。

接口前缀是 `/api/v1/temu-ops`，响应仍是 `{ code, message, data }`。
