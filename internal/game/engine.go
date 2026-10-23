package game

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MinPlayers = 3
	MaxPlayers = 6
)

var (
	ErrRoomFull        = errors.New("房间已满")
	ErrNameTaken       = errors.New("昵称已被占用")
	ErrNotLobby        = errors.New("当前不在大厅阶段")
	ErrNotEnoughPlayers = errors.New("至少需要3名玩家才能开始")
	ErrWrongPhase      = errors.New("当前阶段不允许此操作")
	ErrNotYourTurn     = errors.New("还没轮到你发言")
	ErrAlreadyVoted    = errors.New("你已经投过票了")
	ErrInvalidTarget   = errors.New("无效的投票对象")
	ErrPlayerNotFound  = errors.New("玩家不存在")
	ErrGameStarted     = errors.New("对局已开始，无法加入")
)

type Room struct {
	mu sync.RWMutex

	Code   string
	Script CallScript
	Phase  Phase

	Players map[string]*Player
	order   []string // join order / statement order

	statements     []Statement
	speakerIdx     int
	crossQuestions []CrossQuestion
	crossIdx       int
	votes          map[string]string // from -> to
	hostLine       string
	winner         string
	revealed       string
	valueTheme     string
	valuePoints    []string
	valueClosing   string
	valueSummary   string
	phaseDeadline  time.Time
	createdAt      time.Time
	nextBotActAt   time.Time // 限制机器人出手速度，给真人留操作时间
}

func NewRoom(code string, scriptID string) *Room {
	script, err := ScriptByID(scriptID)
	if err != nil {
		script = DefaultScript()
	}
	return &Room{
		Code:      strings.ToUpper(code),
		Script:    script,
		Phase:     PhaseLobby,
		Players:   make(map[string]*Player),
		votes:     make(map[string]string),
		hostLine:  "欢迎来到《来电疑云》。创建或加入房间，满 3 人后可由任意玩家开始对局。",
		createdAt: time.Now(),
	}
}

func NewPlayerID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func RandomRoomCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 4)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		code[i] = alphabet[n.Int64()]
	}
	return string(code)
}

func (r *Room) AddPlayer(name string, isBot bool) (*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseLobby {
		return nil, ErrGameStarted
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("昵称不能为空")
	}
	if len(r.Players) >= MaxPlayers {
		return nil, ErrRoomFull
	}
	for _, p := range r.Players {
		if strings.EqualFold(p.Name, name) {
			return nil, ErrNameTaken
		}
	}
	p := &Player{
		ID:       NewPlayerID(),
		Name:     name,
		IsBot:    isBot,
		Alive:    true,
		JoinedAt: time.Now(),
	}
	r.Players[p.ID] = p
	r.order = append(r.order, p.ID)
	r.hostLine = fmt.Sprintf("%s 已入座。当前 %d/%d 人。", p.Name, len(r.Players), MaxPlayers)
	return p, nil
}

func (r *Room) FillBots(target int) ([]*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseLobby {
		return nil, ErrNotLobby
	}
	if target < MinPlayers {
		target = MinPlayers
	}
	if target > MaxPlayers {
		target = MaxPlayers
	}
	botNames := []string{"阿北", "小满", "老周", "青青", "阿凯"}
	if len(r.Script.BotNames) > 0 {
		botNames = r.Script.BotNames
	}
	added := make([]*Player, 0)
	idx := 0
	for len(r.Players) < target {
		name := botNames[idx%len(botNames)]
		idx++
		// ensure unique
		candidate := name
		suffix := 1
		for r.nameTakenLocked(candidate) {
			suffix++
			candidate = fmt.Sprintf("%s%d", name, suffix)
		}
		p := &Player{
			ID:       NewPlayerID(),
			Name:     candidate,
			IsBot:    true,
			Alive:    true,
			JoinedAt: time.Now(),
		}
		r.Players[p.ID] = p
		r.order = append(r.order, p.ID)
		added = append(added, p)
	}
	r.hostLine = fmt.Sprintf("已补齐至 %d 人，可以说「开始游戏」。", len(r.Players))
	return added, nil
}

