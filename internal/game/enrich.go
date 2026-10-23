package game

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// 丰富对局：物证板 → 怀疑标记 → 技能 → 加权质询 → 结算评分

const (
	PhaseEvidence Phase = "evidence" // 物证播报
	PhaseMark     Phase = "mark"     // 怀疑标记
)

type AbilityKind string

const (
	AbilityNone     AbilityKind = ""
	AbilityAudit    AbilityKind = "audit"    // 市民：鉴定一条物证真伪（私密）
	AbilityFocus    AbilityKind = "focus"    // 市民：指定一人必进质询
	AbilitySmoke    AbilityKind = "smoke"    // 骗子：在物证阶段混入一条假线索
)

type EvidenceClue struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	IsTrue bool   `json:"-"` // 仅服务端；公开快照不带真伪
}

type PublicClue struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	Planted  bool   `json:"planted,omitempty"` // 是否被技能污染的假线索（结算前不揭示）
	Revealed bool   `json:"revealed"`
}

type SuspicionMark struct {
	FromPlayerID string `json:"fromPlayerId"`
	ToPlayerID   string `json:"toPlayerId"`
}

type PlayerScore struct {
	PlayerID   string `json:"playerId"`
	Name       string `json:"name"`
	Role       Role   `json:"role,omitempty"`
	Score      int    `json:"score"`
	VotedHit   bool   `json:"votedHit"`
	MarkedHit  bool   `json:"markedHit,omitempty"`
	AbilityUse bool   `json:"abilityUsed,omitempty"`
	Note       string `json:"note,omitempty"`
}

func (r *Room) resetEnrichLocked() {
	r.evidenceClues = nil
	r.publicClues = nil
	r.marks = make(map[string]string)
	r.focusTargetID = ""
	r.smokeUsed = false
	r.scores = nil
	for _, p := range r.Players {
		p.Ability = AbilityNone
		p.AbilityUsed = false
		p.AuditResult = ""
	}
}

func (r *Room) assignAbilitiesLocked() {
	for _, id := range r.order {
		p := r.Players[id]
		if p.Role == RoleScammer {
			p.Ability = AbilitySmoke
			continue
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(2))
		if n.Int64() == 0 {
			p.Ability = AbilityAudit
		} else {
			p.Ability = AbilityFocus
		}
	}
}

func abilityHint(kind AbilityKind) string {
	switch kind {
	case AbilityAudit:
		return "技能【物证鉴定】：物证阶段可鉴定一条线索真伪（仅你可见）。"
	case AbilityFocus:
		return "技能【重点盯防】：怀疑标记阶段可指定一人必进交叉质询。"
	case AbilitySmoke:
		return "技能【烟雾弹】：物证阶段可混入一条误导性假线索（公开播报）。"
	default:
		return ""
	}
}

func (r *Room) buildEvidenceDeckLocked() {
	clues := append([]EvidenceClue{}, r.Script.EvidenceClues...)
	if len(clues) == 0 {
		clues = defaultEvidenceClues(r.Script.Title)
	}
	shuffleClues(clues)
	n := 3
	if len(clues) < n {
		n = len(clues)
	}
	r.evidenceClues = clues[:n]
	r.publicClues = make([]PublicClue, 0, n+1)
	for _, c := range r.evidenceClues {
		r.publicClues = append(r.publicClues, PublicClue{
			ID: c.ID, Title: c.Title, Text: c.Text, Revealed: true,
		})
	}
}

func defaultEvidenceClues(title string) []EvidenceClue {
	_ = title
	return []EvidenceClue{
		{ID: "e1", Title: "官方口径", Text: "银行/公检法不会要求你把钱转到「安全账户」。", IsTrue: true},
		{ID: "e2", Title: "验证码红线", Text: "短信验证码等于账户权限，绝不能告诉来电方。", IsTrue: true},
		{ID: "e3", Title: "工号可信？", Text: "电话里报出的客服工号可以伪造，不能单独作为信任依据。", IsTrue: true},
		{ID: "e4", Title: "催促话术", Text: "「必须立刻转、只能电话办」往往是诈骗催促，真实业务允许你挂断核实。", IsTrue: true},
		{ID: "e5", Title: "误导条目", Text: "只要对方报出你的姓名与部分卡号，就可以确认是官方来电。", IsTrue: false},
	}
}

