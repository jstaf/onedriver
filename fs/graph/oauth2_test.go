package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthCodeFormat(t *testing.T) {
	// faked personal auth code
	code, err := parseAuthCode("https://login.live.com/oauth20_desktop.srf?code=M.R3_BAY.abcd526-817f-*!$d8e9-590c-1227b45c7be2&lc=4105")
	assert.NoError(t, err)
	assert.Equal(t, "M.R3_BAY.abcd526-817f-*!$d8e9-590c-1227b45c7be2", code,
		"Personal auth code did not match expected result.")

	// faked business auth code
	code, err = parseAuthCode("https://login.live.com/oauth20_desktop.srf?code=0.BAAA-AeXRPDP_sEe7XktwiDweriowjeirjcDQQvKtFoKktMINkhdEzAAA.AQABAAAAARWERAB2UyzwtQEKR7-rWbgdcBZICdWKCJnfnPJurxUN_QbF3GS6OQqQiK987AbLAv2QykQMIGAz4XCvkO8kB3XC8RYV10qmnmHcMUgo7u5UubpgpR3OW3TVlMSZ-3vxjkcEHlsnVoBqfUFdcj8fYR_mP6w0xkB8MmLG3i5F-JtcaLKfQu13941lsdjkfdh0acjHBGJHVzpBbuiVfzN6vMygFiS2xAQGF668M_l69dXRmG1tq3ZwU6J0-FWYNfK_Ro4YS2m38bcNmZQ8iEolV78t34HKxCYZnl4iqeYF7b7hkTM7ZIcsDBoeZvW1Cu6dIQ7xC4NZGILltOXY5V6A-kcLCZaYuSFW_R8dEM-cqGr_5Gv1GhgfqyXd-2XYNvGda9ok20JrYEmMiezfnyRV-vc7rdtlLOVI_ubzhrjezAvtAApPEj3dJdcmW_0qns_R27pVDlU1xkDagQAquhrftE_sZHbRGvnAsdfaoim1SjcX7QosTELyoWeAczip4MPYqmJ1uVjpWb533vA5WZMyWatiDuNYhnj48SsfEP2zaUQFU55Aj90hEOhOPl77AOu0-zNfAGXeWAQhTPO2rZ0ZgHottFwLoq8aA52sTW-hf7kB0chFUaUvLkxKr1L-Zi7vyCBoArlciFV3zyMxiQ8kjR3vxfwlerjowicmcgqJD-8lxioiwerwlbrlQWyAA&session_state=3fa7b212-7dbb-44e6-bddd-812fwieojw914341")
	assert.NoError(t, err)
	if code != "0.BAAA-AeXRPDP_sEe7XktwiDweriowjeirjcDQQvKtFoKktMINkhdEzAAA.AQABAAAAARWERAB2UyzwtQEKR7-rWbgdcBZICdWKCJnfnPJurxUN_QbF3GS6OQqQiK987AbLAv2QykQMIGAz4XCvkO8kB3XC8RYV10qmnmHcMUgo7u5UubpgpR3OW3TVlMSZ-3vxjkcEHlsnVoBqfUFdcj8fYR_mP6w0xkB8MmLG3i5F-JtcaLKfQu13941lsdjkfdh0acjHBGJHVzpBbuiVfzN6vMygFiS2xAQGF668M_l69dXRmG1tq3ZwU6J0-FWYNfK_Ro4YS2m38bcNmZQ8iEolV78t34HKxCYZnl4iqeYF7b7hkTM7ZIcsDBoeZvW1Cu6dIQ7xC4NZGILltOXY5V6A-kcLCZaYuSFW_R8dEM-cqGr_5Gv1GhgfqyXd-2XYNvGda9ok20JrYEmMiezfnyRV-vc7rdtlLOVI_ubzhrjezAvtAApPEj3dJdcmW_0qns_R27pVDlU1xkDagQAquhrftE_sZHbRGvnAsdfaoim1SjcX7QosTELyoWeAczip4MPYqmJ1uVjpWb533vA5WZMyWatiDuNYhnj48SsfEP2zaUQFU55Aj90hEOhOPl77AOu0-zNfAGXeWAQhTPO2rZ0ZgHottFwLoq8aA52sTW-hf7kB0chFUaUvLkxKr1L-Zi7vyCBoArlciFV3zyMxiQ8kjR3vxfwlerjowicmcgqJD-8lxioiwerwlbrlQWyAA" {
		t.Error("Business auth code did not match expected result.")
	}
}

func TestAuthFromfile(t *testing.T) {
	t.Parallel()
	require.FileExists(t, ".auth_tokens.json")

	var auth Auth
	auth.FromFile(".auth_tokens.json")
	assert.NotEqual(t, "", auth.AccessToken, "Could not load auth tokens from '.auth_tokens.json'!")
}

func TestAuthRefresh(t *testing.T) {
	t.Parallel()
	require.FileExists(t, ".auth_tokens.json")

	var auth Auth
	auth.FromFile(".auth_tokens.json")
	auth.ExpiresAt = 0 // force an auth refresh
	auth.Refresh()
	if auth.ExpiresAt <= time.Now().Unix() {
		t.Fatal("Auth could not be refreshed successfully!")
	}
}

func TestAuthConfigMerge(t *testing.T) {
	t.Parallel()

	testConfig := AuthConfig{RedirectURL: "test"}
	assert.NoError(t, testConfig.applyDefaults())
	assert.Equal(t, "test", testConfig.RedirectURL)
	assert.Equal(t, authClientID, testConfig.ClientID)
}
