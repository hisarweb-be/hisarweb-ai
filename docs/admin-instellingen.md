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

## Voor Altuner of de CTO-wachtrij (niet blind veranderd)

1. `RetryTimes = 0`: een mislukt verzoek wordt niet naar een tweede leverancier
   herhaald. Zodra er meerdere leveranciers per model zijn is 1 of 2 beter.
   Eigenaar: CTO-wachtrij, eerst meten welke modellen een tweede leverancier hebben.
2. `Price = 7.3` en `StripeUnitPrice = 8`: Chinese yuan-resten uit het
   upstream-project. HisarWeb rekent met de eigen TRY-prijzen (`Hw*`-opties);
   deze twee worden door de eigen betaalstroom niet gebruikt. Pas weghalen na
   controle in de code, daarom nu niet aangeraakt.
3. `perf_metrics_setting.retention_days = 0`: betekent waarschijnlijk "voor altijd
   bewaren". Een grens van 30 dagen houdt de database klein; eerst nagaan wat 0
   precies doet.
4. `payment_setting.amount_options = [10,20,50,100,200,500]`: dollarbedragen uit
   upstream. De klantpagina gebruikt de eigen pakketten, dus geen invloed.

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