func (r *Room) nameTakenLocked(name string) bool {
	for _, p := range r.Players {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

func (r *Room) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseLobby {
		return ErrWrongPhase
	}
	if len(r.Players) < MinPlayers {
		return ErrNotEnoughPlayers
	}

	ids := append([]string{}, r.order...)
	shuffle(ids)
	scammerID := ids[0]
	for _, id := range r.order {
		p := r.Players[id]
		if id == scammerID {
			p.Role = RoleScammer
		} else {
			p.Role = RoleCitizen
		}
	}
	r.assignStoryCharactersLocked(scammerID)

	r.statements = nil
	r.votes = make(map[string]string)
	r.crossQuestions = nil
	r.crossIdx = 0
	r.speakerIdx = 0
	r.winner = ""
	r.revealed = ""
	r.valueTheme = ""
	r.valuePoints = nil
	r.valueClosing = ""
	r.valueSummary = ""
	r.Phase = PhaseBriefing
	r.hostLine = r.Script.Briefing
	// 播报阶段不自动跳转：等主持词念完后由玩家/MCP 手动 advance。
	r.phaseDeadline = time.Time{}
	return nil
}

func (r *Room) AdvanceFromBriefing() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseBriefing {
		return ErrWrongPhase
	}
	r.Phase = PhaseStatement
	r.speakerIdx = 0
	cur := r.Players[r.order[r.speakerIdx]]
	r.hostLine = r.Script.statementOpen(cur.Name)
	r.phaseDeadline = time.Now().Add(45 * time.Second)
	return nil
}

func (r *Room) SubmitStatement(playerID, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseStatement {
		return ErrWrongPhase
	}
	if r.order[r.speakerIdx] != playerID {
		return ErrNotYourTurn
	}
	p := r.Players[playerID]
	if p == nil {
		return ErrPlayerNotFound
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("发言不能为空")
	}
	r.statements = append(r.statements, Statement{
		PlayerID: playerID,
		Name:     p.Name,
		Text:     text,
		At:       time.Now(),
	})
	r.speakerIdx++
	if r.speakerIdx >= len(r.order) {
		return r.enterCrossExamLocked()
	}
	cur := r.Players[r.order[r.speakerIdx]]
	r.hostLine = fmt.Sprintf("%s 发言完毕。下一位：%s。", p.Name, cur.Name)
	r.phaseDeadline = time.Now().Add(45 * time.Second)
	return nil
}

func (r *Room) enterCrossExamLocked() error {
	qs := r.Script.CrossQuestions
	if len(qs) == 0 {
		qs = []string{"你觉得谁最像骗子？"}
	}
	// pick up to 2 distinct alive players
	targets := append([]string{}, r.order...)
	shuffle(targets)
	n := 2
	if len(targets) < n {
		n = len(targets)
	}
	r.crossQuestions = nil
	for i := 0; i < n; i++ {
		r.crossQuestions = append(r.crossQuestions, CrossQuestion{
			TargetPlayerID: targets[i],
			Question:       qs[i%len(qs)],
		})
	}
	r.crossIdx = 0
	r.Phase = PhaseCrossExam
	t := r.Players[r.crossQuestions[0].TargetPlayerID]
	r.hostLine = fmt.Sprintf("交叉质询：%s，%s", t.Name, r.crossQuestions[0].Question)
	r.phaseDeadline = time.Now().Add(40 * time.Second)
	return nil
}

func (r *Room) SubmitCrossAnswer(playerID, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseCrossExam {
		return ErrWrongPhase
	}
	if r.crossIdx >= len(r.crossQuestions) {
		return ErrWrongPhase
	}
	cur := &r.crossQuestions[r.crossIdx]
	if cur.TargetPlayerID != playerID {
		return ErrNotYourTurn
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("回答不能为空")
	}
	cur.Answer = text
	r.crossIdx++
	if r.crossIdx >= len(r.crossQuestions) {
		return r.enterVoteLocked()
	}
	next := r.crossQuestions[r.crossIdx]
	t := r.Players[next.TargetPlayerID]
	r.hostLine = fmt.Sprintf("继续质询：%s，%s", t.Name, next.Question)
	r.phaseDeadline = time.Now().Add(40 * time.Second)
	return nil
}

func (r *Room) enterVoteLocked() error {
	r.Phase = PhaseVote
	r.votes = make(map[string]string)
	r.hostLine = r.Script.votePrompt()
	r.phaseDeadline = time.Now().Add(60 * time.Second)
	return nil
}

