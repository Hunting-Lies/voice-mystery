package tts

import "testing"

func TestEnsureSpeakerHostAndStable(t *testing.T) {
	s := &Service{
		assign:   make(map[string]Voice),
		cache:    make(map[string]cachedAudio),
		cacheMax: 8,
	}
	host := s.EnsureSpeaker(HostKey)
	if host.ID != HostVoice.ID {
		t.Fatalf("host want %s got %s", HostVoice.ID, host.ID)
	}
	narr := s.EnsureSpeaker(NarratorKey)
	if narr.ID != NarratorVoice.ID {
		t.Fatalf("narrator want %s got %s", NarratorVoice.ID, narr.ID)
	}
	a := s.EnsureSpeaker("player-alpha")
	b := s.EnsureSpeaker("player-beta")
	if a.ID == "" || b.ID == "" {
		t.Fatal("players need voices")
	}
	if s.EnsureSpeaker("player-alpha").ID != a.ID {
		t.Fatal("assignment must be stable for same key")
	}
	if a.ID == b.ID {
		t.Log("warning: two keys hashed to same voice (possible with pool of 4)")
	}
}

func TestCatalogHasFourVoices(t *testing.T) {
	if len(Catalog()) != 4 {
		t.Fatalf("want 4 voices")
	}
}

func TestVoiceSlotIndexSpread(t *testing.T) {
	seen := map[int]bool{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		seen[voiceSlotIndex(k, len(PlayerPool))] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected hash spread across pool, got %d slots", len(seen))
	}
}
