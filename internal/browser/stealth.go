package browser

import "log/slog"

func configureStealthSession(sendCmd sendCmdFunc, sessionID string, userAgent string) {
	// Set realistic user agent via CDP
	if userAgent != "" {
		_, err := sendCmd("Emulation.setUserAgentOverride", map[string]any{
			"userAgent":      userAgent,
			"acceptLanguage": "en-US,en;q=0.9",
		}, sessionID)
		if err != nil {
			slog.Debug("stealth: user agent override failed", "error", err)
		}
	}

	// Set extra headers to match a real browser
	_, err := sendCmd("Network.setExtraHTTPHeaders", map[string]any{
		"headers": map[string]string{
			"Accept-Language": "en-US,en;q=0.9",
		},
	}, sessionID)
	if err != nil {
		slog.Debug("stealth: extra headers failed", "error", err)
	}
}