func (r *Room) CastVote(fromID, toID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != PhaseVote {
		return ErrWrongPhase
	}
	if _, ok := r.Players[fromID]; !ok {
		return ErrPlayerNotFound
	}
	to, ok := r.Players[toID]
	if !ok || !to.Alive {
		return ErrInvalidTarget
	}
	if _, voted := r.votes[fromID]; voted {
		return ErrAlreadyVoted
	}
	r.votes[fromID] = toID
	if len(r.votes) >= len(r.Players) {
		return r.revealLocked()
	}
	r.hostLine = fmt.Sprintf("已收到 %d/%d 票。", len(r.votes), len(r.Players))
	return nil
}

func (r *Room) revealLocked() error {
	counts := map[string]int{}
	for _, to := range r.votes {
		counts[to]++
	}
	var topID string
	topN := -1
	tie := false
	for id, n := range counts {
		if n > topN {
			topN = n
			topID = id
			tie = false
		} else if n == topN {
			tie = true
		}
	}

	var scammer *Player
	for _, p := range r.Players {
		if p.Role == RoleScammer {
			scammer = p
			break
		}
	}
	r.Phase = PhaseReveal
	if scammer != nil {
		r.revealed = scammer.Name
		if scammer.characterName != "" {
			r.revealed = scammer.characterName
		}
	}
	scammerName := r.revealed
	if scammer != nil {
		scammerName = scammer.Name
	}
	if tie || topID == "" {
		r.winner = "scammer"
		r.hostLine = r.Script.revealTie(scammerName)
	} else if scammer != nil && topID == scammer.ID {
		r.winner = "citizens"
		r.hostLine = r.Script.revealCitizensWin(scammer.Name)
	} else {
		r.winner = "scammer"
		accused := r.Players[topID]
		r.hostLine = r.Script.revealScammerWin(accused.Name, scammerName)
	}
	// 有结构化价值点时走价值复盘卡片；否则回退剧本 Epilogue 拼进主持词
	if len(r.Script.ValuePoints) == 0 {
		if ep := strings.TrimSpace(r.Script.Epilogue); ep != "" {
			r.hostLine = r.hostLine + "\n\n" + ep
		}
	}
	r.applyValueRecapLocked()
	r.Phase = PhaseEnd
	r.phaseDeadline = time.Time{}
	return nil
}

