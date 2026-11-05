package model

// Default values of the options this fork reads. They live here, next to the
// option keys, because two places start from the same table: the option map
// built at startup and the seeding of a fresh database
// (service.HwSeedDefaultOptions). A single table keeps the value a running
// server shows and the value a new database stores from drifting apart.
//
// Service imports model, never the other way round, so the seeding in service/
// reads this table through HwDefaultOptions. model/hw_options_hook.go hands the
// seeding to service once at startup, which is how model reaches service
// without importing it.

// HwDefaultOptions holds the empty or shipped value of every option of this
// fork that has one. An option missing from this table has no built-in default
// and stays empty until an operator or a job writes it.
var HwDefaultOptions = map[string]string{
	// The TRY and EUR rates are published by the daily price job; until then no
	// rate exists and order creation is refused.
	HwTryPerUsdOption:         "",
	HwEurPerUsdOption:         "",
	HwEurPerUsdDateOption:     "",
	HwEurPresetBaseOption:     "",
	HwMinTopUpTryOption:       "100",
	HwMaxTopUpTryOption:       "20000",
	HwMaxOpenOrdersOption:     "3",
	HwLowBalancePercentOption: "10",
	HwLowBalanceMinTryOption:  "0",
	// An empty capacity means "unknown": the reserve gate then refuses customer
	// traffic instead of guessing a ceiling.
	HwWindowCapacityUSDOption:        "",
	HwPhase0MeasurementDoneOption:    "false",
	HwCustomerWindowCeilingUSDOption: "",
	HwCheckoutReturnURLOption:        "",
	HwBonusUsdMinOption:              "10",
	HwBonusUsdAmountOption:           "1",
	HwBonusEurMinOption:              "10",
	HwBonusEurAmountOption:           "1",
	HwBonusTryMinOption:              "500",
	HwBonusTryAmountOption:           "50",
	HwUpstreamPromptOverheadOption:   "",
	// The source URLs of the daily price snapshot and the published USD/TRY
	// rate; the keys are literals because their constants live in service/,
	// which imports this package.
	"HwPriceListURL":  "",
	"HwFxURL":         "",
	"HwTryPerUsdDate": "",
}
