package main

// StealthFlags returns Chrome launch flags for anti-detection.
func StealthFlags() []string {
	return []string{
		"--disable-blink-features=AutomationControlled",
		"--disable-features=IsolateOrigins,site-per-process,TranslateUI,OptimizationHints,MediaRouter,DialMediaRouteProvider",
		"--disable-infobars",
		"--disable-dev-shm-usage",
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-ipc-flooding-protection",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-hang-monitor",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-sync",
		"--disable-component-update",
		"--disable-domain-reliability",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--ignore-certificate-errors",
		"--force-color-profile=srgb",
		"--metrics-recording-only",
		"--password-store=basic",
		"--use-mock-keychain",
		"--mute-audio",
	}
}

