package common

// HwLumiaRetryPauseMs is the fixed pause before the single extra Lumia attempt
// that follows a provider outage. Operators can override it with the
// HwLumiaRetryPauseMs option; the default keeps a transient outage from being
// hammered while staying short enough for the customer not to notice.
var HwLumiaRetryPauseMs = 1000

// HwLumiaCooldownMs is how long a Lumia channel that just failed with an outage
// (429, 5xx, timeout) is passed over while the request still has another
// candidate to try. A success on the channel ends the cooldown early; zero or
// less turns it off.
var HwLumiaCooldownMs = 60000
