# 《来电疑云》

七牛 Builder Day · 灵矽赛题「语音类游戏（非跑团，如狼人杀）」落地作品。

多人语音辨谎：AI/网页主持播报可疑来电 → 玩家轮流盘问 → 交叉质询 → 投票找出「来电骗子」。

## 快速开始

```bash
cd /Users/zengzhaoxia/Work/Code/Github/Fork/voice-mystery
make tidy
make run
```

启用灵矽同款 CosyVoice 音色（推荐）：

```bash
export DASHSCOPE_API_KEY=sk-你的百炼密钥
# 可选：COSYVOICE_MODEL=cosyvoice-v2（默认，与 long*_v2 音色匹配）
make run
```

浏览器打开 [http://127.0.0.1:8080](http://127.0.0.1:8080)

- **单人演示**：点「单人演示（补机器人）」→「开始游戏」→ 按阶段发言/投票  
- **多人**：一人创建房间，其他人输入房间号加入（建议 3–6 人）  
- **语音输入**：按住说话 → 本地 Whisper 识别；也可直接打字  
  - 首次使用请先执行 `make asr-deps`（需已安装 [ffmpeg](https://ffmpeg.org/)）  
- **音色播报**（配置了 `DASHSCOPE_API_KEY`）：
  - 主持 → **豪放可爱女**（`longxian_v2`）
  - 玩家轮换 → **温暖元气男** / **实力歌手男** / **智慧青年男**
  - 与灵矽仓库 `TTS_AliyunDoubleStreamTTS` 种子音色一致  
- 未配置密钥时自动回退浏览器 TTS

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
| **CosyVoice TTS（演示用）** | `internal/tts` + `/api/tts` | 灵矽同款四音色；需 `DASHSCOPE_API_KEY` |
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
| **多音色 / 音色复刻**（可选加分） | Demo 已用豪放可爱女 + 三男声；正式可由设备 TTS 播报 |

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

## 游戏规则

1. **大厅**：满 3 人可开局（可补机器人）；可选剧本  
2. **来电播报**：主持宣读混有真假信息的来电摘要；每人获得**隐藏技能**  
3. **盘问**：每人发言  
4. **物证播报**：公开线索板（真假混杂）；可用技能（鉴定 / 烟雾弹）  
5. **怀疑标记**：每人点名最可疑者；标记热度决定质询名单；可用【重点盯防】  
6. **交叉质询**：按怀疑热度追问 2～3 人  
7. **投票**：指认骗子；结算输出**个人评分**与反诈价值复盘  

身份卡仅私发本人；骗子身份只存在服务端，不会进入公开主持词。

| 技能 | 阵营 | 效果 |
| --- | --- | --- |
| 物证鉴定 | 市民 | 私密得知一条物证真伪 |
| 重点盯防 | 市民 | 指定一人必进质询 |
| 烟雾弹 | 骗子 | 物证阶段混入一条误导假线索 |

## 内置剧本

| ID | 标题 | 题材 |
| --- | --- | --- |
| `midnight-bank` | 午夜银行来电 | 伪冒银行风控 / 安全账户 |
| `black-gold-antifraud` | 黑金公司反诈侦查 | 话术组伪装受害者（角色本） |
| `refund-courier` | 快递清关退款来电 | 虚假清关费 / 退款短链 |
| `pig-butcher-love` | 网恋理财杀猪盘 | 交友诱导虚假投资 |
| `ai-voice-kin` | AI 换声亲友急电 | AI 克隆声音催转保证金 |
| `campus-parttime` | 校园兼职刷单局 | 垫付刷单 / 发展下线 |

大厅可选剧本；创建房间时也可传 `scriptId`。

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
