package probe

import (
	"strings"
	"testing"
)

func TestWpaSupplicantConf(t *testing.T) {
	psk := wpaSupplicantConf(WlanActiveOpts{SSID: "lab", Security: "psk", Password: "secret"}, "")
	for _, want := range []string{`ssid="lab"`, `psk="secret"`, "WPA-PSK SAE", "mac_addr=0", "preassoc_mac_addr=0"} {
		if !strings.Contains(psk, want) {
			t.Errorf("psk conf missing %q:\n%s", want, psk)
		}
	}

	eap := wpaSupplicantConf(WlanActiveOpts{SSID: "corp", Security: "eap-peap", Identity: "user", Password: "pw"}, "/tmp/ca.pem")
	for _, want := range []string{"eap=PEAP", `identity="user"`, `ca_cert="/tmp/ca.pem"`, `phase2="auth=MSCHAPV2"`} {
		if !strings.Contains(eap, want) {
			t.Errorf("eap conf missing %q:\n%s", want, eap)
		}
	}
	// insecure: no ca_cert line
	insecure := wpaSupplicantConf(WlanActiveOpts{SSID: "corp", Security: "eap-peap", Identity: "u", Password: "p", InsecureSkipVerify: true}, "")
	if strings.Contains(insecure, "ca_cert") {
		t.Error("insecure conf must not contain ca_cert")
	}

	open := wpaSupplicantConf(WlanActiveOpts{SSID: "cafe", Security: "open"}, "")
	if !strings.Contains(open, "key_mgmt=NONE") {
		t.Errorf("open conf missing key_mgmt=NONE:\n%s", open)
	}

	// injection: quotes/backslashes in SSID/password must stay escaped
	inj := wpaSupplicantConf(WlanActiveOpts{SSID: `x" }network={`, Security: "psk", Password: `p"w\`}, "")
	if !strings.Contains(inj, `ssid="x\" }network={"`) || !strings.Contains(inj, `psk="p\"w\\"`) {
		t.Errorf("escaping failed:\n%s", inj)
	}
}

func TestParseWpaEvent(t *testing.T) {
	tests := []struct {
		line  string
		event wpaEvent
		bssid string
	}{
		{"wlan1: Associated with dc:ce:c1:2c:b2:cf", wpaEventAssociated, "dc:ce:c1:2c:b2:cf"},
		{"wlan1: CTRL-EVENT-CONNECTED - Connection to dc:ce:c1:2c:b2:cf completed [id=0]", wpaEventConnected, ""},
		{"wlan1: CTRL-EVENT-ASSOC-REJECT bssid=aa:bb:cc:dd:ee:ff status_code=17", wpaEventAssocFail, ""},
		{"wlan1: CTRL-EVENT-SSID-TEMP-DISABLED id=0 ssid=\"lab\" auth_failures=1 reason=WRONG_KEY", wpaEventAuthFail, ""},
		{"wlan1: CTRL-EVENT-EAP-FAILURE EAP authentication failed", wpaEventAuthFail, ""},
		{"wlan1: SME: Trying to authenticate with dc:ce:c1:2c:b2:cf", wpaEventTrying, ""},
		{"wlan1: Trying to associate with dc:ce:c1:2c:b2:cf (SSID='lab' freq=5500 MHz)", wpaEventTrying, ""},
		{"wlan1: Scan completed in 8.2 seconds", wpaEventNone, ""},
	}
	for _, tc := range tests {
		ev, bssid := parseWpaEvent(tc.line)
		if ev != tc.event || bssid != tc.bssid {
			t.Errorf("%q: got (%v, %q), want (%v, %q)", tc.line, ev, bssid, tc.event, tc.bssid)
		}
	}
}
