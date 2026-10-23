# 《来电疑云》

七牛 Builder Day · 灵矽赛题「语音类游戏（非跑团，如狼人杀）」落地作品。

多人语音辨谎：AI/网页主持播报可疑来电 → 玩家轮流盘问 → 交叉质询 → 投票找出「来电骗子」。

## 快速开始

```bash
cd lai-dian-yi-yun
make tidy
make run
```

浏览器打开 [http://127.0.0.1:8080](http://127.0.0.1:8080)

- **单人演示**：点「单人演示（补机器人）」→「开始游戏」→ 按阶段发言/投票  
- **多人**：一人创建房间，其他人输入房间号加入（建议 3–6 人）  
- **语音输入**：按住说话 → 本地 Whisper 识别；也可直接打字  
- **主持播报**：浏览器 TTS 朗读 `hostLine`（对接灵矽后改由设备 TTS 播报）

无界面跑通一局：

```bash
make demo
```

## 涉及灵矽的功能模块

本项目按「灵矽做语音通道与智能体，本服务做对局权威状态」拆分。与灵矽相关的能力如下。

### 1. 本仓库内：灵矽对接层

| 模块 | 路径 | 作用 |
| --- | --- | --- |
| **MCP 工具面** | `internal/mcp` | 向灵矽暴露对局工具，供自定义 MCP / Function Call 调用 |
| **对局状态机** | `internal/game` | 房间阶段、身份、发言、质询、投票；产出主持词 `hostLine` |
| **房间管理** | `internal/room` | 多房间实例、机器人补齐与自动推进 |
| **HTTP / WebSocket API** | `internal/api` | Web 演示端同步状态；亦可被中间层转发 |
| **本地 ASR（演示用）** | `internal/api/asr.go` + `scripts/asr_transcribe.py` | Web 端按住说话的转写；正式环境可换成灵矽 ASR |
| **Web 客户端** | `web/` | 无设备时的完整可玩演示（含主持朗读、投票 UI） |

### 2. 灵矽平台侧应对齐的能力（xrobotd / linxdv0）

| 灵矽能力 | 在《来电疑云》中的用途 |
| --- | --- |
| **设备接入（小智协议 WebSocket）** | 玩家用智能硬件/客户端连上灵矽，说话进游戏 |
| **Realtime API / WebRTC** | 手机/浏览器以 OpenAI Realtime 形态接入，替代或补充设备端 |
| **流式 ASR** | 玩家发言转文字后，经工具写入 `submit_statement` / `submit_cross_answer` / `vote` |
| **流式 TTS** | 播报 `get_state` 返回的 `hostLine`（来电摘要、点名、结算） |
| **主 Agent + 意图 / Function Call** | 识别「开始游戏 / 我投 XXX」等话术，调用 MCP 工具 |
| **自定义 MCP** | 把本服务的 `/mcp/call` 配成智能体工具后端 |
| **APP State + 动态工具**（可选增强） | 按阶段禁用工具（如非投票阶段禁用 `vote`），减少模型误调 |
| **声纹 V2**（可选加分） | 同设备多人时区分说话人，映射到 `player_id` |
| **多音色 / 音色复刻**（可选加分） | 「骗子来电」播报用独立音色增强沉浸感 |

> 说明：灵矽仓库中的 `app/meeting`（会议纪要）是沉浸录音场景，**不是**本游戏的多人房间底座；本游戏房间状态由本服务 MCP 维护。

### 3. 推荐对接链路

```text
玩家语音
  → 灵矽设备 / Realtime 客户端
  → linxdv0（ASR → 主 Agent → TTS）
  → Function Call / 自定义 MCP
  → 本服务 POST /mcp/call
  → internal/game 状态机更新
  → get_state.hostLine 回传
  → 灵矽 TTS 播报给所有玩家会话
```

当前 Web Demo 用浏览器 TTS + 本地 Whisper 模拟「灵矽 ASR/TTS」，逻辑与正式对接一致，只是通道不同。

## 与灵矽（xrobotd）对接

本服务暴露 MCP 风格工具面，可在灵矽控制台配置为自定义 MCP / Function Call 后端：

| 接口 | 说明 |
| --- | --- |
| `GET /mcp/tools` | 工具列表 |
| `POST /mcp/call` | `{"name":"start_game","arguments":{"room_code":"ABCD"}}` |
| `GET /mcp/` | 简易 SSE endpoint 提示 |

| 工具名 | 说明 |
| --- | --- |
| `create_room` | 创建房间 |
| `join_room` | 加入房间 |
| `fill_bots` | 补机器人（单人演示） |
| `start_game` | 开局并分配骗子身份 |
| `advance_phase` | 播报结束 → 进入盘问 |
| `submit_statement` | 提交盘问发言 |
| `submit_cross_answer` | 提交质询回答 |
| `vote` | 投票指认骗子 |
| `get_state` | 拉取公开状态 + 主持词（可附带私密身份卡） |

本地灵矽代码参考：`~/Work/Code/Github/Fork/xrobotd`（已对齐 upstream `qbox/xrobotd` `v0`）。

## 游戏规则（MVP）

1. **大厅**：满 3 人可开局（可补机器人）  
2. **来电播报**：主持宣读混有真假信息的来电摘要  
3. **盘问**：每人发言  
4. **交叉质询**：主持点名追问 2 人  
5. **投票**：指认骗子；命中则市民胜，否则骗子胜  

身份卡仅私发本人；骗子身份只存在服务端，不会进入公开主持词。

## 目录

```text
cmd/server                 入口
internal/game              状态机与剧本（对局核心）
internal/room              房间管理
internal/api               HTTP + WebSocket + 本地 ASR
internal/mcp               灵矽 MCP 工具面
web                        浏览器演示客户端
scripts/asr_transcribe.py  本地 Whisper 转写
scripts/demo.sh            无头端到端演示
```
