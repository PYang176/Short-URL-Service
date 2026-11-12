# 短链管理端

Vue 3 + TypeScript 管理端，覆盖登录注册、工作台、短链管理、链接详情、
点击趋势、维度排行和二维码下载。

## 本地启动

```powershell
npm install
npm run dev
```

开发地址：`http://127.0.0.1:5173`

Vite 将以下路径代理到后端：

```text
/api -> http://127.0.0.1:18080
/r   -> http://127.0.0.1:18080
```

后端开发环境应将 `PUBLIC_BASE_URL` 指向前端地址，使创建接口返回可访问的短链：

```text
PUBLIC_BASE_URL=http://127.0.0.1:5173
```

## 质量检查

```powershell
npm run typecheck
npm run build
python scripts\e2e.py
```

浏览器验收脚本复用本机 Chrome，并覆盖：

- 注册与自动登录。
- 创建短链接。
- 添加并模拟验证自定义域名。
- 使用自定义域名创建短链并通过 Host 请求跳转。
- 人类与机器人 User-Agent 请求。
- Redis Streams、Worker 和 MySQL 点击汇总。
- 分析页读取。
- PNG/SVG 二维码下载。
- 编辑目标地址。
- 390px 移动端布局。
- 浏览器控制台零错误。

## 图表实现

点击趋势使用项目内 SVG，排名使用 CSS 条，不引入大型图表运行时。
CSP、响应式布局和无脚本图表都更容易控制。
