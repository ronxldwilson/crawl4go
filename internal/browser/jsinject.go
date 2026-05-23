package browser

import "log/slog"

func injectBrowserScripts(sendCmd sendCmdFunc, sessionID string) {
	scripts := []struct {
		name    string
		js      string
		isAsync bool
	}{
		{"navigator_override", jsNavigatorOverride, false},
		{"remove_consent_popups", jsRemoveConsentPopups, true},
		{"remove_overlays", jsRemoveOverlays, true},
		{"flatten_shadow_dom", jsFlattenShadowDOM, false},
		{"update_image_dimensions", jsUpdateImageDimensions, false},
	}

	for _, s := range scripts {
		expr := s.js
		if s.isAsync {
			expr = "(" + expr + ")()"
		}
		_, err := sendCmd("Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
			"awaitPromise":  s.isAsync,
		}, sessionID)
		if err != nil {
			slog.Debug("js inject failed", "script", s.name, "error", err)
		}
	}
}

const jsNavigatorOverride = `
const originalQuery = window.navigator.permissions.query;
window.navigator.permissions.query = (parameters) =>
    parameters.name === "notifications"
        ? Promise.resolve({ state: Notification.permission })
        : originalQuery(parameters);
Object.defineProperty(navigator, "webdriver", { get: () => undefined });
window.navigator.chrome = { runtime: {} };
Object.defineProperty(navigator, "plugins", { get: () => [1, 2, 3, 4, 5] });
Object.defineProperty(navigator, "languages", { get: () => ["en-US", "en"] });
Object.defineProperty(document, "hidden", { get: () => false });
Object.defineProperty(document, "visibilityState", { get: () => "visible" });
`