func shuffleClues(clues []EvidenceClue) {
	for i := len(clues) - 1; i > 0; i-- {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(n.Int64())
		clues[i], clues[j] = clues[j], clues[i]
	}
}

func (r *Room) enterEvidenceLocked() error {
	r.buildEvidenceDeckLocked()
	r.Phase = PhaseEvidence
	var b strings.Builder
	b.WriteString("盘问结束，进入【物证播报】。请听下列关键线索（其中可能混有假信息）：")
	for i, c := range r.publicClues {
		b.WriteString(fmt.Sprintf("%d.%s：%s ", i+1, c.Title, c.Text))
	}
	b.WriteString("听完后可使用技能，或进入怀疑标记。")
	r.hostLine = strings.TrimSpace(b.String())
	r.phaseDeadline = time.Now().Add(50 * time.Second)
	return nil
}

func (r *Room) AdvanceFromEvidence() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseEvidence {
		return ErrWrongPhase
	}
	return r.enterMarkLocked()
}

func (r *Room) enterMarkLocked() error {
	r.Phase = PhaseMark
	r.marks = make(map[string]string)
	r.hostLine = "【怀疑标记】请每人点名你此刻最怀疑的一位。标记将影响交叉质询名单；可同时发动一次技能。"
	r.phaseDeadline = time.Now().Add(40 * time.Second)
	return nil
}

func (r *Room) MarkSuspect(fromID, toID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseMark {
		return ErrWrongPhase
	}
	if _, ok := r.Players[fromID]; !ok {
		return ErrPlayerNotFound
	}
	to, ok := r.Players[toID]
	if !ok || !to.Alive {
		return ErrInvalidTarget
	}
	if fromID == toID {
		return errors.New("不能标记自己")
	}
	if _, ok := r.marks[fromID]; ok {
		return errors.New("你已经标记过了")
	}
	r.marks[fromID] = toID
	r.hostLine = fmt.Sprintf("已收到怀疑标记 %d/%d。", len(r.marks), len(r.Players))
	if len(r.marks) >= len(r.Players) {
		return r.enterCrossExamLocked()
	}
	return nil
}

// UseAbility 发动一次性技能。
// audit: clueID；focus: targetPlayerID；smoke: 无参数（植入默认假线索）。
func (r *Room) UseAbility(playerID, clueID, targetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.Players[playerID]
	if !ok {
		return ErrPlayerNotFound
	}
	if p.AbilityUsed || p.Ability == AbilityNone {
		return errors.New("技能不可用或已使用")
	}
	switch p.Ability {
	case AbilityAudit:
		if r.Phase != PhaseEvidence && r.Phase != PhaseMark {
			return ErrWrongPhase
		}
		clueID = strings.TrimSpace(clueID)
		var found *EvidenceClue
		for i := range r.evidenceClues {
			if r.evidenceClues[i].ID == clueID {
				found = &r.evidenceClues[i]
				break
			}
		}
		if found == nil {
			return errors.New("无效物证 ID")
		}
		if found.IsTrue {
			p.AuditResult = fmt.Sprintf("鉴定结果：【%s】为真线索，可写入公开判断。", found.Title)
		} else {
			p.AuditResult = fmt.Sprintf("鉴定结果：【%s】为假线索，有人在误导现场。", found.Title)
		}
		p.AbilityUsed = true
		r.hostLine = fmt.Sprintf("%s 使用了【物证鉴定】（结果仅本人可见）。", p.Name)
	case AbilityFocus:
		if r.Phase != PhaseMark && r.Phase != PhaseEvidence {
			return ErrWrongPhase
		}
		to, ok := r.Players[targetID]
		if !ok || !to.Alive || targetID == playerID {
			return ErrInvalidTarget
		}
		r.focusTargetID = targetID
		p.AbilityUsed = true
		r.hostLine = fmt.Sprintf("%s 发动【重点盯防】：%s 将被优先质询。", p.Name, to.Name)
	case AbilitySmoke:
		if r.Phase != PhaseEvidence {
			return ErrWrongPhase
		}
		if r.smokeUsed {
			return errors.New("烟雾弹已使用")
		}
		fake := PublicClue{
			ID: "smoke-fake", Title: "临时补充", Text: "有内部消息称：只要配合转账到安全账户，损失今晚就能追回。",
			Planted: true, Revealed: true,
		}
		r.publicClues = append(r.publicClues, fake)
		r.smokeUsed = true
		p.AbilityUsed = true
		r.hostLine = "系统追加一条「临时补充」物证（真伪未知）。" + fake.Text
	default:
		return errors.New("未知技能")
	}
	return nil
}

