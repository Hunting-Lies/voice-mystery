package game

import (
	"testing"
	"time"
)

func TestFullMatchWithBots(t *testing.T) {
	r := NewRoom("TEST", DefaultScriptID())
	if _, err := r.AddPlayer("人类", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FillBots(3); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().Phase != PhaseBriefing {
		t.Fatalf("want briefing")
	}
	priv, err := r.PrivateView(r.order[0])
	if err != nil || priv.Ability == AbilityNone {
		t.Fatalf("expected ability on start, priv=%+v err=%v", priv, err)
	}
	if err := r.AdvanceFromBriefing(); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 120; i++ {
		r.mu.Lock()
		r.nextBotActAt = time.Time{}
		r.mu.Unlock()
		r.AutoActBots()
		snap := r.Snapshot()
		if snap.Phase == PhaseEnd {
			break
		}
		var humanID string
		for id, p := range r.Players {
			if !p.IsBot {
				humanID = id
				break
			}
		}
		switch snap.Phase {
		case PhaseStatement, PhaseCrossExam:
			if snap.CurrentSpeakerID == humanID {
				if snap.Phase == PhaseStatement {
					_ = r.SubmitStatement(humanID, "我认为要求转安全账户是假的。")
				} else {
					_ = r.SubmitCrossAnswer(humanID, "因为正规银行不会这么做。")
				}
			}
		case PhaseEvidence:
			if len(snap.PublicClues) == 0 {
				t.Fatal("evidence should expose clues")
			}
			_ = r.AdvanceFromEvidence()
		case PhaseMark:
			for _, o := range snap.Players {
				if o.ID != humanID {
					_ = r.MarkSuspect(humanID, o.ID)
					break
				}
			}
		case PhaseVote:
			for _, o := range snap.Players {
				if o.ID != humanID {
					_ = r.CastVote(humanID, o.ID)
					break
				}
			}
		}
	}
	snap := r.Snapshot()
	if snap.Phase != PhaseEnd {
		t.Fatalf("game did not end, phase=%s host=%s", snap.Phase, snap.HostLine)
	}
	if snap.RevealedScammer == "" {
		t.Fatal("scammer not revealed")
	}
	if snap.Winner != "citizens" && snap.Winner != "scammer" {
		t.Fatalf("bad winner %s", snap.Winner)
	}
	if snap.ValueSummary == "" || snap.ValueTheme == "" || len(snap.ValuePoints) == 0 {
		t.Fatalf("expected value recap, theme=%q points=%d summary=%q", snap.ValueTheme, len(snap.ValuePoints), snap.ValueSummary)
	}
	if len(snap.Scores) == 0 {
		t.Fatal("expected scores")
	}
}
