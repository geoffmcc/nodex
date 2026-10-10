package config

import "testing"

func TestValidateMonitoringICMPHost(t *testing.T) {
	for _, address := range []string{"192.0.2.1", "2001:db8::1", "node.example.test"} {
		t.Run(address, func(t *testing.T) {
			cfg := &Config{Version: 2, Profiles: map[string]Profile{}, Monitoring: &Monitoring{Targets: map[string]MonitorTarget{
				"ping": {Type: "icmp", Address: address},
			}}}
			if err := Validate(cfg); err != nil {
				t.Fatalf("valid ICMP address rejected: %v", err)
			}
		})
	}
	for _, address := range []string{"https://node.example.test", "node.example.test:22", "bad host"} {
		t.Run("reject_"+address, func(t *testing.T) {
			cfg := &Config{Version: 2, Profiles: map[string]Profile{}, Monitoring: &Monitoring{Targets: map[string]MonitorTarget{
				"ping": {Type: "icmp", Address: address},
			}}}
			if err := Validate(cfg); err == nil {
				t.Fatal("invalid ICMP address was accepted")
			}
		})
	}
}

func TestValidateMonitoringServiceRequiresEnrolledHostAndUnit(t *testing.T) {
	base := func(target MonitorTarget) *Config {
		return &Config{
			Version: 2, Profiles: map[string]Profile{},
			Inventory: &Inventory{Hosts: map[string]InventoryHost{
				"fileserver": {Address: "192.0.2.4", Role: RoleGeneric, SSHUser: "automation"},
			}},
			Monitoring: &Monitoring{Targets: map[string]MonitorTarget{"samba": target}},
		}
	}
	if err := Validate(base(MonitorTarget{Type: "service", Address: "fileserver", Service: "smbd.service"})); err != nil {
		t.Fatalf("valid enrolled service target rejected: %v", err)
	}
	for _, target := range []MonitorTarget{
		{Type: "service", Address: "unknown", Service: "smbd.service"},
		{Type: "service", Address: "fileserver", Service: "smbd;touch-pwned.service"},
		{Type: "service", Address: "fileserver"},
	} {
		if err := Validate(base(target)); err == nil {
			t.Errorf("invalid service target was accepted: %+v", target)
		}
	}
	noInventory := &Config{Version: 2, Profiles: map[string]Profile{}, Monitoring: &Monitoring{Targets: map[string]MonitorTarget{
		"samba": {Type: "service", Address: "fileserver", Service: "smbd.service"},
	}}}
	if err := Validate(noInventory); err == nil {
		t.Fatal("service monitoring without inventory was accepted")
	}
}
