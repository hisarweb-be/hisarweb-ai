# Woordenlijst beginnersmodus (Turks)

Regel: een gewone klant (rol 1 tot en met 9) ziet geen beheerderstaal. Geen kanaal, groep, ratio, RPM/TPM, endpoint of tokentelling. Beheerders (rol 10 en hoger) houden alles. De rolcontrole staat in `web/src/lib/roles.ts` (`isOrdinaryCustomer`) en `web/src/components/layout/lib/role-visibility.ts`.

Waarden staan in `web/src/i18n/locales/tr.json`. De sleutel is de Engelse brontekst en verandert nooit.

| Oud woord | Nieuw Turks woord | Waar gebruikt |
|---|---|---|
| General | Genel | Menugroep |
| Playground | Deneme Alanı | Menu, titel |
| Token Management | Anahtar Yönetimi | Menu, sleutelpagina |
| Create / Update / Delete API token | API anahtarı oluştur / güncelle / sil | Sleutelformulier |
| Token name | Anahtar adı | Sleutels |
| Copy token | Anahtarı kopyala | Sleutels |
| Quota (op sleutels) | Harcama limiti | Sleutelformulier |
| Unlimited Quota | Sınırsız limit | Sleutelformulier |
| Quota (bij tegoed) | Bakiye | Wallet, profiel |
| Remaining quota | Kalan bakiye | Dashboard |
| Total consumed quota | Toplam harcama | Dashboard |
| Requests made | İstek sayısı | Dashboard, Model Analizi |
| Total spent | Toplam harcama | Dashboard, Model Analizi |
| Quota Distribution | Harcama Dağılımı | Model Analizi |
| RPM / TPM | İstek/dk, Token/dk | Dashboard (beheerder) |
| Route active | Bağlantı çalışıyor | Dashboard (beheerder) |
| Build on your API gateway in minutes | Uygulamanızı dakikalar içinde bağlayın | Dashboard (beheerder) |
| Stream / Streaming | Canlı yanıt | Gebruikslog |
| Non-stream | Tek seferde | Gebruikslog |
| Ratio, Group Ratio | Çarpan, Grup çarpanı | Gebruikslog (beheerder) |
| Input / Output Tokens | Gönderilen token, Alınan token | Gebruikslog |
| Prompt | Soru | Gebruikslog, detailvenster |
| System prompt | Sistem talimatı | Gebruikslog |
| No Logs Found | Kayıt bulunamadı | Gebruikslog |
| Max Tokens | En uzun cevap | Deneme Alanı |
| Toggle Sidebar | Menüyü aç veya kapat | Schermlezer en tooltip |
| Request failed with status code 429 | Çok fazla istek gönderildi. Lütfen biraz bekleyip tekrar deneyin. | Foutmelding |
| By clicking sign in, you agree to our | Giriş yaparak şunları kabul etmiş olursunuz: | Inlogscherm |
| and (tussen voorwaarden) | ve | Inlogscherm, registratie |

## Wat een gewone klant niet ziet

- Menu: Yönetim (beheer), Dashboard-console, chatvoorinstellingen, auditlogs en takenlogs.
- Bovenmenu: console, modelcatalogus, ranglijst, over ons.
- Overzicht: API-adreskaart, uptime, startgids, "Önerilen işlemler".
- Model Analizi: Zaman Ayrıntısı en grafiekinstellingen. Alleen gewone perioden.
- Gebruikslog: kolommen RPM, TPM, stream, timing, kanaal, groep, ratio en tokendetails.
- Sleutelformulier: geavanceerde velden met veilige standaardwaarden.
- Profiel: zijbalkinstellingen, Webhook, Bark en Gotify, en de voorkeur voor prijsloze modellen. Meldingen gaan alleen per e-mail.
- Groepsnamen "default" en "paid" in de modelkiezer en op het profiel.

## Nieuwe tekst toevoegen

1. Zoek eerst of de sleutel al in `tr.json` staat. Pas de waarde aan, nooit de sleutel.
2. Staat de sleutel er niet in, voeg hem toe aan `tr.json` en `en.json`.
3. Kort, actief, geen jargon. Geen em-dash midden in een zin en geen emoji.
4. Bedragen en getallen via `@/lib/format` en `@/lib/currency`, nooit met een eigen afronding.
