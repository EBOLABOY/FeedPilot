# 🍪 NotebookLM Cookies 手动更新指南

> **适用场景**：程序已默认自动刷新 `NLM_AUTH_TOKEN`、`NLM_F_SID`、`NLM_BL`。仅当出现 `401 Signaler refresh failed` 或 `Request had invalid authentication` 且自动刷新失败时，再进行本手动更新。

---

## 📋 快速概览

| 步骤 | 操作 | 预计耗时 |
|------|------|----------|
| 1 | 登录 NotebookLM | 30秒 |
| 2 | 打开开发者工具 | 5秒 |
| 3 | 复制 Cookies | 30秒 |
| 4 | 更新 `.env` 文件 | 30秒 |
| 5 | 重新运行程序 | 10秒 |

**总计：约 2 分钟**

---

## 🤖 先确认自动刷新是否启用

默认情况下，程序会在认证阶段自动从 NotebookLM 页面同步会话参数并尝试刷新凭证。  
请先检查是否误设置了以下开关：

```env
NLM_DISABLE_WEB_SESSION_REFRESH=true
```

如果你希望保持自动同步，请删除该配置或改为 `false`，再重试一次。

---

## 🐳 Docker 交互重登（可选）

如果你在 Docker 中运行，且希望“失效后自动进入可登录页面”，请确保：

```env
NLM_BROWSER_AUTH_ON_REFRESH_FAIL=true
NLM_BROWSER_KEEP_OPEN_SECONDS=600
NLM_ENABLE_VNC_BROWSER=true
NLM_BROWSER_HEADLESS=false
NLM_VNC_PORT=5900
```

当自动刷新失败后，服务会触发容器内浏览器重登流程。  
使用 VNC 客户端连接到容器 `5900` 端口完成登录，程序会自动提取并持久化鉴权信息。

---

## 🔧 详细步骤

### 步骤 1：登录 NotebookLM

1. 打开 Chrome/Edge 浏览器
2. 访问 **[https://notebooklm.google.com/](https://notebooklm.google.com/)**
3. 使用你的 Google 账号登录
4. 确保已成功进入 NotebookLM 主界面（能看到你的 Notebooks 列表）

> ⚠️ **重要**：必须使用与 `.env` 中相同的 Google 账号登录！

---

### 步骤 2：打开开发者工具

**方法一（推荐）：键盘快捷键**
- Windows/Linux: `F12` 或 `Ctrl + Shift + I`
- macOS: `Cmd + Option + I`

**方法二：右键菜单**
- 在页面空白处右键 → 选择 **"检查"** 或 **"Inspect"**

---

### 步骤 3：获取 Cookies

1. 在开发者工具中，点击顶部的 **"Network"（网络）** 标签

2. **刷新页面**（按 `F5` 或 `Ctrl + R`）

3. 在请求列表中，点击第一个请求（通常是 `notebooklm.google.com`）

4. 在右侧面板中，找到 **"Headers"（标头）** 标签

5. 向下滚动到 **"Request Headers"（请求标头）** 部分

6. 找到 **`Cookie:`** 字段（注意是 Request Headers 中的，不是 Response Headers）

7. **复制完整的 Cookie 值**：
   - 双击 Cookie 值选中
   - 或右键 → "Copy value"
   - Cookie 值很长，确保完整复制！

```
示例格式（实际值会更长）：
__Secure-BUCKET=xxx; NID=xxx; HSID=xxx; SSID=xxx; APISID=xxx; SAPISID=xxx; ...
```

---

### 步骤 4：更新 .env 文件

1. 用文本编辑器打开项目根目录下的 `.env` 文件：
   ```
   D:\FeedPilot-1\notebook-podcast-automator\.env
   ```

2. 找到 `NLM_COOKIES=` 这一行

3. 将旧的 Cookie 值替换为新复制的值：
   ```env
   NLM_COOKIES="粘贴你复制的完整Cookie值"
   ```

4. **保存文件** (`Ctrl + S`)

> ⚠️ **注意事项**：
> - Cookie 值必须用英文双引号 `"..."` 包裹
> - Cookie 值中不能有换行符
> - 不要删除其他配置项（如 R2_ACCOUNT_ID 等）

---

### 步骤 5：重新运行程序

在项目目录下执行：

```powershell
go run .
```

然后通过 HTTP 触发一次完整流程（示例）：

```powershell
$body = @{
  input_url = "http://192.168.100.3:10082/atom"
  max_entries = 10
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri http://localhost:8080/run -ContentType "application/json" -Body $body
```

---

## ✅ 验证成功

如果看到以下输出，说明 Cookies 更新成功：

```
[workflow] auth: ensuring NotebookLM credentials
[workflow] notebooklm: creating NotebookLM project
```

---

## ❓ 常见问题

### Q1: Cookie 值有多长？
正常情况下，完整的 Cookie 值约 **2000-3000 个字符**。如果明显比这短，可能没有完整复制。

### Q2: 复制时包含 "Cookie:" 前缀了怎么办？
需要删除前缀，`.env` 中只需要冒号后面的值：
```env
# ❌ 错误
NLM_COOKIES="Cookie: __Secure-BUCKET=xxx..."

# ✅ 正确
NLM_COOKIES="__Secure-BUCKET=xxx..."
```

### Q3: Cookies 多久过期一次？
Google 的认证 Cookies 仍可能在 **24-48 小时** 内变化，但本项目会自动刷新 token 和会话参数（`NLM_F_SID` / `NLM_BL`）。  
因此多数情况下不需要每天手动提取。只有在 Cookies 整体失效、账号状态变化或浏览器登录态失效时，才需要按本文手工更新。

### Q4: 更新后仍然报 401 错误？
可能的原因：
1. Cookie 没有完整复制（缺少部分字段）
2. 复制的是 Response Cookie 而非 Request Cookie
3. 使用了不同的 Google 账号
4. 账号被 Google 限制（尝试换账号或等待）

---

## 📁 相关文件

| 文件 | 说明 |
|------|------|
| `.env` | 存放 Cookies 和其他配置 |
| `.env.example` | 配置文件模板 |
| `internal/auth/credentials.go` | 凭证确保与刷新入口（含会话参数同步） |
| `internal/auth/webtoken.go` | 从 NotebookLM 页面提取 `SNlM0e` / `FdrFJe` / `bl` |
| `internal/auth/refresh.go` | 凭证刷新与过期处理逻辑 |

---

## 🔗 快捷链接

- [NotebookLM 官网](https://notebooklm.google.com/)
- [Chrome DevTools 文档](https://developer.chrome.com/docs/devtools/)

---

*最后更新：2026-01-02*