const jsRemoveConsentPopups = `async () => {
    const isVisible = (elem) => {
        if (!elem) return false;
        const style = window.getComputedStyle(elem);
        return style.display !== "none" && style.visibility !== "hidden" && style.opacity !== "0";
    };
    const cmpAcceptSelectors = [
        '#onetrust-accept-btn-handler','#accept-recommended-btn-handler',
        '#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll','#CybotCookiebotDialogBodyButtonAccept',
        '#didomi-notice-agree-button','.didomi-button-highlight',
        '.qc-cmp2-summary-buttons button[mode="primary"]',
        '.sp_choice_type_11','.sp_choice_type_ACCEPT_ALL',
        '.fc-button.fc-cta-consent.fc-primary-button','.fc-cta-consent',
        '#truste-consent-button','.cmpboxbtnyes','#cmpwelcomebtnyes',
        '.osano-cm-accept-all','.osano-cm-accept',
        '#iubenda-cs-accept-btn','.iubenda-cs-accept-btn',
        '.cmplz-btn.cmplz-accept','.fides-accept-all-button',
        '.cky-btn-accept','[data-cky-tag="accept-button"]',
        '.klaro .cm-btn-accept-all','.klaro .cm-btn-success',
        '[data-tid="banner-accept"]','button[data-cookiefirst-action="accept"]',
        '#cookiescript_accept','a[data-cookie-accept-all]','.brlbs-btn-accept-all',
        '#ccc-recommended-settings','#ccc-notify-accept',
        '.coi-banner__accept','#_evidon-accept-button',
        'button#axeptio_btn_acceptAll','#hs-eu-confirmation-button',
        '.moove-gdpr-infobar-allow-all','.cc-nb-okagree',
        '#tarteaucitronPersonalize2','.tarteaucitronAllow',
        '.ch2-allow-all-btn','#cn-accept-cookie',
        '.eu-cookie-compliance-banner .agree-button',
        '[data-cli_action="accept"]','#shopify-pc__banner__btn-accept',
        '[data-hook="ccsu-banner-accept"]','[fs-consent-element="allow"]',
        '#pandectes-banner .cc-allow','#cl-consent [data-role="b_agree"]',
        '.snigel-cmp-framework #accept-choices','.cassie-accept-all',
        '#acceptAllMain','#pt-accept-all','#unic-agree','#ez-accept-all',
        '#cf_consent-buttons__accept-all','#s-all-bn',
        '.cm__btn[data-role="all"]','#lm-accept-all','#catapultCookie',
        '.isense-cc-allow','#adopt-accept-all-button',
        'button#acceptAllCookieButton','#kc-acceptAndHide',
        '#ct-ultimate-gdpr-cookie-accept','.rcb-banner-cta-accept-all',
        '#bnp_btn_accept'
    ];
    const genericAcceptSelectors = [
        'button[id*="accept" i]','button[class*="accept-all" i]',
        'button[class*="acceptAll" i]','a[id*="accept" i]',
        'button[id*="agree" i]','button[class*="agree" i]',
        'button[class*="allow-all" i]','button[class*="allowAll" i]',
        'button[data-action="accept"]','button[data-action="accept-all"]',
        'button[data-gdpr="accept"]','button[data-consent="accept"]'
    ];
    const clickButton = async (selectors) => {
        for (const selector of selectors) {
            try {
                const btn = document.querySelector(selector);
                if (btn && isVisible(btn)) { btn.click(); await new Promise(r=>setTimeout(r,300)); return true; }
            } catch(e) {}
        }
        return false;
    };
    let accepted = await clickButton(cmpAcceptSelectors);
    if (!accepted) accepted = await clickButton(genericAcceptSelectors);
    if (!accepted) {
        const acceptPatterns = [/^accept\s*(all)?(\s*cookies)?$/i,/^allow\s*(all)?(\s*cookies)?$/i,/^i\s*agree$/i,/^got\s*it[!]?$/i,/^consent$/i];
        const candidates = document.querySelectorAll('button, a[role="button"], [role="button"], input[type="submit"]');
        for (const btn of candidates) {
            const text = (btn.textContent||btn.value||'').trim();
            if (text.length>0 && text.length<40) {
                for (const p of acceptPatterns) { if (p.test(text) && isVisible(btn)) { btn.click(); accepted=true; break; } }
                if (accepted) break;
            }
        }
    }
    if (!accepted) {
        const shadowRoots = [{id:'usercentrics-root',btn:'button[data-testid="uc-accept-all-button"]'},{cls:'axeptio_mount',btn:'button#axeptio_btn_acceptAll'}];
        for (const cfg of shadowRoots) {
            try { const host=cfg.id?document.getElementById(cfg.id):document.querySelector('.'+cfg.cls); if(host&&host.shadowRoot){const btn=host.shadowRoot.querySelector(cfg.btn);if(btn){btn.click();accepted=true;break;}} } catch(e){}
        }
    }
    await new Promise(r=>setTimeout(r,500));
    const containers = ['#onetrust-consent-sdk','#CybotCookiebotDialog','#truste-consent-track','#didomi-host','#usercentrics-root','div[id^="sp_message_container"]','.fc-consent-root','.klaro','.osano-cm-window','#iubenda-cs-banner','.cmplz-cookiebanner','.cky-consent-container','.cmpbox','.fides-overlay','#termly-code-snippet-support','#cookiefirst-root','#cookiescript_injected','#BorlabsCookieBox','#ccc','#cookie-information-template-wrapper','#_evidon_banner','.axeptio_widget','#hs-eu-cookie-confirmation','#lanyard_root','#tarteaucitronRoot','.ch2-container','#moove_gdpr_cookie_info_bar','.termsfeed-com---nb','#cookie-notice','#cookie-law-info-bar','.eu-cookie-compliance-banner','#gdpr-cookie-consent-bar','#shopify-pc__banner','[class*="cookie-consent" i]','[class*="cookie-banner" i]','[class*="consent-banner" i]','[class*="gdpr-banner" i]','[class*="cookie-notice" i]','[class*="cookie-popup" i]','.cc-banner','.cc-window','.rcb-banner','#bnp_container'];
    for (const sel of containers) { try { document.querySelectorAll(sel).forEach(el=>el.remove()); } catch(e){} }
    const iframeSels = ['iframe[id^="sp_message_iframe"]','iframe#fast-cmp-iframe','iframe[src*="consent" i]','iframe[title*="consent" i]','iframe[title*="cookie" i]','iframe[name="__tcfapiLocator"]'];
    for (const sel of iframeSels) { try { document.querySelectorAll(sel).forEach(el=>el.remove()); } catch(e){} }
    document.body.style.overflow='';document.body.style.overflowY='';document.body.style.position='';
    document.body.style.marginRight='';document.body.style.paddingRight='';
    document.documentElement.style.overflow='';document.documentElement.style.overflowY='';
    const cmpClasses=['ot-overflow-hidden','sp_message_open','didomi-popup-open','cmpbox-show','cmplz-blocked','qc-cmp2-no-scroll','osano-cm-show','cky-modal-open','fides-overlay-modal-open','cc-no-scroll','fc-consent-root-open'];
    for (const cls of cmpClasses) { document.body.classList.remove(cls); document.documentElement.classList.remove(cls); }
}`

