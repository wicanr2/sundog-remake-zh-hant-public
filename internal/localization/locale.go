package localization

// Locale 是玩家可選的全域文字語系。語系只屬於設定，不進入遊戲存檔。
type Locale string

const (
	English            Locale = "en"
	TraditionalChinese Locale = "zh-Hant"
	SimplifiedChinese  Locale = "zh-Hans"
	Japanese           Locale = "ja"
)

var supportedLocales = []Locale{TraditionalChinese, SimplifiedChinese, Japanese, English}

// Locales 回傳固定的選項循環順序。
func Locales() []Locale { return append([]Locale(nil), supportedLocales...) }

// KnownLocale 判斷設定值是否為正式支援的語系。
func KnownLocale(locale Locale) bool {
	for _, candidate := range supportedLocales {
		if candidate == locale {
			return true
		}
	}
	return false
}

// NormalizeLocale 讓舊設定與未知值安全回到繁體中文。
func NormalizeLocale(locale string) Locale {
	wish := Locale(locale)
	if KnownLocale(wish) {
		return wish
	}
	return TraditionalChinese
}

// LocaleNameKey 是語系名稱的穩定 catalog key。
func LocaleNameKey(locale Locale) string { return "ui.locale." + string(locale) }