// applyValueRecapLocked 按剧本情节生成结算价值复盘（反诈宣传 / 安全教育）。
func (r *Room) applyValueRecapLocked() {
	theme := strings.TrimSpace(r.Script.Theme)
	if theme == "" {
		theme = "反诈宣传 · 安全教育"
	}
	points := append([]string{}, r.Script.ValuePoints...)
	if len(points) == 0 {
		points = []string{
			"不轻信陌生来电的紧急转账、验证码索要要求。",
			"涉及资金与账户操作，请走官方 App 或网点核实。",
			"保持冷静，拒绝催促：给自己留出查证时间。",
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("本局剧本《%s》。主题：%s。", r.Script.Title, theme))
	if r.winner == "citizens" {
		b.WriteString("大家识破了骗局。请把下列防范点带走：")
	} else {
		b.WriteString("骗子本局得逞——现实中一次误判就可能造成损失。请对照下列防范点复盘：")
	}
	for i, p := range points {
		b.WriteString(fmt.Sprintf("%d、%s", i+1, strings.TrimSpace(p)))
		if i < len(points)-1 {
			b.WriteString(" ")
		}
	}
	closing := strings.TrimSpace(r.Script.Closing)
	if closing == "" {
		closing = "玩游戏是入口，守住钱袋子、守住安全感才是目的。把今晚学到的判断力用在下一次真实来电里。"
	}
	b.WriteString(" ")
	b.WriteString(closing)

	r.valueTheme = theme
	r.valuePoints = points
	r.valueClosing = closing
	r.valueSummary = b.String()
	// 主持词附上简短引导，完整复盘见 valueSummary（前端展示并朗读）
	r.hostLine = r.hostLine + " 接下来进入本局价值复盘。"
}

func (r *Room) AutoActBots() (changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Now().Before(r.nextBotActAt) {
		return false
	}

	switch r.Phase {
	case PhaseBriefing:
		// 不自动结束播报，避免打断朗读主持词。
		return false
	case PhaseStatement:
		curID := r.order[r.speakerIdx]
		cur := r.Players[curID]
		if cur != nil && cur.IsBot {
			text := r.pickBotLine(cur.Role, true)
			_ = r.submitStatementNoLock(curID, text)
			// 盘问间隔：兼顾节奏与 TTS 朗读
			r.nextBotActAt = time.Now().Add(2 * time.Second)
			return true
		}
	case PhaseCrossExam:
		if r.crossIdx < len(r.crossQuestions) {
			cur := r.crossQuestions[r.crossIdx]
			p := r.Players[cur.TargetPlayerID]
			if p != nil && p.IsBot {
				text := r.pickBotLine(p.Role, false)
				_ = r.submitCrossNoLock(cur.TargetPlayerID, text)
				r.nextBotActAt = time.Now().Add(2 * time.Second)
				return true
			}
		}
	case PhaseVote:
		for _, id := range r.order {
			if _, ok := r.votes[id]; ok {
				continue
			}
			p := r.Players[id]
			if p == nil || !p.IsBot {
				continue
			}
			target := r.botVoteTarget(id)
			_ = r.castVoteNoLock(id, target)
			r.nextBotActAt = time.Now().Add(1200 * time.Millisecond)
			return true
		}
	}
	return false
}

func (r *Room) advanceBriefingNoLock() error {
	r.Phase = PhaseStatement
	r.speakerIdx = 0
	cur := r.Players[r.order[r.speakerIdx]]
	r.hostLine = r.Script.statementOpen(cur.Name)
	r.phaseDeadline = time.Now().Add(45 * time.Second)
	return nil
}

func (r *Room) submitStatementNoLock(playerID, text string) error {
	p := r.Players[playerID]
	r.statements = append(r.statements, Statement{PlayerID: playerID, Name: p.Name, Text: text, At: time.Now()})
	r.speakerIdx++
	if r.speakerIdx >= len(r.order) {
		return r.enterCrossExamLockedUnsafe()
	}
	cur := r.Players[r.order[r.speakerIdx]]
	r.hostLine = fmt.Sprintf("%s 发言完毕。下一位：%s。", p.Name, cur.Name)
	r.phaseDeadline = time.Now().Add(45 * time.Second)
	return nil
}

func (r *Room) enterCrossExamLockedUnsafe() error {
	return r.enterCrossExamLocked()
}

func (r *Room) submitCrossNoLock(playerID, text string) error {
	cur := &r.crossQuestions[r.crossIdx]
	cur.Answer = text
	r.crossIdx++
	if r.crossIdx >= len(r.crossQuestions) {
		return r.enterVoteLocked()
	}
	next := r.crossQuestions[r.crossIdx]
	t := r.Players[next.TargetPlayerID]
	r.hostLine = fmt.Sprintf("继续质询：%s，%s", t.Name, next.Question)
	r.phaseDeadline = time.Now().Add(40 * time.Second)
	return nil
}

func (r *Room) castVoteNoLock(fromID, toID string) error {
	r.votes[fromID] = toID
	if len(r.votes) >= len(r.Players) {
		return r.revealLocked()
	}
	r.hostLine = fmt.Sprintf("已收到 %d/%d 票。", len(r.votes), len(r.Players))
	return nil
}

func (r *Room) pickBotLine(role Role, statement bool) string {
	var lines []string
	if statement {
		lines = r.Script.BotStatements[role]
	} else {
		lines = r.Script.BotCrossAnswers[role]
	}
	if len(lines) == 0 {
		return "我再想想……感觉有人在撒谎。"
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(lines))))
	return lines[n.Int64()]
}

func (r *Room) botVoteTarget(selfID string) string {
	self := r.Players[selfID]
	// scammer votes a random citizen; citizen votes someone who sounds suspicious — prefer non-self
	candidates := make([]string, 0)
	for _, id := range r.order {
		if id == selfID {
			continue
		}
		candidates = append(candidates, id)
	}
	if len(candidates) == 0 {
		return selfID
	}
	if self.Role == RoleScammer {
		// avoid voting self; pick a citizen
		citizens := make([]string, 0)
		for _, id := range candidates {
			if r.Players[id].Role == RoleCitizen {
				citizens = append(citizens, id)
			}
		}
		if len(citizens) > 0 {
			candidates = citizens
		}
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
	return candidates[n.Int64()]
}

func (r *Room) assignStoryCharactersLocked(scammerPlayerID string) {
	chars := r.Script.Characters
	if len(chars) == 0 {
		return
	}
	scammerIdx := -1
	for i, c := range chars {
		if c.IsScammer {
			scammerIdx = i
			break
		}
	}
	if scammerIdx < 0 {
		return
	}
	others := make([]int, 0, len(chars)-1)
	for i := range chars {
		if i != scammerIdx {
			others = append(others, i)
		}
	}
	for i := len(others) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		others[i], others[j.Int64()] = others[j.Int64()], others[i]
	}
	oi := 0
	for _, id := range r.order {
		p := r.Players[id]
		p.characterName = ""
		p.characterLabel = ""
		p.characterCard = ""
		var ci int
		if id == scammerPlayerID {
			ci = scammerIdx
		} else if oi < len(others) {
			ci = others[oi]
			oi++
		} else {
			ci = others[oi%len(others)]
			oi++
		}
		c := chars[ci]
		p.characterName = c.Name
		p.characterLabel = c.Label
		p.characterCard = c.SecretCard
	}
}