func (r *Room) pickCrossTargetsLocked(n int) []string {
	if n < 1 {
		n = 2
	}
	scores := map[string]int{}
	for _, id := range r.order {
		scores[id] = 0
	}
	for _, to := range r.marks {
		scores[to]++
	}
	if r.focusTargetID != "" {
		scores[r.focusTargetID] += 3
	}
	type kv struct {
		id string
		n  int
	}
	arr := make([]kv, 0, len(scores))
	for id, sc := range scores {
		arr = append(arr, kv{id, sc})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].n == arr[j].n {
			return arr[i].id < arr[j].id
		}
		return arr[i].n > arr[j].n
	})
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for _, a := range arr {
		if len(out) >= n {
			break
		}
		out = append(out, a.id)
		seen[a.id] = true
	}
	// 补足：随机未入选
	rest := make([]string, 0)
	for _, id := range r.order {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	shuffle(rest)
	for _, id := range rest {
		if len(out) >= n {
			break
		}
		out = append(out, id)
	}
	return out
}

func (r *Room) computeScoresLocked() {
	scammerID := ""
	for id, p := range r.Players {
		if p.Role == RoleScammer {
			scammerID = id
			break
		}
	}
	r.scores = make([]PlayerScore, 0, len(r.order))
	for _, id := range r.order {
		p := r.Players[id]
		ps := PlayerScore{PlayerID: id, Name: p.Name, Role: p.Role, AbilityUse: p.AbilityUsed}
		voteTo := r.votes[id]
		if p.Role == RoleCitizen {
			if voteTo == scammerID {
				ps.VotedHit = true
				ps.Score += 40
				ps.Note = "投票命中骗子"
			} else {
				ps.Note = "投票未中"
			}
			if markTo, ok := r.marks[id]; ok && markTo == scammerID {
				ps.MarkedHit = true
				ps.Score += 20
				if ps.Note != "" {
					ps.Note += "；"
				}
				ps.Note += "怀疑标记命中"
			}
			if p.AbilityUsed {
				ps.Score += 10
			}
			if r.winner == "citizens" {
				ps.Score += 20
			}
		} else {
			// 骗子：存活欺骗成功加分
			if r.winner == "scammer" {
				ps.Score += 50
				ps.Note = "骗过全场"
			} else {
				ps.Note = "身份被识破"
			}
			if p.AbilityUsed {
				ps.Score += 15
				if ps.Note != "" {
					ps.Note += "；"
				}
				ps.Note += "烟雾弹已投放"
			}
			// 被投中则减分
			hitVotes := 0
			for _, to := range r.votes {
				if to == id {
					hitVotes++
				}
			}
			ps.Score -= hitVotes * 5
		}
		if ps.Score < 0 {
			ps.Score = 0
		}
		r.scores = append(r.scores, ps)
	}
	sort.Slice(r.scores, func(i, j int) bool {
		return r.scores[i].Score > r.scores[j].Score
	})
}
