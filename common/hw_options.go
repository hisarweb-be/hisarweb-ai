package common

// HwLumiaRetryPauseMs is the fixed pause before the single extra Lumia attempt
// that follows a provider outage. Operators can override it with the
// HwLumiaRetryPauseMs option; the default keeps a transient outage from being
// hammered while staying short enough for the customer not to notice.
var HwLumiaRetryPauseMs = 1000

// HwLumiaCooldownMs is the base cooldown of the Lumia breaker. One outage
// failure (429, 5xx, timeout) on any Lumia channel holds back the whole Lumia
// account for this long; each further consecutive failure doubles it up to five
// minutes. After the cooldown a single request probes Lumia. A success ends the
// cooldown and resets the escalation; zero or less turns the breaker off.
var HwLumiaCooldownMs = 60000
