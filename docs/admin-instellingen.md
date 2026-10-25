# Beheerinstellingen api.hisarweb.be (stand 24 okt 2026)

Opgesteld na het doorlopen van alle opties in het beheerpaneel (`/api/option/`).
Doel: geen standaardwaarden uit het upstream-project die niet bij HisarWeb
passen, geen dubbele of nutteloze opties, alles in Turks en in TRY.

## Doorgevoerd op 24 okt 2026

| Optie | Was | Nu | Reden |
|---|---|---|---|
| `console_setting.faq_enabled` | aan | uit | Blok toont niets: geen vragen ingevuld. De echte veelgestelde vragen staan op de klantpagina's. |
| `console_setting.api_info_enabled` | aan | uit | Leeg blok. |
| `console_setting.uptime_kuma_enabled` | aan | uit | Geen Uptime Kuma gekoppeld; de statuspagina is eigen werk (`/status`). |
| `DrawingEnabled` | aan | uit | Geen Midjourney-aanbod; het menu "Tekenen" leidt nergens heen. |
| `MjForwardUrlEnabled` | aan | uit | Hoort bij Midjourney. |
| `GroupGroupRatio` | `{"vip":{"edit_this":0.9}}` | `{}` | Voorbeeldwaarde uit het upstream-project ("edit_this"). |
| `PayMethods` | Alipay en WeChat (Chinees) | `[]` | Klanten betalen alleen met Stripe Checkout; Chinese methoden horen niet bij een Turks aanbod. |

## Bewust ongewijzigd, nagekeken

| Optie | Waarde | Toelichting |
|---|---|---|
| `general_setting.custom_currency_symbol` | ₺ | Klanten zien TRY. |
| `HwMinTopUpTry` / `HwMaxTopUpTry` | 100 / 20000 | Grenzen voor een bijstorting. |
| `HwMaxOpenOrders` | 3 | Maximaal drie openstaande bestellingen per klant. |
| `QuotaForNewUser`, `QuotaForInvitee` | 0 | Geen automatisch tegoed bij aanmelden of uitnodigen via de upstream-opties. |
| `fetch_setting.enable_ssrf_protection` | aan | Veiligheid, niet veranderen. |
| `ServerAddress` | `https://api.hisarweb.be` | Juist. |
| `legal.privacy_policy`, `legal.user_agreement` | Turks | Juist. |
| `AutomaticRetryStatusCodes` | upstream-standaard | Zie punt voor Altuner hieronder. |

## Nagekeken op 25 okt 2026

1. `RetryTimes = 0` blijft zo. Verzoeken via de eigen leveranciersroute
   (`service/hw_provider_policy.go`, `HwRetryLoopExtends`) proberen de volgende
   leverancier los van `RetryTimes`; een hogere waarde zou daar algemene
   herhalingen bovenop zetten.
2. `Price = 7.3`, `StripeUnitPrice = 8` en `payment_setting.amount_options` zijn
   yuan- en dollarresten uit upstream. Ze worden alleen gelezen door de upstream-
   betaalroutes, en die staan uit zolang `HW_UPSTREAM_PAYMENTS_ENABLED` niet aan
   staat (`router/api-router.go`). Geen invloed op klanten, dus niet aangeraakt.
3. `perf_metrics_setting.retention_days`: 0 betekende "nooit opruimen"
   (`pkg/perf_metrics/flush.go`). De standaard is nu 90 dagen. Staat er in de
   database al een eigen waarde, dan wint die; zet hem dan in het beheerpaneel
   op 90.

## Beheerassistent

In het beheerpaneel staat rechtsonder een assistent (alleen voor beheerders).
Melden van een fout of moeilijk woord geeft een voorstel; met één klik wordt een
tekst vervangen zonder nieuwe uitrol (tabel `hw_text_overrides`, publiek lezen
via `GET /api/hw/text-overrides?lang=tr`). Grotere fouten gaan als kaart naar
Bureau V2.

Instellingen op de server (Coolify), geen geheimen in de repo:

- `HW_ADMIN_AGENT_BASE_URL`, `HW_ADMIN_AGENT_API_KEY`, `HW_ADMIN_AGENT_MODEL`
  (standaard `claude-haiku-5-5`, voorlopig beste model voor de beste prijs). Zonder
  eigen waarden valt de assistent terug op `HW_QUALITY_BASE_URL` en `HW_QUALITY_API_KEY`.
- `HW_BUREAU_URL`, `HW_BUREAU_COMPANY_ID`, optioneel `HW_BUREAU_TOKEN` voor de
  kaart naar Bureau V2. Zonder deze meldt de knop eerlijk dat het bord niet is
  gekoppeld.
