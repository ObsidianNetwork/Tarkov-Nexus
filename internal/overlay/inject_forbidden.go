package overlay

// injectScriptForbidden is the CI + livenet contract for behaviours the
// inject script must not reintroduce. Kept next to the script so both test
// files (plain and -tags livenet) share one list.
var injectScriptForbidden = []struct{ needle, why string }{
	{"CookieConsent=true", "answers tarkov.dev's cookie consent on the user's behalf"},
	{".CookieConsent { display: none", "suppresses the consent banner so the user cannot answer it"},
	{".id-wrapper { display: none", "hides tarkov.dev's own branding and session widget"},
	{"window.L.Map = function", "replaces Leaflet's Map constructor; use L.Map.addInitHook"},
	{"savedMapSettings", "overrides the user's own map settings on tarkov.dev"},
	{"cb.click()", "programmatically unchecks tarkov.dev's map filters"},
	{"seen || mapLooksShort()", "fills before React can mount the cookie banner"},
}
