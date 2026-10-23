package game

import "time"

type Phase string

const (
	PhaseLobby     Phase = "lobby"
	PhaseBriefing  Phase = "briefing"
	PhaseStatement Phase = "statement"
	PhaseCrossExam Phase = "cross_exam"
	PhaseVote      Phase = "vote"
	PhaseReveal    Phase = "reveal"
	PhaseEnd       Phase = "end"
)

type Role string

const (
	RoleCitizen Role = "citizen"
	RoleScammer Role = "scammer"
)

type Player struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IsBot           bool   `json:"isBot"`
	Role            Role   `json:"role,omitempty"`
	Alive           bool   `json:"alive"`
	Ready           bool   `json:"ready"`
	JoinedAt        time.Time `json:"joinedAt"`
	characterName   string
	characterLabel  string
	characterCard   string
}

type Statement struct {
	PlayerID string    `json:"playerId"`
	Name     string    `json:"name"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
}

type Vote struct {
	FromPlayerID string `json:"fromPlayerId"`
	ToPlayerID   string `json:"toPlayerId"`
}

type CrossQuestion struct {
	TargetPlayerID string `json:"targetPlayerId"`
	Question       string `json:"question"`
	Answer         string `json:"answer,omitempty"`
}

type PublicState struct {
	RoomCode          string          `json:"roomCode"`
	Phase             Phase           `json:"phase"`
	HostLine          string          `json:"hostLine"`
	Players           []PublicPlayer  `json:"players"`
	MinPlayers        int             `json:"minPlayers"`
	MaxPlayers        int             `json:"maxPlayers"`
	StatementOrder    []string        `json:"statementOrder,omitempty"`
	CurrentSpeakerID  string          `json:"currentSpeakerId,omitempty"`
	Statements        []Statement     `json:"statements,omitempty"`
	CrossQuestions    []CrossQuestion `json:"crossQuestions,omitempty"`
	CurrentCrossIndex int             `json:"currentCrossIndex"`
	VotesCast         int             `json:"votesCast"`
	VotesNeeded       int             `json:"votesNeeded"`
	Winner            string          `json:"winner,omitempty"` // citizens | scammer
	RevealedScammer   string          `json:"revealedScammer,omitempty"`
	ScriptID          string          `json:"scriptId,omitempty"`
	ScriptTitle       string          `json:"scriptTitle,omitempty"`
	CallerHint        string          `json:"callerHint,omitempty"`
	VoteLabel         string          `json:"voteLabel,omitempty"`
	CitizenRoleLabel  string          `json:"citizenRoleLabel,omitempty"`
	ScammerRoleLabel  string          `json:"scammerRoleLabel,omitempty"`
	ValueTheme        string          `json:"valueTheme,omitempty"`   // 结算主题，如反诈宣传
	ValuePoints       []string        `json:"valuePoints,omitempty"`  // 关键防范点
	ValueClosing      string          `json:"valueClosing,omitempty"` // 收束寄语
	ValueSummary      string          `json:"valueSummary,omitempty"` // 完整价值复盘文案（可朗读）
	PhaseDeadlineUnix int64           `json:"phaseDeadlineUnix,omitempty"`
	CanStart          bool            `json:"canStart"`
}

type PublicPlayer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IsBot bool   `json:"isBot"`
	Alive bool   `json:"alive"`
	Ready bool   `json:"ready"`
}

type PrivateView struct {
	PlayerID       string `json:"playerId"`
	Role           Role   `json:"role"`
	SecretCard     string `json:"secretCard"`
	CharacterName  string `json:"characterName,omitempty"`
	CharacterLabel string `json:"characterLabel,omitempty"`
}
