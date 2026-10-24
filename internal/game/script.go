package game

import (
	"fmt"
	"strings"
)

// StoryCharacter 剧本固定角色（如黑金公司案五人），开局随机分给玩家。
type StoryCharacter struct {
	Name       string
	Label      string // 如「受害者·餐饮店主」
	SecretCard string
	IsScammer  bool   // 本局中需被指认的隐藏角色（仅一名）
}

// CallScript 是一局固定剧本：真线索与假线索混在「来电」里，骗子持有完整谎言剧本。
type CallScript struct {
	ID          string
	Title       string
	Synopsis    string // 选关卡片上的短梗概
	CallerHint  string // 来电界面副标题
	Briefing    string
	CitizenCard     string
	ScammerCard     string
	CrossQuestions  []string
	BotStatements   map[Role][]string
	BotCrossAnswers map[Role][]string
	BotNames        []string // 补机器人时优先使用的昵称
	VoteLabel       string   // Web 投票区标题
	CitizenRoleLabel string
	ScammerRoleLabel string
	VotePrompt        string
	StatementOpenFmt  string // 盘问开场，含一个 %s（玩家名）
	RevealTieFmt      string // %s 隐藏嫌疑人名
	RevealCitizensWinFmt string // %s 隐藏嫌疑人名
	RevealScammerWinFmt  string // %s 被误投者, %s 隐藏嫌疑人名
	Epilogue          string
	Characters        []StoryCharacter // 非空时按角色发身份卡，替代 CitizenCard/ScammerCard

	// 结算价值复盘（反诈宣传 / 安全教育等）
	Theme       string   // 如：银行反诈宣传 · 安全教育
	ValuePoints []string // 关键防范点
	Closing     string   // 收束寄语

	// 物证板（盘问后公开，真伪混杂；真伪仅服务端持有）
	EvidenceClues []EvidenceClue
}

// ScriptMeta 供大厅展示，不含剧透身份卡。
type ScriptMeta struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Synopsis    string `json:"synopsis"`
	CallerHint  string `json:"callerHint"`
	PlayerCount int    `json:"playerCount"`
	FixedRoster bool   `json:"fixedRoster"`
}

const ScriptMidnightBank = "midnight-bank"
const ScriptBlackGold = "black-gold-antifraud"
// 其余剧本 ID 见 script_extra.go

func (s CallScript) votePrompt() string {
	if t := strings.TrimSpace(s.VotePrompt); t != "" {
		return t
	}
	return "投票阶段：请投出你认为的「来电骗子」。每人一票，不能弃权。"
}

func (s CallScript) statementOpen(name string) string {
	if t := strings.TrimSpace(s.StatementOpenFmt); t != "" {
		return fmt.Sprintf(t, name)
	}
	return fmt.Sprintf("盘问开始。请 %s 先发言：你听到了什么？你信哪一句？", name)
}

func (s CallScript) revealTie(scammerName string) string {
	if t := strings.TrimSpace(s.RevealTieFmt); t != "" {
		return fmt.Sprintf(t, scammerName)
	}
	return fmt.Sprintf("平票！骗子逃脱。真正的来电骗子是 %s。", scammerName)
}

func (s CallScript) revealCitizensWin(scammerName string) string {
	if t := strings.TrimSpace(s.RevealCitizensWinFmt); t != "" {
		return fmt.Sprintf(t, scammerName)
	}
	return fmt.Sprintf("投票命中！%s 就是来电骗子。市民胜利。", scammerName)
}

func (s CallScript) revealScammerWin(accusedName, scammerName string) string {
	if t := strings.TrimSpace(s.RevealScammerWinFmt); t != "" {
		return fmt.Sprintf(t, accusedName, scammerName)
	}
	return fmt.Sprintf("投错了！大家票给了 %s，真正的骗子是 %s。骗子胜利。", accusedName, scammerName)
}

func (s CallScript) voteLabel() string {
	if t := strings.TrimSpace(s.VoteLabel); t != "" {
		return t
	}
	return "指认来电骗子"
}

func (s CallScript) citizenRoleLabel() string {
	if t := strings.TrimSpace(s.CitizenRoleLabel); t != "" {
		return t
	}
	return "市民"
}

func (s CallScript) scammerRoleLabel() string {
	if t := strings.TrimSpace(s.ScammerRoleLabel); t != "" {
		return t
	}
	return "来电骗子（隐藏）"
}

// HasFixedRoster 固定角色剧本：人数必须等于 len(Characters)。
func (s CallScript) HasFixedRoster() bool {
	return len(s.Characters) > 0
}

func (s CallScript) MinPlayersForScript() int {
	if s.HasFixedRoster() {
		return len(s.Characters)
	}
	return MinPlayers
}

func (s CallScript) MaxPlayersForScript() int {
	if s.HasFixedRoster() {
		return len(s.Characters)
	}
	return MaxPlayers
}

func DefaultScriptID() string {
	return ScriptMidnightBank
}

func DefaultScript() CallScript {
	s, _ := ScriptByID(DefaultScriptID())
	return s
}

func ScriptMetas() []ScriptMeta {
	out := make([]ScriptMeta, 0, len(scriptRegistry))
	for _, id := range scriptOrder {
		s, ok := scriptRegistry[id]
		if !ok {
			continue
		}
		out = append(out, ScriptMeta{
			ID:          s.ID,
			Title:       s.Title,
			Synopsis:    s.Synopsis,
			CallerHint:  s.CallerHint,
			PlayerCount: s.MinPlayersForScript(),
			FixedRoster: s.HasFixedRoster(),
		})
	}
	return out
}

