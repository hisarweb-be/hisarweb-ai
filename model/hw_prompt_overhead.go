package model

// HwUpstreamPromptOverheadOption holds a JSON object of customer model name to
// prompt tokens that the supplier adds to every request without the customer
// having sent them. The optional key "*" is the default for models without an
// entry. Empty means no correction: the supplier's reported count is billed.
const HwUpstreamPromptOverheadOption = "HwUpstreamPromptOverhead"
