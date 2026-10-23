package tts

import "testing"

func TestEnsureSpeakerRotation(t *testing.T) {
	s := &Service{
		assign:   make(map[string]Voice),
		cache:    make(map[string]cachedAudio),
		cacheMax: 8,
	}
	host := s.EnsureSpeaker(HostKey)
	if host.ID != HostVoice.ID {
		t.Fatalf("host want %s got %s", HostVoice.ID, host.ID)
	}
	a := s.EnsureSpeaker("p1")
	b := s.EnsureSpeaker("p2")
	c := s.EnsureSpeaker("p3")
	d := s.EnsureSpeaker("p4")
	if a.ID != VoiceWarmMale.ID || b.ID != VoiceSingerMale.ID || c.ID != VoiceSmartMale.ID {
		t.Fatalf("player pool order wrong: %s %s %s", a.Name, b.Name, c.Name)
	}
	if d.ID != VoiceWarmMale.ID {
		t.Fatalf("4th player should wrap to warm male, got %s", d.Name)
	}
	if s.EnsureSpeaker("p1").ID != a.ID {
		t.Fatal("assignment must be stable")
	}
}

func TestCatalogHasFourVoices(t *testing.T) {
	if len(Catalog()) != 4 {
		t.Fatalf("want 4 voices")
	}
}