const jsRemoveOverlays = `async () => {
    const isVisible = (elem) => {
        const style = window.getComputedStyle(elem);
        return style.display !== "none" && style.visibility !== "hidden" && style.opacity !== "0";
    };
    const closeSelectors = ['button[class*="close" i]','button[class*="dismiss" i]','button[aria-label*="close" i]','button[title*="close" i]','a[class*="close" i]','span[class*="close" i]'];
    for (const selector of closeSelectors) {
        const buttons = document.querySelectorAll(selector);
        for (const btn of buttons) { if (isVisible(btn)) { try { btn.click(); await new Promise(r=>setTimeout(r,100)); } catch(e){} } }
    }
    const allElements = document.querySelectorAll("*");
    for (const elem of allElements) {
        const style = window.getComputedStyle(elem);
        const zIndex = parseInt(style.zIndex);
        if (isVisible(elem) && (zIndex>999||style.position==="fixed"||style.position==="absolute") && (elem.offsetWidth>window.innerWidth*0.5||elem.offsetHeight>window.innerHeight*0.5||style.backgroundColor.includes("rgba")||parseFloat(style.opacity)<1)) { elem.remove(); }
    }
    const overlaySelectors = ['[class*="popup" i]','[class*="modal" i]','[class*="overlay" i]','[class*="dialog" i]','[role="dialog"]','[role="alertdialog"]','[class*="newsletter" i]','[class*="subscribe" i]'];
    for (const sel of overlaySelectors) { document.querySelectorAll(sel).forEach(el=>{ if(isVisible(el)) el.remove(); }); }
    document.body.style.marginRight="0px";document.body.style.paddingRight="0px";document.body.style.overflow="auto";
}`

const jsFlattenShadowDOM = `(() => {
    const VOID = new Set(['area','base','br','col','embed','hr','img','input','link','meta','param','source','track','wbr']);
    const serialize = (node) => {
        if (node.nodeType===Node.TEXT_NODE) return node.textContent;
        if (node.nodeType===Node.COMMENT_NODE) return '';
        if (node.nodeType!==Node.ELEMENT_NODE) return '';
        const tag=node.tagName.toLowerCase();
        const attrs=serializeAttrs(node);
        let inner='';
        if (node.shadowRoot) { inner=serializeShadowRoot(node); }
        else { for (const child of node.childNodes) inner+=serialize(child); }
        if (VOID.has(tag)) return '<'+tag+attrs+'>';
        return '<'+tag+attrs+'>'+inner+'</'+tag+'>';
    };
    const serializeShadowRoot = (host) => {
        let result='';
        for (const child of host.shadowRoot.childNodes) result+=serializeShadowChild(child,host);
        return result;
    };
    const serializeShadowChild = (node, host) => {
        if (node.nodeType===Node.TEXT_NODE) return node.textContent;
        if (node.nodeType===Node.COMMENT_NODE) return '';
        if (node.nodeType!==Node.ELEMENT_NODE) return '';
        const tag=node.tagName.toLowerCase();
        if (tag==='style') return '';
        if (tag==='slot') {
            const assigned=node.assignedNodes({flatten:true});
            if (assigned.length>0) { let out=''; for(const a of assigned) out+=serialize(a); return out; }
            let fallback=''; for(const child of node.childNodes) fallback+=serializeShadowChild(child,host); return fallback;
        }
        const attrs=serializeAttrs(node);
        let inner='';
        if (node.shadowRoot) { inner=serializeShadowRoot(node); }
        else { for(const child of node.childNodes) inner+=serializeShadowChild(child,host); }
        if (VOID.has(tag)) return '<'+tag+attrs+'>';
        return '<'+tag+attrs+'>'+inner+'</'+tag+'>';
    };
    const serializeAttrs = (node) => {
        let s='';
        for (const a of node.attributes||[]) s+=' '+a.name+'="'+a.value.replace(/&/g,'&amp;').replace(/"/g,'&quot;')+'"';
        return s;
    };
    return serialize(document.documentElement);
})()`

const jsUpdateImageDimensions = `(() => {
    return new Promise((resolve) => {
        const filterImage = (img) => {
            if (img.width<100&&img.height<100) return false;
            const rect=img.getBoundingClientRect();
            if (rect.width===0||rect.height===0) return false;
            if (img.classList.contains("icon")||img.classList.contains("thumbnail")) return false;
            if (img.src.includes("placeholder")||img.src.includes("icon")) return false;
            return true;
        };
        const images=Array.from(document.querySelectorAll("img")).filter(filterImage);
        let left=images.length;
        if (left===0) { resolve(); return; }
        const check=(img)=>{ if(img.complete&&img.naturalWidth!==0){img.setAttribute("width",img.naturalWidth);img.setAttribute("height",img.naturalHeight);left--;if(left===0)resolve();} };
        images.forEach(img=>{ check(img); if(!img.complete){img.onload=()=>check(img);img.onerror=()=>{left--;if(left===0)resolve();};} });
        resolve();
    });
})()`
