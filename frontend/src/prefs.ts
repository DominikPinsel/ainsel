const REPORT_BTN_KEY = 'ainsel-report-btn'
const REPORT_SCREENSHOT_KEY = 'ainsel-report-screenshot'

function getBool(key: string): boolean {
  try { return localStorage.getItem(key) === 'true' } catch { return false }
}

export const PREF_CHANGE_EVENT = 'ainsel-pref-change'

function setBool(key: string, v: boolean): void {
  try { localStorage.setItem(key, String(v)) } catch { /* ignore */ }
  window.dispatchEvent(new CustomEvent(PREF_CHANGE_EVENT))
}

export const getReportBtnEnabled = () => getBool(REPORT_BTN_KEY)
export const setReportBtnEnabled = (v: boolean) => setBool(REPORT_BTN_KEY, v)
export const getReportScreenshotEnabled = () => getBool(REPORT_SCREENSHOT_KEY)
export const setReportScreenshotEnabled = (v: boolean) => setBool(REPORT_SCREENSHOT_KEY, v)