func (r *Room) PrivateView(playerID string) (*PrivateView, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.Players[playerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	if r.Phase == PhaseLobby {
		return &PrivateView{PlayerID: playerID, Role: "", SecretCard: "对局开始后将下发你的秘密身份卡。"}, nil
	}
	if p.characterCard != "" {
		return &PrivateView{
			PlayerID:       playerID,
			Role:           p.Role,
			SecretCard:     p.characterCard,
			CharacterName:  p.characterName,
			CharacterLabel: p.characterLabel,
		}, nil
	}
	card := r.Script.CitizenCard
	if p.Role == RoleScammer {
		card = r.Script.ScammerCard
	}
	return &PrivateView{PlayerID: playerID, Role: p.Role, SecretCard: card}, nil
}

func (r *Room) Snapshot() PublicState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	players := make([]PublicPlayer, 0, len(r.order))
	for _, id := range r.order {
		p := r.Players[id]
		players = append(players, PublicPlayer{
			ID: p.ID, Name: p.Name, IsBot: p.IsBot, Alive: p.Alive, Ready: p.Ready,
		})
	}

	var deadline int64
	if !r.phaseDeadline.IsZero() {
		deadline = r.phaseDeadline.Unix()
	}
	curSpeaker := ""
	if r.Phase == PhaseStatement && r.speakerIdx < len(r.order) {
		curSpeaker = r.order[r.speakerIdx]
	}
	if r.Phase == PhaseCrossExam && r.crossIdx < len(r.crossQuestions) {
		curSpeaker = r.crossQuestions[r.crossIdx].TargetPlayerID
	}

	st := PublicState{
		RoomCode:          r.Code,
		Phase:             r.Phase,
		HostLine:          r.hostLine,
		Players:           players,
		MinPlayers:        MinPlayers,
		MaxPlayers:        MaxPlayers,
		StatementOrder:    append([]string{}, r.order...),
		CurrentSpeakerID:  curSpeaker,
		Statements:        append([]Statement{}, r.statements...),
		CrossQuestions:    append([]CrossQuestion{}, r.crossQuestions...),
		CurrentCrossIndex: r.crossIdx,
		VotesCast:         len(r.votes),
		VotesNeeded:       len(r.Players),
		Winner:            r.winner,
		RevealedScammer:   r.revealed,
		ScriptID:          r.Script.ID,
		ScriptTitle:       r.Script.Title,
		CallerHint:        r.Script.CallerHint,
		VoteLabel:         r.Script.voteLabel(),
		CitizenRoleLabel:  r.Script.citizenRoleLabel(),
		ScammerRoleLabel:  r.Script.scammerRoleLabel(),
		ValueTheme:        r.valueTheme,
		ValuePoints:       append([]string{}, r.valuePoints...),
		ValueClosing:      r.valueClosing,
		ValueSummary:      r.valueSummary,
		PhaseDeadlineUnix: deadline,
		CanStart:          r.Phase == PhaseLobby && len(r.Players) >= MinPlayers,
	}
	return st
}

func (r *Room) FindPlayerByName(name string) *Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.Players {
		if strings.EqualFold(p.Name, name) {
			cp := *p
			return &cp
		}
	}
	return nil
}

func (r *Room) PlayerNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.order))
	for _, id := range r.order {
		names = append(names, r.Players[id].Name)
	}
	sort.Strings(names)
	return names
}

func shuffle(ids []string) {
	for i := len(ids) - 1; i > 0; i-- {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(n.Int64())
		ids[i], ids[j] = ids[j], ids[i]
	}
}