func ScriptByID(id string) (CallScript, error) {
	if id == "" {
		id = DefaultScriptID()
	}
	s, ok := scriptRegistry[id]
	if !ok {
		return CallScript{}, fmt.Errorf("未知剧本: %s", id)
	}
	return s, nil
}

var scriptOrder = []string{
	ScriptMidnightBank,
	ScriptBlackGold,
	ScriptRefundCourier,
	ScriptPigButcher,
	ScriptAIVoiceKin,
	ScriptCampusPartTime,
}

var scriptRegistry = map[string]CallScript{
	ScriptMidnightBank:   scriptMidnightBank(),
	ScriptBlackGold:      scriptBlackGold(),
	ScriptRefundCourier:  scriptRefundCourier(),
	ScriptPigButcher:     scriptPigButcher(),
	ScriptAIVoiceKin:     scriptAIVoiceKin(),
	ScriptCampusPartTime: scriptCampusPartTime(),
}

func scriptMidnightBank() CallScript {
	return CallScript{
		ID:         ScriptMidnightBank,
		Title:      "午夜银行来电",
		Synopsis:   "自称星河银行风控的深夜来电，跨境异常、安全账户与验证码——谁在圆谎？",
		CallerHint: "未知来电 · 星河银行？",
		Briefing: `总台播报：刚才有一通自称「星河银行风控中心」的来电。
来电内容摘要如下——请仔细听，其中混有真假信息：
一、对方称你的账户今日出现一笔异常跨境转账，金额约三万八。
二、对方要求你提供短信验证码，并立刻转账到「安全账户」做冻结保护。
三、对方报出的客服工号是 HG-9281，并声称今晚系统升级、只能电话处理。
四、对方说你的常用设备在外地登录过。

各位接线员（玩家）刚刚都接到了相关线索。现在进入盘问阶段：请逐一说明你听到了什么、你信哪一句、你怀疑谁在撒谎。`,
		CitizenCard: `你的身份：市民（普通人）。
你掌握的可靠事实：
1. 星河银行官方绝不会要求客户把钱转到所谓「安全账户」。
2. 真正的风控来电不会索要短信验证码。
3. 你确实今天下午收到过一条银行短信，但只是普通消费提醒，不是跨境转账。
请用口语解释这些点，但不要直接说「我是市民」。尽量通过细节让别人相信你。`,
		ScammerCard: `你的身份：来电骗子（隐藏身份）。
你的任务：混在市民中，把假来电说成真的，并引导大家投错人。
你的谎言要点：
1. 坚称自己也接到了「异常跨境三万八」的通知，听起来很急。
2. 为「安全账户」和「验证码」辩解：说是临时保护流程。
3. 可以攻击别人「反应太大」「故意搅局」。
绝对不要承认自己是骗子。被追问时优先甩锅给发言最清楚的人。`,
		CrossQuestions: []string{
			"如果银行真要保护账户，为什么一定要你立刻转账？请用一句话回答。",
			"你刚才说的『客服工号』，你怎么确认它是真的？",
			"假如这是诈骗，你觉得房间里谁最像在帮骗子圆谎？为什么？",
		},
		BotStatements: map[Role][]string{
			RoleCitizen: {
				"我接到的信息里，最假的是让人转安全账户。正规银行不会这么干。",
				"验证码那一段太危险了。谁要验证码，我就直接挂断。",
				"我下午确实有银行短信，但只是消费提醒，根本没有跨境三万八。",
			},
			RoleScammer: {
				"我也接到了，对方说跨境异常，听起来挺专业的，工号都报了。",
				"他们说今晚系统升级，只能电话处理，所以才这么急。",
				"我觉得有人反应过度，也许是在转移视线。",
			},
		},
		BotCrossAnswers: map[Role][]string{
			RoleCitizen: {
				"正因为要转账，所以更像诈骗，我不会照做。",
				"工号可以官网上查，电话里报一个数字说明不了什么。",
				"谁一直强调『必须立刻转』，我就怀疑谁。",
			},
			RoleScammer: {
				"对方说是临时保护，转过去才能冻结，不然损失更大。",
				"工号我记下了，HG-9281，听着挺正规。",
				"我觉得说话最冲的那个人很可疑。",
			},
		},
		Theme: "银行反诈宣传 · 防伪冒客服安全教育",
		ValuePoints: []string{
			"正规银行、支付机构绝不会要求你把钱转到所谓「安全账户」「保证金账户」。",
			"任何索要短信验证码、动态口令的来电/链接都要警惕：验证码等于把账户交给对方。",
			"「系统升级、只能电话办、必须立刻转」是典型催促话术，真实业务允许你挂断后走官方渠道核实。",
			"客服工号、来电显示可以伪造；请通过官方 App、官网客服或网点二次确认，不要回拨陌生号码。",
			"收到异常提醒时：不转账、不泄密、不远程操控；可拨打银行官方客服或报警电话核实。",
		},
		Closing: "《来电疑云》用一局游戏演练真实骗局。把今晚记住的防范点带回家、带进单位：识破一通电话，就可能守住一笔积蓄。",
		EvidenceClues: []EvidenceClue{
			{ID: "mb1", Title: "安全账户", Text: "正规银行不会要求客户把资金转到所谓「安全账户」。", IsTrue: true},
			{ID: "mb2", Title: "验证码", Text: "任何索要短信验证码的来电都应直接挂断。", IsTrue: true},
			{ID: "mb3", Title: "工号 HG-9281", Text: "电话报工号即可证明对方是官方客服。", IsTrue: false},
			{ID: "mb4", Title: "系统升级", Text: "「今晚只能电话处理」是常见催促话术，真实业务可挂断后走官方渠道。", IsTrue: true},
			{ID: "mb5", Title: "跨境三万八", Text: "只要金额对得上短信，就可以按来电指示操作。", IsTrue: false},
		},
	}
}
