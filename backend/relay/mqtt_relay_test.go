package relay

import (
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestBuildModbusRelayCommandRelay1(t *testing.T) {
	tests := []struct {
		name string
		on   bool
		want string
	}{
		{name: "on", on: true, want: "01050000ff008c3a"},
		{name: "off", on: false, want: "010500000000cdca"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildModbusRelayCommand(1, tt.on)
			if err != nil {
				t.Fatalf("BuildModbusRelayCommand returned error: %v", err)
			}
			if hex.EncodeToString(got) != tt.want {
				t.Fatalf("payload mismatch: got %s want %s", hex.EncodeToString(got), tt.want)
			}
		})
	}
}

func TestBuildModbusRelayCommandRejectsInvalidRelay(t *testing.T) {
	for _, relayNumber := range []uint8{0, 9} {
		if _, err := BuildModbusRelayCommand(relayNumber, true); err == nil {
			t.Fatalf("expected error for relay %d", relayNumber)
		}
	}
}

func TestBuildModbusReadRelayStatusCommand(t *testing.T) {
	got := BuildModbusReadRelayStatusCommand()
	want := "0101000000083dcc"
	if hex.EncodeToString(got) != want {
		t.Fatalf("payload mismatch: got %s want %s", hex.EncodeToString(got), want)
	}
}

func TestDisableRuntimeConfigOverridesRelayEnv(t *testing.T) {
	t.Setenv("SITE_ENV_PATH", filepath.Join(t.TempDir(), "site.env"))
	t.Setenv("MQTT_RELAY_BROKER", "tcp://example.test:1883")
	t.Setenv("MQTT_RELAY_CMD_TOPIC", "snooker/test/relay/cmd")
	t.Setenv("MQTT_RELAY_REQUIRED", "true")

	if !Enabled() {
		t.Fatalf("expected relay to be enabled before runtime disable")
	}
	if !Required() {
		t.Fatalf("expected relay to be required before runtime disable")
	}

	if err := DisableRuntimeConfig(); err != nil {
		t.Fatalf("DisableRuntimeConfig returned error: %v", err)
	}

	if !Disabled() {
		t.Fatalf("expected relay to be disabled")
	}
	if Enabled() {
		t.Fatalf("expected disabled relay to not be enabled")
	}
	if Required() {
		t.Fatalf("expected disabled relay to not be required")
	}
}
