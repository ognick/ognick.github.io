package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ognick/zabkiss/internal/domain"
)

// fakeHA is a stub for haGateway that records calls and lets tests assert
// which actions reached HA.
type fakeHA struct {
	calls []string
}

func (f *fakeHA) GetDeviceInfos(_ context.Context, _ []string) ([]domain.Device, error) {
	return nil, nil
}

func (f *fakeHA) CallService(_ context.Context, entityID, service string, _ map[string]any) error {
	f.calls = append(f.calls, entityID+"|"+service)
	return nil
}

func TestExecuteActions_RejectsUnauthorizedTarget(t *testing.T) {
	ha := &fakeHA{}
	s := &SmartHomeService{ha: ha, log: nilLogger{}}

	allowed := map[string]map[string]struct{}{
		"light.kitchen": {"light.turn_on": {}},
	}
	actions := []domain.Action{
		{TargetID: "lock.front_door", Service: "lock.unlock"}, // NOT in policy
	}

	results := s.executeActions(context.Background(), actions, allowed)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error for unauthorized target")
	}
	if !errors.Is(results[0].Err, results[0].Err) { // presence check
		// nil error would have been caught above; just guard:
	}
	if len(ha.calls) != 0 {
		t.Errorf("ha.CallService should NOT be called for unauthorized action, got calls: %v", ha.calls)
	}
}

func TestExecuteActions_RejectsUnauthorizedService(t *testing.T) {
	ha := &fakeHA{}
	s := &SmartHomeService{ha: ha, log: nilLogger{}}

	allowed := map[string]map[string]struct{}{
		"light.kitchen": {"light.turn_on": {}},
	}
	actions := []domain.Action{
		{TargetID: "light.kitchen", Service: "lock.unlock"}, // entity is OK, service is not
	}

	results := s.executeActions(context.Background(), actions, allowed)

	if len(results) != 1 || results[0].Err == nil {
		t.Errorf("expected policy rejection, got: %+v", results)
	}
	if len(ha.calls) != 0 {
		t.Errorf("ha.CallService should NOT be called for unauthorized service, got: %v", ha.calls)
	}
}

func TestExecuteActions_AllowsValidAction(t *testing.T) {
	ha := &fakeHA{}
	s := &SmartHomeService{ha: ha, log: nilLogger{}}

	allowed := map[string]map[string]struct{}{
		"light.kitchen": {"light.turn_on": {}},
	}
	actions := []domain.Action{
		{TargetID: "light.kitchen", Service: "light.turn_on"},
	}

	results := s.executeActions(context.Background(), actions, allowed)

	if len(results) != 1 || results[0].Err != nil {
		t.Errorf("expected success, got: %+v", results)
	}
	if len(ha.calls) != 1 || ha.calls[0] != "light.kitchen|light.turn_on" {
		t.Errorf("ha.CallService should be called once with light.kitchen|light.turn_on, got: %v", ha.calls)
	}
}

type nilLogger struct{}

func (nilLogger) Info(_ string, _ ...any)   {}
func (nilLogger) Debug(_ string, _ ...any)  {}
func (nilLogger) Warn(_ string, _ ...any)   {}
func (nilLogger) Error(_ string, _ ...any)  {}
func (nilLogger) Infof(_ string, _ ...any)  {}
func (nilLogger) Errorf(_ string, _ ...any) {}

func TestBuildAllowedActionSet(t *testing.T) {
	devices := []domain.Device{
		{
			EntityID: "light.kitchen",
			Services: []domain.DeviceService{
				{Service: "light.turn_on"},
				{Service: "light.turn_off"},
			},
		},
		{
			EntityID: "media_player.tv",
			Services: []domain.DeviceService{
				{Service: "media_player.turn_on"},
				{Service: "media_player.volume_set"},
			},
		},
		{
			EntityID: "sensor.temp", // no services
		},
	}

	t.Run("youtube disabled", func(t *testing.T) {
		got := buildAllowedActionSet(devices, false)
		if _, ok := got["light.kitchen"]["light.turn_on"]; !ok {
			t.Error("light.turn_on should be allowed for light.kitchen")
		}
		if _, ok := got["media_player.tv"]["media_player.play_youtube"]; ok {
			t.Error("play_youtube should NOT be allowed when youtube disabled")
		}
		// sensor.temp has no services — it doesn't appear in the allow map.
		// This is fine: validateAction will reject any action targeting it.
	})

	t.Run("youtube enabled adds play_youtube to media_player", func(t *testing.T) {
		got := buildAllowedActionSet(devices, true)
		if _, ok := got["media_player.tv"]["media_player.play_youtube"]; !ok {
			t.Error("play_youtube should be allowed for media_player when youtube enabled")
		}
		if _, ok := got["light.kitchen"]["media_player.play_youtube"]; ok {
			t.Error("play_youtube should NOT be allowed for non-media_player entities")
		}
	})
}

func TestValidateAction(t *testing.T) {
	allowed := map[string]map[string]struct{}{
		"light.kitchen": {"light.turn_on": {}, "light.turn_off": {}},
		"lock.front":    {"lock.lock": {}, "lock.unlock": {}},
	}

	tests := []struct {
		name    string
		action  domain.Action
		wantErr bool
	}{
		{
			name:    "allowed action",
			action:  domain.Action{TargetID: "light.kitchen", Service: "light.turn_on"},
			wantErr: false,
		},
		{
			name:    "target not in policy",
			action:  domain.Action{TargetID: "lock.back_door", Service: "lock.unlock"},
			wantErr: true,
		},
		{
			name:    "service not allowed for entity",
			action:  domain.Action{TargetID: "light.kitchen", Service: "lock.unlock"},
			wantErr: true,
		},
		{
			name:    "empty target",
			action:  domain.Action{TargetID: "", Service: "light.turn_on"},
			wantErr: true,
		},
		{
			name:    "empty service",
			action:  domain.Action{TargetID: "light.kitchen", Service: ""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAction(tc.action, allowed)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
